package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
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
	note      *model.Note
	backlinks []backlinkDisplay
	scroll    int
}

type notesLoadedMsg struct {
	notes []*model.Note
}

type noteBacklinksMsg struct {
	note      *model.Note
	backlinks []backlinkDisplay
}

func (a *App) loadNotes() tea.Cmd {
	return func() tea.Msg {
		notes, err := a.db.ListNotes()
		if err != nil {
			return errMsg{err}
		}
		return notesLoadedMsg{notes}
	}
}

func (a *App) switchToNotes() tea.Cmd {
	a.mode = modeNotes
	a.noteList = noteListModel{}
	return a.loadNotes()
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
	sections := []string{titleBar, content}
	if filterBar != "" {
		sections = []string{titleBar, filterBar, content}
	}
	sections = append(sections, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// --- Note View Mode ---

func (a *App) updateNoteView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case noteBacklinksMsg:
		a.noteView.backlinks = msg.backlinks

	case errMsg:
		a.mode = modeNotes
		a.noteList.err = msg.err
		return a, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "b", "esc":
			a.mode = modeNotes
			return a, a.loadNotes()
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
	statusBar := statusBarStyle.Width(w).Render(" j/k: scroll   b: back   q: quit")

	contentH := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	contentW := max(20, w-4)

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

	content := "  " + strings.Join(visible, "\n  ")
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}
