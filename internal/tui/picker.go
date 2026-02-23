package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

type pickerModel struct {
	boards      []*model.Board
	cursor      int
	creating    bool
	confirming  string
	input       string
	err         error
	autoSelect  bool
	filter      string
	filterInput string
	filtering   bool
}

type boardsLoadedMsg struct {
	boards []*model.Board
}

type boardCreatedMsg struct {
	board *model.Board
}

type boardDeletedMsg struct{}

type errMsg struct {
	err error
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
			boards, err := a.db.ListBoards()
			if err != nil {
				return errMsg{err}
			}
			return boardsLoadedMsg{boards}
		}
	}

	return func() tea.Msg {
		boards, err := a.db.ListBoards()
		if err != nil {
			return errMsg{err}
		}
		return boardsLoadedMsg{boards}
	}
}

func (a *App) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case boardsLoadedMsg:
		a.picker.boards = msg.boards
		if a.picker.cursor >= len(msg.boards) && len(msg.boards) > 0 {
			a.picker.cursor = len(msg.boards) - 1
		}
		if len(msg.boards) == 1 && a.picker.autoSelect {
			a.picker.autoSelect = false
			return a, a.switchToBoard(msg.boards[0])
		}
		a.picker.autoSelect = false

	case boardCreatedMsg:
		return a, a.switchToBoard(msg.board)

	case boardDeletedMsg:
		return a, a.initPicker()

	case errMsg:
		a.picker.err = msg.err

	case tea.KeyMsg:
		if a.picker.creating {
			return a.updatePickerCreating(msg)
		}

		if a.picker.filtering {
			return a.updatePickerFiltering(msg)
		}

		if a.picker.confirming != "" {
			return a.updatePickerConfirming(msg)
		}

		visible := a.filteredBoards()

		switch msg.String() {
		case "j", "down":
			if a.picker.cursor < len(visible)-1 {
				a.picker.cursor++
			}
		case "k", "up":
			if a.picker.cursor > 0 {
				a.picker.cursor--
			}
		case "enter":
			if len(visible) > 0 && a.picker.cursor < len(visible) {
				board := visible[a.picker.cursor]
				return a, a.switchToBoard(board)
			}
		case "n":
			a.picker.creating = true
			a.picker.input = ""
			if a.boardName != "" {
				a.picker.input = a.boardName
			}
		case "d", "D":
			if len(visible) > 0 && a.picker.cursor < len(visible) {
				a.picker.confirming = "delete"
			}
		case "/":
			a.picker.filtering = true
			a.picker.filterInput = ""
		case "esc":
			if a.picker.filter != "" {
				a.picker.filter = ""
				a.picker.cursor = 0
			}
		case "q":
			return a, tea.Quit
		}
	}

	return a, nil
}

func (a *App) updatePickerConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		a.picker.confirming = ""
		visible := a.filteredBoards()
		if a.picker.cursor >= len(visible) {
			return a, nil
		}
		board := visible[a.picker.cursor]
		return a, func() tea.Msg {
			if err := a.db.DeleteBoard(board.ID); err != nil {
				return errMsg{err}
			}
			return boardDeletedMsg{}
		}
	default:
		a.picker.confirming = ""
	}
	return a, nil
}

func (a *App) updatePickerCreating(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(a.picker.input)
		if name == "" {
			a.picker.creating = false
			return a, nil
		}
		a.picker.creating = false
		return a, func() tea.Msg {
			board, err := a.db.CreateBoard(name, "")
			if err != nil {
				return errMsg{err}
			}
			return boardCreatedMsg{board}
		}
	case "esc":
		a.picker.creating = false
	case "backspace":
		if len(a.picker.input) > 0 {
			a.picker.input = a.picker.input[:len(a.picker.input)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.picker.input += msg.String()
		}
	}
	return a, nil
}

