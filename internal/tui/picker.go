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

		if a.picker.confirming != "" {
			return a.updatePickerConfirming(msg)
		}

		switch msg.String() {
		case "j", "down":
			if a.picker.cursor < len(a.picker.boards)-1 {
				a.picker.cursor++
			}
		case "k", "up":
			if a.picker.cursor > 0 {
				a.picker.cursor--
			}
		case "enter":
			if len(a.picker.boards) > 0 {
				board := a.picker.boards[a.picker.cursor]
				return a, a.switchToBoard(board)
			}
		case "n":
			a.picker.creating = true
			a.picker.input = ""
			if a.boardName != "" {
				a.picker.input = a.boardName
			}
		case "d", "D":
			if len(a.picker.boards) > 0 {
				a.picker.confirming = "delete"
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
		if a.picker.cursor >= len(a.picker.boards) {
			return a, nil
		}
		board := a.picker.boards[a.picker.cursor]
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

func (a *App) switchToBoard(board *model.Board) tea.Cmd {
	a.mode = modeBoard
	a.board = boardModel{
		board: board,
	}
	return a.loadBoard()
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
	statusBar := statusBarStyle.Width(w).Render(" j/k: select   enter: open   n: new board   d: delete board   q: quit")

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
		contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	if len(a.picker.boards) == 0 {
		rows = append(rows, helpStyle.Render("No boards found."))
		rows = append(rows, "")
		rows = append(rows, helpStyle.Render("Press n to create your first board."))

		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
		content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
		return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
	}

	for i, board := range a.picker.boards {
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

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1

	var content string
	if a.picker.confirming != "" && a.picker.cursor < len(a.picker.boards) {
		board := a.picker.boards[a.picker.cursor]
		prompt := fmt.Sprintf("Delete board %q?", board.Name)
		content = renderCenteredConfirm(w, contentHeight, prompt)
	} else {
		dialog := dialogBoxStyle.Width(50).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
		content = lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
	}

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}
