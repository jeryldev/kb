package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Workspace picker (modePicker) ---

type pickerModel struct {
	workspaces []*model.Workspace
	cursor     int
}

func (a *App) initPicker() {
	a.mode = modePicker
	if !a.reload() {
		return
	}
	a.picker.workspaces = a.db.ListWorkspaces()
	a.picker.cursor = max(0, min(a.picker.cursor, len(a.picker.workspaces)-1))

	// The board named at start-up opens once; after that the picker is the
	// picker, or "b" from that board would land straight back on it.
	if name := a.boardName; name != "" {
		a.boardName = ""
		board, err := a.db.GetBoard(name)
		if err != nil {
			if errors.Is(err, fstore.ErrNotFound) {
				a.feedback = fmt.Sprintf("No board named %q; pick a workspace", name)
			} else {
				a.err = err
			}
			return
		}
		if ws, err := a.db.GetWorkspace(board.WorkspaceID); err == nil {
			a.wsContent = wsContentModel{workspace: ws}
		}
		a.switchToBoard(board)
	}
}

func (a *App) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch key.String() {
	case "j", "down":
		if a.picker.cursor < len(a.picker.workspaces)-1 {
			a.picker.cursor++
		}
	case "k", "up":
		if a.picker.cursor > 0 {
			a.picker.cursor--
		}
	case "enter":
		if a.picker.cursor < len(a.picker.workspaces) {
			a.switchToWSContent(a.picker.workspaces[a.picker.cursor])
		}
	case "q":
		return a, tea.Quit
	}
	return a, nil
}

func (a *App) viewPicker() string {
	return a.frame(" kb: Select Workspace ", " j/k: select   enter: open   q: quit", nil, func(w, h int) string {
		var rows []string
		var heights []int
		for i, ws := range a.picker.workspaces {
			cursor, style := "  ", formValueStyle
			if i == a.picker.cursor {
				cursor, style = "▸ ", formLabelActiveStyle
			}
			line := cursor + style.Render(ws.Name) + " " + helpStyle.Render(fmt.Sprintf("(%s)", ws.Kind))
			if ws.Description != "" {
				line += helpStyle.Render("  " + truncate(ws.Description, 40))
			}
			rows = append(rows, ansi.Truncate(line, max(1, min(60, w-2)-6), "…"))
			heights = append(heights, 1)
		}
		// The dialog's border and padding take 4 lines.
		start, end := window(heights, a.picker.cursor, max(1, h-4))
		// Width leaves out the border, a cell on each side.
		dialog := dialogBoxStyle.Width(min(60, w-2)).Render(lipgloss.JoinVertical(lipgloss.Left, rows[start:end]...))
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
	})
}

// --- Workspace content (modeWSContent) ---

type wsContentModel struct {
	workspace  *model.Workspace
	boards     []*model.Board
	notes      []*model.Note
	cursor     int
	creating   string // "board" or "note" while a name is typed
	input      string
	confirming bool
}

func (m *wsContentModel) totalItems() int { return len(m.boards) + len(m.notes) }

func (m *wsContentModel) selectedBoard() *model.Board {
	if m.cursor < len(m.boards) {
		return m.boards[m.cursor]
	}
	return nil
}

func (m *wsContentModel) selectedNote() *model.Note {
	if i := m.cursor - len(m.boards); i >= 0 && i < len(m.notes) {
		return m.notes[i]
	}
	return nil
}

// switchToWSContent shows a workspace's boards and notes, as they are on
// disk now. Coming back to the same workspace keeps the cursor.
func (a *App) switchToWSContent(ws *model.Workspace) {
	a.mode = modeWSContent
	if a.wsContent.workspace == nil || a.wsContent.workspace.ID != ws.ID {
		a.wsContent = wsContentModel{workspace: ws}
	}
	a.wsContent.creating, a.wsContent.confirming = "", false
	a.loadWSContent()
}

func (a *App) loadWSContent() {
	m := &a.wsContent
	if !a.reload() {
		return
	}
	if fresh, err := a.db.GetWorkspace(m.workspace.ID); err == nil {
		m.workspace = fresh
	}
	m.boards = nil
	for _, b := range a.db.ListBoards() {
		if b.WorkspaceID == m.workspace.ID {
			m.boards = append(m.boards, b)
		}
	}
	m.notes = a.db.ListNotesByWorkspace(m.workspace.ID)
	m.cursor = max(0, min(m.cursor, m.totalItems()-1))
}

func (a *App) updateWSContent(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	m := &a.wsContent
	if m.creating != "" {
		return a.updateWSContentCreating(key)
	}
	if m.confirming {
		m.confirming = false
		if isYes(key) {
			a.deleteSelectedItem()
		}
		return a, nil
	}
	switch key.String() {
	case "j", "down":
		if m.cursor < m.totalItems()-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if b := m.selectedBoard(); b != nil {
			a.switchToBoard(b)
		} else if n := m.selectedNote(); n != nil {
			a.switchToNoteView(n.ID)
		}
	case "n":
		m.creating, m.input = "board", ""
	case "N":
		m.creating, m.input = "note", ""
	case "d", "D":
		m.confirming = m.totalItems() > 0
	case "b", "esc":
		a.initPicker()
	case "q":
		return a, tea.Quit
	}
	return a, nil
}