func (a *App) updatePickerFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		a.picker.filter = strings.TrimSpace(a.picker.filterInput)
		a.picker.filtering = false
		a.picker.cursor = 0
	case "esc":
		a.picker.filtering = false
		a.picker.filter = ""
		a.picker.cursor = 0
	case "backspace":
		if len(a.picker.filterInput) > 0 {
			a.picker.filterInput = a.picker.filterInput[:len(a.picker.filterInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.picker.filterInput += msg.String()
		}
	}
	return a, nil
}

func (a *App) filteredBoards() []*model.Board {
	if a.picker.filter == "" {
		return a.picker.boards
	}
	filter := strings.ToLower(a.picker.filter)
	var result []*model.Board
	for _, b := range a.picker.boards {
		if strings.Contains(strings.ToLower(b.Name), filter) ||
			strings.Contains(strings.ToLower(b.Description), filter) {
			result = append(result, b)
		}
	}
	return result
}

func (a *App) switchToBoard(board *model.Board) tea.Cmd {
	a.mode = modeBoard
	a.board = boardModel{
		board: board,
	}
	return a.loadBoard()
}

func (a *App) pickerLayout(w, h int, titleBar, filterBar, statusBar, content string) string {
	sections := []string{titleBar, content}
	if filterBar != "" {
		sections = append(sections, filterBar)
	}
	sections = append(sections, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (a *App) pickerContentHeight(h int, titleBar, filterBar, statusBar string) int {
	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	if filterBar != "" {
		contentHeight--
	}
	return contentHeight
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

	titleBar := titleBarStyle.Width(w).Render(" kb: Select Board ")
	statusBar := statusBarStyle.Width(w).Render(" j/k: select   enter: open   n: new board   /: search   d: delete board   q: quit")

	filterBar := ""
	if a.picker.filtering {
		filterBar = filterBarStyle.Render(fmt.Sprintf(" / %s", a.picker.filterInput)) + "█"
	} else if a.picker.filter != "" {
		filterBar = filterBarStyle.Render(fmt.Sprintf(" filter: %s", a.picker.filter)) +
			helpStyle.Render("  (/ to change, esc clears)")
	}

	contentHeight := a.pickerContentHeight(h, titleBar, filterBar, statusBar)

	var rows []string

	if a.picker.err != nil {
		rows = append(rows, errorStyle.Render(fmt.Sprintf("Error: %s", a.picker.err)))
		rows = append(rows, "")
	}

	if a.picker.creating {
		rows = append(rows, formLabelActiveStyle.Render("New board name:"))
		rows = append(rows, "")
		rows = append(rows, fmt.Sprintf("  %s█", a.picker.input))
		rows = append(rows, "")
		rows = append(rows, helpStyle.Render("  enter: create   esc: cancel"))

		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return a.pickerLayout(w, h, titleBar, filterBar, statusBar, content)
	}

	visible := a.filteredBoards()

	if len(visible) == 0 {
		if a.picker.filter != "" {
			rows = append(rows, helpStyle.Render("No boards match filter."))
		} else {
			rows = append(rows, helpStyle.Render("No boards found."))
			rows = append(rows, "")
			rows = append(rows, helpStyle.Render("Press n to create your first board."))
		}

		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return a.pickerLayout(w, h, titleBar, filterBar, statusBar, content)
	}

	for i, board := range visible {
		cursor := "  "
		style := formValueStyle
		if i == a.picker.cursor {
			cursor = "▸ "
			style = formLabelActiveStyle
		}
		line := fmt.Sprintf("%s%s", cursor, style.Render(board.Name))
		if board.Description != "" {
			line += helpStyle.Render(fmt.Sprintf("  %s", board.Description))
		}
		rows = append(rows, line)
	}

	var content string
	if a.picker.confirming != "" && a.picker.cursor < len(visible) {
		board := visible[a.picker.cursor]
		prompt := fmt.Sprintf("Delete board %q?", board.Name)
		content = renderCenteredConfirm(w, contentHeight, prompt)
	} else {
		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content = lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
	}

	return a.pickerLayout(w, h, titleBar, filterBar, statusBar, content)
}
