package tui

import (
	"github.com/jeryldev/kb/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modePicker mode = iota
	modeWSContent
	modeBoard
	modeCardView
	modeCardEdit
	modeNotes
	modeNoteView
)

type App struct {
	db        *store.DB
	boardName string
	mode      mode

	picker    pickerModel
	wsContent wsContentModel
	board     boardModel
	cardView  cardViewModel
	card      cardModel
	noteList  noteListModel
	noteView  noteViewModel

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
	case modeWSContent:
		return a.updateWSContent(msg)
	case modeBoard:
		return a.updateBoard(msg)
	case modeCardView:
		return a.updateCardView(msg)
	case modeCardEdit:
		return a.updateCard(msg)
	case modeNotes:
		return a.updateNoteList(msg)
	case modeNoteView:
		return a.updateNoteView(msg)
	}

	return a, nil
}

func (a *App) View() string {
	switch a.mode {
	case modePicker:
		return a.viewPicker()
	case modeWSContent:
		return a.viewWSContent()
	case modeBoard:
		return a.viewBoard()
	case modeCardView:
		return a.viewCardReadonly()
	case modeCardEdit:
		return a.viewCard()
	case modeNotes:
		return a.viewNoteList()
	case modeNoteView:
		return a.viewNoteDetail()
	}
	return ""
}