func isYes(key tea.KeyMsg) bool { return key.String() == "y" || key.String() == "Y" }

// deleteSelectedItem moves the selected board or note to the vault's trash.
func (a *App) deleteSelectedItem() {
	var dest string
	var err error
	if b := a.wsContent.selectedBoard(); b != nil {
		dest, err = a.db.TrashBoard(b.ID)
	} else if n := a.wsContent.selectedNote(); n != nil {
		dest, err = a.db.TrashNote(n.ID)
	} else {
		return
	}
	if a.fail(err) {
		return
	}
	a.loadWSContent()
	a.feedback = "Moved to " + dest
}

func (a *App) updateWSContentCreating(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m := &a.wsContent
	switch key.String() {
	case "enter":
		name := strings.TrimSpace(m.input)
		kind := m.creating
		m.creating = ""
		if name == "" {
			return a, nil
		}
		if kind == "note" {
			note, err := a.db.CreateNote(name, "", "", m.workspace.ID)
			if a.fail(err) {
				return a, nil
			}
			a.loadWSContent()
			a.feedback = fmt.Sprintf("Created %s", note.Path)
			return a, nil
		}
		board, err := a.db.CreateBoard(name, "", m.workspace.ID)
		if a.fail(err) {
			return a, nil
		}
		a.switchToBoard(board)
	case "esc":
		m.creating = ""
	case "backspace":
		if runes := []rune(m.input); len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	default:
		if text, ok := typedText(key); ok {
			m.input += text
		}
	}
	return a, nil
}

func (a *App) switchToBoard(board *model.Board) {
	a.board = boardModel{board: board}
	a.loadBoard()
	if a.err != nil {
		a.board = boardModel{}
		return
	}
	a.mode = modeBoard
}

func (a *App) viewWSContent() string {
	m := &a.wsContent
	ws := m.workspace
	title := fmt.Sprintf(" kb: %s (%s) ", ws.Name, ws.Kind)
	hints := " j/k: select   enter: open   n: new board   N: new note   d: delete   b: back   q: quit"
	return a.frame(title, hints, nil, func(w, h int) string {
		if m.creating != "" {
			label := "New board name:"
			if m.creating == "note" {
				label = "New note title:"
			}
			dialog := dialogBoxStyle.Width(min(50, w-2)).Render(lipgloss.JoinVertical(lipgloss.Left,
				formLabelActiveStyle.Render(label), "", "  "+m.input+"█", "",
				helpStyle.Render("  enter: create   esc: cancel")))
			return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
		}
		if m.confirming {
			prompt := ""
			if b := m.selectedBoard(); b != nil {
				prompt = fmt.Sprintf("Move board %q to the trash?", b.Name)
			} else if n := m.selectedNote(); n != nil {
				prompt = fmt.Sprintf("Move note %q to the trash?", n.Title)
			}
			return renderCenteredConfirm(w, h, prompt)
		}
		if m.totalItems() == 0 {
			return lipgloss.NewStyle().Padding(1, 2).Render(emptyColumnStyle.Render("No boards or notes in this workspace.") +
				"\n\n" + helpStyle.Render("Press n to create a board, N a note."))
		}

		// Each item is one row; the section headings are rows too, and
		// belong to the item below them so they scroll with it.
		var rows []string
		var heights []int
		item := func(i int, name, extra string) string {
			cursor, style := "  ", formValueStyle
			if i == m.cursor {
				cursor, style = "▸ ", formLabelActiveStyle
			}
			return cursor + style.Render(name) + extra
		}
		heading := func(s string) string { return lipgloss.NewStyle().Bold(true).Underline(true).Render(s) }
		for i, b := range m.boards {
			row := item(i, b.Name, helpStyle.Render("  "+truncate(b.Description, 30)))
			if i == 0 {
				row = heading("Boards") + "\n" + row
			}
			rows = append(rows, row)
		}
		for i, n := range m.notes {
			extra := ""
			if tags := n.TagList(); len(tags) > 0 {
				extra = "  " + labelStyle.Render("#"+strings.Join(tags, " #"))
			}
			row := item(len(m.boards)+i, n.Title, extra)
			if i == 0 {
				row = heading("Notes") + "\n" + row
				if len(m.boards) > 0 {
					row = "\n" + row
				}
			}
			rows = append(rows, row)
		}
		for _, row := range rows {
			heights = append(heights, strings.Count(row, "\n")+1)
		}
		// Padding takes 2 lines.
		start, end := window(heights, m.cursor, max(1, h-2))
		var lines []string
		for _, row := range rows[start:end] {
			for _, line := range strings.Split(row, "\n") {
				// One long row would widen the whole view past the terminal,
				// title bar and all, so each is cut to the space inside the
				// padding.
				lines = append(lines, ansi.Truncate(line, max(1, w-4), "…"))
			}
		}
		return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(lines, "\n"))
	})
}
