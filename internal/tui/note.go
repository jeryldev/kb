package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	cachedEditor     string
	cachedEditorOnce sync.Once
)

type noteListModel struct {
	notes       []*model.Note
	cursor      int
	filter      string
	filterInput string
	filtering   bool
	err         error
}

type backlinkDisplay struct {
	label   string
	context string
}

type noteViewModel struct {
	note       *model.Note
	backlinks  []backlinkDisplay
	scroll     int
	confirming string
}

type notesLoadedMsg struct {
	notes []*model.Note
}

type noteBacklinksMsg struct {
	note      *model.Note
	backlinks []backlinkDisplay
}

type noteEditedMsg struct {
	note *model.Note
}

func (a *App) switchToNoteView(note *model.Note) tea.Cmd {
	a.mode = modeNoteView
	a.noteView = noteViewModel{note: note}
	return func() tea.Msg {
		links, err := a.db.GetBacklinks("note", note.ID)
		if err != nil {
			return errMsg{err}
		}
		var blds []backlinkDisplay
		for _, bl := range links {
			label := bl.SourceID[:min(8, len(bl.SourceID))]
			sourceNote, err := a.db.GetNote(bl.SourceID)
			if err == nil && sourceNote != nil {
				label = fmt.Sprintf("[[%s]] %s", sourceNote.Slug, sourceNote.Title)
			}
			blds = append(blds, backlinkDisplay{label: label, context: bl.Context})
		}
		return noteBacklinksMsg{note: note, backlinks: blds}
	}
}

// --- Note List Mode ---

func (a *App) updateNoteList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case notesLoadedMsg:
		a.noteList.notes = msg.notes
		if a.noteList.cursor >= len(msg.notes) && len(msg.notes) > 0 {
			a.noteList.cursor = len(msg.notes) - 1
		}

	case errMsg:
		a.noteList.err = msg.err

	case tea.KeyMsg:
		if a.noteList.filtering {
			return a.updateNoteListFiltering(msg)
		}

		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "b", "esc":
			if a.wsContent.workspace != nil {
				return a, a.switchToWSContent(a.wsContent.workspace)
			}
			a.mode = modePicker
			return a, a.initPicker()
		case "j", "down":
			notes := a.filteredNotes()
			if a.noteList.cursor < len(notes)-1 {
				a.noteList.cursor++
			}
		case "k", "up":
			if a.noteList.cursor > 0 {
				a.noteList.cursor--
			}
		case "enter":
			notes := a.filteredNotes()
			if len(notes) > 0 && a.noteList.cursor < len(notes) {
				return a, a.switchToNoteView(notes[a.noteList.cursor])
			}
		case "/":
			a.noteList.filtering = true
			a.noteList.filterInput = a.noteList.filter
		}
	}
	return a, nil
}

func (a *App) updateNoteListFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		a.noteList.filter = a.noteList.filterInput
		a.noteList.filtering = false
		a.noteList.cursor = 0
	case "esc":
		a.noteList.filter = ""
		a.noteList.filterInput = ""
		a.noteList.filtering = false
		a.noteList.cursor = 0
	case "backspace":
		if len(a.noteList.filterInput) > 0 {
			a.noteList.filterInput = a.noteList.filterInput[:len(a.noteList.filterInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.noteList.filterInput += msg.String()
		}
	}
	return a, nil
}

func (a *App) filteredNotes() []*model.Note {
	if a.noteList.filter == "" {
		return a.noteList.notes
	}
	f := strings.ToLower(a.noteList.filter)
	var result []*model.Note
	for _, n := range a.noteList.notes {
		if strings.Contains(strings.ToLower(n.Title), f) ||
			strings.Contains(strings.ToLower(n.Slug), f) ||
			strings.Contains(strings.ToLower(n.Tags), f) {
			result = append(result, n)
		}
	}
	return result
}

