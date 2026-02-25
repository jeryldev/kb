package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Shared messages ---

type errMsg struct {
	err error
}

type boardCreatedMsg struct {
	board *model.Board
}

type boardDeletedMsg struct {
	workspaceID string
}

// --- Workspace picker (modePicker) ---

type pickerModel struct {
	workspaces []*model.Workspace
	cursor     int
	err        error
	autoSelect bool
}

type workspacesLoadedMsg struct {
	workspaces []*model.Workspace
}

func (a *App) initPicker() tea.Cmd {
	if a.boardName != "" {
		return func() tea.Msg {
			board, err := a.db.GetBoardByName(a.boardName)
			if err != nil {
				return errMsg{err}
			}
			if board != nil {
				return boardCreatedMsg{board}
			}
			workspaces, err := a.db.ListWorkspaces()
			if err != nil {
				return errMsg{err}
			}
			return workspacesLoadedMsg{workspaces}
		}
	}

	return func() tea.Msg {
		workspaces, err := a.db.ListWorkspaces()
		if err != nil {
			return errMsg{err}
		}
		return workspacesLoadedMsg{workspaces}
	}
}

func (a *App) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case workspacesLoadedMsg:
		a.picker.workspaces = msg.workspaces
		if a.picker.cursor >= len(msg.workspaces) && len(msg.workspaces) > 0 {
			a.picker.cursor = len(msg.workspaces) - 1
		}
		if len(msg.workspaces) == 1 && a.picker.autoSelect {
			a.picker.autoSelect = false
			return a, a.switchToWSContent(msg.workspaces[0])
		}
		a.picker.autoSelect = false

	case boardCreatedMsg:
		return a, a.switchToBoard(msg.board)

	case errMsg:
		a.picker.err = msg.err

	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if a.picker.cursor < len(a.picker.workspaces)-1 {
				a.picker.cursor++
			}
		case "k", "up":
			if a.picker.cursor > 0 {
				a.picker.cursor--
			}
		case "enter":
			if len(a.picker.workspaces) > 0 && a.picker.cursor < len(a.picker.workspaces) {
				return a, a.switchToWSContent(a.picker.workspaces[a.picker.cursor])
			}
		case "q":
			return a, tea.Quit
		}
	}

	return a, nil
}

func (a *App) viewPicker() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	titleBar := titleBarStyle.Width(w).Render(" kb: Select Workspace ")
	statusBar := statusBarStyle.Width(w).Render(" j/k: select   enter: open   q: quit")

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1

	var rows []string

	if a.picker.err != nil {
		rows = append(rows, errorStyle.Render(fmt.Sprintf("Error: %s", a.picker.err)))
		rows = append(rows, "")
	}

	if len(a.picker.workspaces) == 0 {
		rows = append(rows, helpStyle.Render("No workspaces found."))
		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	for i, ws := range a.picker.workspaces {
		cursor := "  "
		style := formValueStyle
		if i == a.picker.cursor {
			cursor = "▸ "
			style = formLabelActiveStyle
		}
		badge := helpStyle.Render(fmt.Sprintf("(%s)", ws.Kind))
		line := fmt.Sprintf("%s%s %s", cursor, style.Render(ws.Name), badge)
		if ws.Description != "" {
			line += helpStyle.Render(fmt.Sprintf("  %s", truncate(ws.Description, 40)))
		}
		rows = append(rows, line)
	}

	dialog := dialogBoxStyle.Width(60).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}

// --- Workspace content (modeWSContent) ---

type wsContentModel struct {
	workspace  *model.Workspace
	boards     []*model.Board
	notes      []*model.Note
	cursor     int
	creating   string
	input      string
	confirming string
	err        error
	feedback   string
}

type noteCreatedMsg struct {
	note *model.Note
}

type noteDeletedMsg struct {
	workspaceID string
}

type wsContentLoadedMsg struct {
	boards []*model.Board
	notes  []*model.Note
}

func (m *wsContentModel) totalItems() int {
	return len(m.boards) + len(m.notes)
}

func (m *wsContentModel) selectedKind() string {
	if m.cursor < len(m.boards) {
		return "board"
	}
	return "note"
}

func (m *wsContentModel) selectedBoard() *model.Board {
	if m.cursor < len(m.boards) {
		return m.boards[m.cursor]
	}
	return nil
}

