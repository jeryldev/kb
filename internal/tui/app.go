package tui

import (
	"github.com/jeryldev/kb/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modePicker mode = iota
	modeBoard
	modeCardView
	modeCardEdit
)

type App struct {
	db        *store.DB
	boardName string
	mode      mode

	picker   pickerModel
	board    boardModel
	cardView cardViewModel
	card     cardModel

	width  int
	height int
}

func NewApp(db *store.DB, boardName string) *App {
	return &App{
		db:        db,
		boardName: boardName,
		mode:      modePicker,
	}
}

func (a *App) Init() tea.Cmd {
	a.picker.autoSelect = true
	return a.initPicker()
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	switch a.mode {
	case modePicker:
		return a.updatePicker(msg)
	case modeBoard:
		return a.updateBoard(msg)
	case modeCardView:
		return a.updateCardView(msg)
	case modeCardEdit:
		return a.updateCard(msg)
	}

	return a, nil
}

func (a *App) View() string {
	switch a.mode {
	case modePicker:
		return a.viewPicker()
	case modeBoard:
		return a.viewBoard()
	case modeCardView:
		return a.viewCardReadonly()
	case modeCardEdit:
		return a.viewCard()
	}
	return ""
}