func (a *App) viewNoteList() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	titleBar := titleBarStyle.Width(w).Render(" Notes ")

	statusHints := "j/k: navigate   Enter: view   /: filter   b: back   q: quit"
	statusBar := statusBarStyle.Width(w).Render(" " + statusHints)

	var filterBar string
	if a.noteList.filtering {
		filterBar = filterBarStyle.Render("Filter: " + a.noteList.filterInput + "█")
	} else if a.noteList.filter != "" {
		filterBar = filterBarStyle.Render("Filter: " + a.noteList.filter + "  (/ to edit, Esc to clear)")
	}

	notes := a.filteredNotes()

	contentH := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	if filterBar != "" {
		contentH -= lipgloss.Height(filterBar) + 1
	}

	if a.noteList.err != nil {
		content := errorStyle.Render(fmt.Sprintf("Error: %v", a.noteList.err))
		return a.noteListLayout(w, titleBar, filterBar, statusBar, content)
	}

	if len(notes) == 0 {
		msg := "No notes found."
		if a.noteList.filter != "" {
			msg = fmt.Sprintf("No notes matching %q", a.noteList.filter)
		}
		content := emptyColumnStyle.Render(msg)
		return a.noteListLayout(w, titleBar, filterBar, statusBar, content)
	}

	// Determine visible range for scrolling
	visibleStart := 0
	if a.noteList.cursor >= contentH {
		visibleStart = a.noteList.cursor - contentH + 1
	}

	var rows []string
	for i := visibleStart; i < len(notes) && len(rows) < contentH; i++ {
		n := notes[i]
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == a.noteList.cursor {
			cursor = "> "
			style = style.Bold(true)
		}

		slug := helpStyle.Render(n.Slug)
		tags := ""
		if n.Tags != "" {
			tags = "  " + labelStyle.Render("["+n.Tags+"]")
		}
		updated := helpStyle.Render(relativeTime(n.UpdatedAt))

		title := style.Render(n.Title)
		line := fmt.Sprintf("%s%s  %s%s  %s", cursor, title, slug, tags, updated)

		if len(line) > w {
			line = line[:w]
		}
		rows = append(rows, line)
	}

	content := strings.Join(rows, "\n")
	return a.noteListLayout(w, titleBar, filterBar, statusBar, content)
}

func (a *App) noteListLayout(_ int, titleBar, filterBar, statusBar, content string) string {
	h := a.height
	if h == 0 {
		h = 24
	}
	contentH := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	if filterBar != "" {
		contentH -= lipgloss.Height(filterBar)
	}
	sized := lipgloss.NewStyle().Height(contentH).Render(content)

	sections := []string{titleBar, sized}
	if filterBar != "" {
		sections = []string{titleBar, filterBar, sized}
	}
	sections = append(sections, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// --- Note View Mode ---

func (a *App) updateNoteView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case noteBacklinksMsg:
		a.noteView.backlinks = msg.backlinks

	case noteEditedMsg:
		a.noteView.note = msg.note

	case noteDeletedMsg:
		if a.wsContent.workspace != nil {
			return a, a.switchToWSContent(a.wsContent.workspace)
		}
		a.mode = modePicker
		return a, a.initPicker()

	case errMsg:
		if a.wsContent.workspace != nil {
			a.wsContent.err = msg.err
			return a, a.switchToWSContent(a.wsContent.workspace)
		}
		a.mode = modePicker
		return a, a.initPicker()

	case tea.KeyMsg:
		if a.noteView.confirming != "" {
			return a.updateNoteViewConfirming(msg)
		}

		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "b", "esc":
			if a.wsContent.workspace != nil {
				return a, a.switchToWSContent(a.wsContent.workspace)
			}
			a.mode = modePicker
			return a, a.initPicker()
		case "e":
			return a, a.editNoteExternal()
		case "d":
			a.noteView.confirming = "delete"
		case "j", "down":
			a.noteView.scroll++
		case "k", "up":
			if a.noteView.scroll > 0 {
				a.noteView.scroll--
			}
		}
	}
	return a, nil
}

