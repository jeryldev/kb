package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/fstore"

	tea "github.com/charmbracelet/bubbletea"
)

type mode int

const (
	modePicker mode = iota
	modeWSContent
	modeBoard
	modeCardView
	modeCardEdit
	modeNoteView
)

// App is the TUI. Every store call happens in Update, never in a tea.Cmd:
// the store is not safe for use from several goroutines. A call that
// writes reads the vault again, parsing only files that changed, which
// is quick enough to wait for.
type App struct {
	db        *fstore.Store
	boardName string
	mode      mode

	picker    pickerModel
	wsContent wsContentModel
	board     boardModel
	cardView  cardViewModel
	card      cardModel
	noteView  noteViewModel

	// err and feedback are shown in one bar above the key hints, on every
	// screen, until the next key; they never replace a screen's content.
	err      error
	feedback string

	width  int
	height int
}

func NewApp(db *fstore.Store, boardName string) *App {
	return &App{db: db, boardName: boardName, mode: modePicker}
}

func (a *App) Init() tea.Cmd {
	a.initPicker()
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		if a.board.board != nil {
			a.adjustScroll()
		}
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
		// Keys typed faster than kb reads them arrive as one event ("jj");
		// outside a text field each is a key of its own.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste && !a.typing() {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
				cmds = append(cmds, cmd)
			}
			return a, tea.Batch(cmds...)
		}
		a.err, a.feedback = nil, ""
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
	case modeNoteView:
		return a.updateNoteView(msg)
	}
	return a, nil
}

// typing reports whether a text field has the keys, which take a burst of
// characters as text.
func (a *App) typing() bool {
	switch {
	case a.mode == modeCardEdit && !a.card.confirmDiscard:
		return true
	case a.mode == modeBoard && a.board.filtering:
		return true
	case a.mode == modeWSContent && a.wsContent.creating != "":
		return true
	}
	return false
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
	case modeNoteView:
		return a.viewNoteDetail()
	}
	return ""
}

// fail shows an error and reports whether there was one.
func (a *App) fail(err error) bool {
	if err != nil {
		a.err = err
		return true
	}
	return false
}

// reload reads the vault again, so each screen shows what is on disk now,
// including changes made in an editor or by another kb.
func (a *App) reload() bool {
	return !a.fail(a.db.Reload())
}

func (a *App) size() (int, int) {
	w, h := a.width, a.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	return w, h
}

// messageBar is the error or feedback line, or "" when there is none.
func (a *App) messageBar(w int) string {
	switch {
	case a.err != nil:
		return errorStyle.Width(w).Render(truncate(fmt.Sprintf(" Error: %s", a.err), w))
	case a.feedback != "":
		return helpStyle.Width(w).Render(truncate(" "+a.feedback, w))
	}
	return ""
}

// frame lays out a screen: the title bar, the content (given the height
// it may use), the extra lines (a filter bar, say), the message bar and
// the key hints.
func (a *App) frame(title, hints string, extra []string, content func(w, h int) string) string {
	w, h := a.size()
	titleBar := titleBarStyle.Width(w).Render(truncate(title, max(1, w-2)))
	statusBar := statusBarStyle.Width(w).Render(truncate(hints, max(1, w-2)))
	// Each bar below the content is one line, cut to the width, so the
	// title bar is never pushed off the top.
	oneLine := lipgloss.NewStyle().MaxWidth(w).MaxHeight(1)
	var below []string
	for _, e := range extra {
		if e != "" {
			below = append(below, oneLine.Render(e))
		}
	}
	if m := a.messageBar(w); m != "" {
		below = append(below, oneLine.Render(m))
	}
	contentH := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - len(below)
	ch := max(1, contentH)
	body := lipgloss.NewStyle().Height(ch).MaxHeight(ch).MaxWidth(w).Render(content(w, ch))
	sections := append([]string{titleBar, body}, below...)
	sections = append(sections, statusBar)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// backToWorkspace leaves a board or note for the workspace it is in.
func (a *App) backToWorkspace() {
	if a.wsContent.workspace != nil {
		a.switchToWSContent(a.wsContent.workspace)
		return
	}
	a.mode = modePicker
	a.initPicker()
}

// window is the range of a list of n rows, each of the given height, to
// show in h lines so that the row at cursor is on screen.
func window(heights []int, cursor, h int) (start, end int) {
	n := len(heights)
	if n == 0 {
		return 0, 0
	}
	cursor = max(0, min(cursor, n-1))
	used := 0
	start = cursor
	for start >= 0 && used+heights[start] <= h {
		used += heights[start]
		start--
	}
	start++
	if start > cursor {
		start = cursor
	}
	end = start
	used = 0
	for end < n && used+heights[end] <= h {
		used += heights[end]
		end++
	}
	if end <= cursor {
		end = cursor + 1
	}
	return start, end
}