func (m *wsContentModel) selectedNote() *model.Note {
	idx := m.cursor - len(m.boards)
	if idx >= 0 && idx < len(m.notes) {
		return m.notes[idx]
	}
	return nil
}

func (a *App) switchToWSContent(ws *model.Workspace) tea.Cmd {
	a.mode = modeWSContent
	a.wsContent = wsContentModel{workspace: ws}
	return a.loadWSContent(ws.ID)
}

func (a *App) loadWSContent(workspaceID string) tea.Cmd {
	return func() tea.Msg {
		boards, err := a.db.ListBoardsByWorkspace(workspaceID)
		if err != nil {
			return errMsg{err}
		}
		notes, err := a.db.ListNotesByWorkspace(workspaceID)
		if err != nil {
			return errMsg{err}
		}
		return wsContentLoadedMsg{boards: boards, notes: notes}
	}
}

func (a *App) updateWSContent(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case wsContentLoadedMsg:
		a.wsContent.boards = msg.boards
		a.wsContent.notes = msg.notes
		if a.wsContent.cursor >= a.wsContent.totalItems() && a.wsContent.totalItems() > 0 {
			a.wsContent.cursor = a.wsContent.totalItems() - 1
		}

	case boardCreatedMsg:
		return a, a.switchToBoard(msg.board)

	case boardDeletedMsg:
		a.wsContent.feedback = "Board deleted"
		return a, a.loadWSContent(msg.workspaceID)

	case errMsg:
		a.wsContent.err = msg.err

	case noteCreatedMsg:
		a.wsContent.feedback = fmt.Sprintf("Note %q created", msg.note.Title)
		return a, a.loadWSContent(a.wsContent.workspace.ID)

	case noteDeletedMsg:
		a.wsContent.feedback = "Note deleted"
		return a, a.loadWSContent(msg.workspaceID)

	case tea.KeyMsg:
		a.wsContent.feedback = ""

		if a.wsContent.creating != "" {
			return a.updateWSContentCreating(msg)
		}

		if a.wsContent.confirming != "" {
			return a.updateWSContentConfirming(msg)
		}

		total := a.wsContent.totalItems()

		switch msg.String() {
		case "j", "down":
			if a.wsContent.cursor < total-1 {
				a.wsContent.cursor++
			}
		case "k", "up":
			if a.wsContent.cursor > 0 {
				a.wsContent.cursor--
			}
		case "enter":
			if total > 0 {
				if a.wsContent.selectedKind() == "board" {
					return a, a.switchToBoard(a.wsContent.selectedBoard())
				}
				return a, a.switchToNoteView(a.wsContent.selectedNote())
			}
		case "n":
			a.wsContent.creating = "board"
			a.wsContent.input = ""
		case "N":
			a.wsContent.creating = "note"
			a.wsContent.input = ""
		case "d", "D":
			if total > 0 {
				a.wsContent.confirming = "delete"
			}
		case "b", "esc":
			a.mode = modePicker
			return a, a.initPicker()
		case "q":
			return a, tea.Quit
		}
	}

	return a, nil
}

func (a *App) updateWSContentCreating(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(a.wsContent.input)
		if name == "" {
			a.wsContent.creating = ""
			return a, nil
		}
		kind := a.wsContent.creating
		a.wsContent.creating = ""
		wsID := a.wsContent.workspace.ID
		if kind == "note" {
			slug := model.Slugify(name)
			return a, func() tea.Msg {
				note, err := a.db.CreateNote(name, slug, "", wsID)
				if err != nil {
					return errMsg{err}
				}
				return noteCreatedMsg{note}
			}
		}
		return a, func() tea.Msg {
			board, err := a.db.CreateBoard(name, "", wsID)
			if err != nil {
				return errMsg{err}
			}
			return boardCreatedMsg{board}
		}
	case "esc":
		a.wsContent.creating = ""
	case "backspace":
		if len(a.wsContent.input) > 0 {
			a.wsContent.input = a.wsContent.input[:len(a.wsContent.input)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.wsContent.input += msg.String()
		}
	}
	return a, nil
}