func (a *App) updateNoteViewConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		a.noteView.confirming = ""
		note := a.noteView.note
		if note == nil {
			return a, nil
		}
		wsID := ""
		if a.wsContent.workspace != nil {
			wsID = a.wsContent.workspace.ID
		}
		return a, func() tea.Msg {
			if err := a.db.DeleteNote(note.ID); err != nil {
				return errMsg{err}
			}
			return noteDeletedMsg{workspaceID: wsID}
		}
	default:
		a.noteView.confirming = ""
	}
	return a, nil
}

// --- Note Edit (external editor) ---

func resolveEditor() string {
	cachedEditorOnce.Do(func() {
		if ed := os.Getenv("EDITOR"); ed != "" {
			cachedEditor = ed
			return
		}
		for _, name := range []string{"nvim", "vim", "vi", "nano"} {
			if p, err := exec.LookPath(name); err == nil {
				cachedEditor = p
				return
			}
		}
	})
	return cachedEditor
}

func editorDisplayName(editor string) string {
	return filepath.Base(editor)
}

func (a *App) editNoteExternal() tea.Cmd {
	note := a.noteView.note
	if note == nil {
		return nil
	}

	editor := resolveEditor()
	if editor == "" {
		return nil
	}

	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kb-note-%s-*.md", note.Slug))
	if err != nil {
		return func() tea.Msg { return errMsg{err} }
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(note.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return func() tea.Msg { return errMsg{err} }
	}
	tmpFile.Close()

	c := exec.Command(editor, tmpPath)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	noteID := note.ID
	db := a.db

	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return errMsg{err}
		}
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return errMsg{err}
		}
		current, err := db.GetNote(noteID)
		if err != nil {
			return errMsg{err}
		}
		current.Body = string(data)
		if err := db.UpdateNote(current); err != nil {
			return errMsg{err}
		}
		return noteEditedMsg{note: current}
	})
}

func (a *App) viewNoteDetail() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	note := a.noteView.note
	titleBar := titleBarStyle.Width(w).Render(" " + note.Title + " ")
	editHint := "e: edit"
	if editor := resolveEditor(); editor != "" {
		editHint = fmt.Sprintf("e: edit (%s)", editorDisplayName(editor))
	}
	statusBar := statusBarStyle.Width(w).Render(fmt.Sprintf(" j/k: scroll   %s   d: delete   b: back   q: quit", editHint))

	contentH := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	contentW := max(20, w-4)

	if a.noteView.confirming != "" {
		content := renderCenteredConfirm(w, contentH, fmt.Sprintf("Delete note %q?", note.Title))
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	var sections []string

	// Metadata line
	meta := helpStyle.Render(fmt.Sprintf("[[%s]]   Updated: %s", note.Slug, relativeTime(note.UpdatedAt)))
	if note.Tags != "" {
		meta += "   " + labelStyle.Render("["+note.Tags+"]")
	}
	sections = append(sections, meta, "")

	// Body
	if note.Body != "" {
		body := lipgloss.NewStyle().Width(contentW).Render(note.Body)
		sections = append(sections, body)
	} else {
		sections = append(sections, emptyColumnStyle.Render("(empty note)"))
	}

	// Backlinks
	if len(a.noteView.backlinks) > 0 {
		sections = append(sections, "")
		blHeader := lipgloss.NewStyle().Bold(true).Underline(true).Render(
			fmt.Sprintf("Backlinks (%d)", len(a.noteView.backlinks)))
		sections = append(sections, blHeader)

		for _, bl := range a.noteView.backlinks {
			ctx := ""
			if bl.context != "" {
				ctx = "  " + helpStyle.Render("\""+bl.context+"\"")
			}
			sections = append(sections, "  "+bl.label+ctx)
		}
	}

	allContent := strings.Join(sections, "\n")
	lines := strings.Split(allContent, "\n")

	// Apply scroll
	if a.noteView.scroll > len(lines)-contentH {
		a.noteView.scroll = max(0, len(lines)-contentH)
	}
	start := a.noteView.scroll
	end := min(start+contentH, len(lines))
	visible := lines[start:end]

	inner := "  " + strings.Join(visible, "\n  ")
	content := lipgloss.NewStyle().Height(contentH).Render(inner)
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}