func (a *App) updateWSContentConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		a.wsContent.confirming = ""
		wsID := a.wsContent.workspace.ID
		if a.wsContent.selectedKind() == "board" {
			board := a.wsContent.selectedBoard()
			if board == nil {
				return a, nil
			}
			return a, func() tea.Msg {
				if err := a.db.DeleteBoard(board.ID); err != nil {
					return errMsg{err}
				}
				return boardDeletedMsg{workspaceID: wsID}
			}
		}
		note := a.wsContent.selectedNote()
		if note == nil {
			return a, nil
		}
		return a, func() tea.Msg {
			if err := a.db.DeleteNote(note.ID); err != nil {
				return errMsg{err}
			}
			return noteDeletedMsg{workspaceID: wsID}
		}
	default:
		a.wsContent.confirming = ""
	}
	return a, nil
}

func (a *App) switchToBoard(board *model.Board) tea.Cmd {
	a.mode = modeBoard
	a.board = boardModel{
		board: board,
	}
	return a.loadBoard()
}

func (a *App) viewWSContent() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	ws := a.wsContent.workspace
	titleBar := titleBarStyle.Width(w).Render(fmt.Sprintf(" kb: %s (%s) ", ws.Name, ws.Kind))
	statusBar := statusBarStyle.Width(w).Render(" j/k: select   enter: open   n: new board   N: new note   d: delete   b: back   q: quit")

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1

	if a.wsContent.err != nil {
		content := errorStyle.Render(fmt.Sprintf("Error: %v", a.wsContent.err))
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	if a.wsContent.creating != "" {
		label := "New board name:"
		if a.wsContent.creating == "note" {
			label = "New note title:"
		}
		var rows []string
		rows = append(rows, formLabelActiveStyle.Render(label))
		rows = append(rows, "")
		rows = append(rows, fmt.Sprintf("  %s█", a.wsContent.input))
		rows = append(rows, "")
		rows = append(rows, helpStyle.Render("  enter: create   esc: cancel"))

		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	if a.wsContent.confirming != "" {
		var prompt string
		if a.wsContent.selectedKind() == "board" {
			board := a.wsContent.selectedBoard()
			name := ""
			if board != nil {
				name = board.Name
			}
			prompt = fmt.Sprintf("Delete board %q?", name)
		} else {
			note := a.wsContent.selectedNote()
			name := ""
			if note != nil {
				name = note.Title
			}
			prompt = fmt.Sprintf("Delete note %q?", name)
		}
		content := renderCenteredConfirm(w, contentHeight, prompt)
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	var rows []string
	idx := 0

	// Boards section
	if len(a.wsContent.boards) > 0 {
		rows = append(rows, lipgloss.NewStyle().Bold(true).Underline(true).Render("Boards"))
		for _, board := range a.wsContent.boards {
			cursor := "  "
			style := formValueStyle
			if idx == a.wsContent.cursor {
				cursor = "▸ "
				style = formLabelActiveStyle
			}
			line := fmt.Sprintf("%s%s", cursor, style.Render(board.Name))
			if board.Description != "" {
				line += helpStyle.Render("  " + truncate(board.Description, 30))
			}
			rows = append(rows, line)
			idx++
		}
	}

	// Notes section
	if len(a.wsContent.notes) > 0 {
		if len(a.wsContent.boards) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, lipgloss.NewStyle().Bold(true).Underline(true).Render("Notes"))
		for _, note := range a.wsContent.notes {
			cursor := "  "
			style := formValueStyle
			if idx == a.wsContent.cursor {
				cursor = "▸ "
				style = formLabelActiveStyle
			}
			slug := helpStyle.Render(note.Slug)
			tags := ""
			if note.Tags != "" {
				tags = "  " + labelStyle.Render("["+note.Tags+"]")
			}
			line := fmt.Sprintf("%s%s  %s%s", cursor, style.Render(note.Title), slug, tags)
			rows = append(rows, line)
			idx++
		}
	}

	if len(a.wsContent.boards) == 0 && len(a.wsContent.notes) == 0 {
		rows = append(rows, emptyColumnStyle.Render("No boards or notes in this workspace."))
		rows = append(rows, "")
		rows = append(rows, helpStyle.Render("Press n to create a board."))
	}

	if a.wsContent.feedback != "" {
		rows = append(rows, "")
		rows = append(rows, helpStyle.Render(a.wsContent.feedback))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	padded := lipgloss.NewStyle().Padding(1, 2).Height(contentHeight).Render(content)
	return lipgloss.JoinVertical(lipgloss.Left, titleBar, padded, statusBar)
}
