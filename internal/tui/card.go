package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

var timeNow = time.Now

func relativeTime(t time.Time) string {
	d := timeNow().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return t.Format("02 Jan 2006")
	}
}

// --- Card viewer (modeCardView) ---

type cardViewModel struct {
	card       *model.Card
	confirming string
}

func (a *App) updateCardView(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	v := &a.cardView
	if v.confirming == "" && !a.followCardOnDisk() {
		return a, nil
	}
	if v.confirming != "" {
		action := v.confirming
		v.confirming = ""
		if isYes(key) && a.archiveOrDelete(action, v.card) {
			a.mode = modeBoard
			a.loadBoard()
		}
		return a, nil
	}
	switch key.String() {
	case "e":
		return a, a.editSelectedCard(modeCardView)
	case "d":
		v.confirming = "archive"
	case "D":
		v.confirming = "delete"
	case "esc", "q", "b":
		a.mode = modeBoard
	}
	return a, nil
}

// followCardOnDisk shows the viewed card as its board's file has it now.
// It reports false, going back to the board, when the card is gone.
func (a *App) followCardOnDisk() bool {
	rev := a.boardFileRev()
	if rev == "" || rev == a.board.diskRev {
		return true
	}
	a.loadBoard()
	if fresh := a.findCard(a.cardView.card.ID); fresh != nil {
		a.cardView.card = fresh
		return true
	}
	a.err = fmt.Errorf("card %q is no longer on the board; it was changed in its file", a.cardView.card.Title)
	a.mode = modeBoard
	return false
}

// findCard is the card with the id among the board's cards as last read.
func (a *App) findCard(id string) *model.Card {
	for _, cards := range a.board.cards {
		for _, c := range cards {
			if c.ID == id {
				return c
			}
		}
	}
	return nil
}

func (a *App) viewCardReadonly() string {
	card := a.cardView.card
	w, _ := a.size()
	// The dialog's width less its border, padding, label column and gap.
	fw := min(a.cardFormWidth(), w-2)
	const labelW = 14
	valueW := max(4, fw-4-labelW-2)
	field := func(name, value string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, formLabelStyle.Width(labelW).Align(lipgloss.Right).Render(name), "  ", value)
	}
	rows := []string{
		field("Title", lipgloss.NewStyle().Bold(true).Width(valueW).Render(card.Title)),
		field("Column", formValueStyle.Render(card.ColumnID)),
		field("Priority", priorityStyle(string(card.Priority)).Render(string(card.Priority))),
	}
	if l := labelText(card); l != "" {
		rows = append(rows, field("Labels", labelStyle.Render(l)))
	}
	if card.ExternalID != "" {
		rows = append(rows, field("External ID", formValueStyle.Render(card.ExternalID)))
	}
	return a.frame(" View Card ", " e: edit   d: archive   D: delete   Esc: back", nil, func(w, h int) string {
		if a.cardView.confirming != "" {
			verb := "Archive"
			if a.cardView.confirming == "delete" {
				verb = "Delete"
			}
			return renderCenteredConfirm(w, h, fmt.Sprintf("%s %q?", verb, truncate(card.Title, 30)))
		}
		dialogH := min(h, max(8, h*90/100))
		all := rows
		if card.Description != "" {
			desc := strings.Split(formValueStyle.Width(valueW).Render(card.Description), "\n")
			// The border and padding take 4 lines, the blank line 1.
			room := max(1, dialogH-4-len(rows)-1)
			if len(desc) > room {
				desc = append(desc[:room-1], helpStyle.Render("... (press e to see it all)"))
			}
			all = append(append([]string{}, rows...), "", field("Description", strings.Join(desc, "\n")))
		}
		// Width leaves out the border, which takes a cell on each side.
		dialog := dialogBoxStyle.Width(fw).MaxHeight(dialogH).Render(lipgloss.JoinVertical(lipgloss.Left, all...))
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
	})
}

// --- Card form (modeCardEdit) ---

type cardField int

const (
	fieldTitle cardField = iota
	fieldPriority
	fieldLabels
	fieldExternalID
	fieldDescription
	fieldCount
)

type cardModel struct {
	card     *model.Card // nil for a new card
	boardID  string
	lane     string
	returnTo mode
	field    cardField
	priority model.Priority

	titleInput      textinput.Model
	labelsInput     textinput.Model
	externalIDInput textinput.Model
	descInput       textarea.Model

	// The fields clean text as it is loaded (a tab becomes spaces, a CR a
	// line break, past 10000 lines it is cut). loaded is what each held
	// when filled, and orig the card's own text: a field that still holds
	// what it was loaded with is saved as orig, not as its cleaned text.
	loaded, orig fieldText

	confirmForce   bool // Enter again adds the card over its column's limit
	confirmDiscard bool // Esc asked whether to drop the typed changes

	formWidth int
}

type fieldText struct{ title, labels, ext, desc string }

func (c *cardModel) values() fieldText {
	return fieldText{c.titleInput.Value(), c.labelsInput.Value(), c.externalIDInput.Value(), c.descInput.Value()}
}

// fill puts a card's text in the fields.
func (c *cardModel) fill(card *model.Card) {
	c.orig = fieldText{card.Title, strings.Join(card.LabelList(), ", "), card.ExternalID, card.Description}
	c.titleInput.SetValue(c.orig.title)
	c.labelsInput.SetValue(c.orig.labels)
	c.externalIDInput.SetValue(c.orig.ext)
	c.descInput.SetValue(c.orig.desc)
	c.loaded = c.values()
}

// text is what to save for each field: the card's own text where the
// field was left alone, else what was typed.
func (c *cardModel) text() fieldText {
	now, out := c.values(), c.orig
	if now.title != c.loaded.title {
		out.title = now.title
	}
	if now.labels != c.loaded.labels {
		out.labels = now.labels
	}
	if now.ext != c.loaded.ext {
		out.ext = now.ext
	}
	if now.desc != c.loaded.desc {
		out.desc = now.desc
	}
	return out
}

// dirty reports whether anything was typed or changed in the form.
func (c *cardModel) dirty() bool {
	priority := model.PriorityMedium
	if c.card != nil {
		priority = c.card.Priority
	}
	return c.values() != c.loaded || c.priority != priority
}

func newCardModel(card *model.Card, boardID, lane string, termWidth int, returnTo mode) cardModel {
	formW := formWidth(termWidth)
	inputWidth := formW - 22

	ti := textinput.New()
	ti.Placeholder = "Card title"
	ti.Focus()
	ti.Width = inputWidth

	li := textinput.New()
	li.Placeholder = "label1, label2"
	li.Width = inputWidth

	ei := textinput.New()
	ei.Placeholder = "e.g. linear:DEV-42"
	ei.Width = inputWidth

	di := textarea.New()
	di.Placeholder = "Description..."
	di.SetWidth(inputWidth + 2)
	// No limits: a field shorter than the card's value would cut it on save.
	di.MaxHeight = 0
	di.SetHeight(8)
	di.FocusedStyle.CursorLine = lipgloss.NewStyle()
	di.BlurredStyle.CursorLine = lipgloss.NewStyle()
	di.FocusedStyle.Base = lipgloss.NewStyle()
	di.BlurredStyle.Base = lipgloss.NewStyle()

	cm := cardModel{
		boardID: boardID, lane: lane, returnTo: returnTo,
		titleInput: ti, labelsInput: li, externalIDInput: ei, descInput: di,
		formWidth: formW, priority: model.PriorityMedium,
	}
	if card != nil {
		cm.card = card
		cm.priority = card.Priority
		cm.fill(card)
	}
	return cm
}

func (c cardModel) Init() tea.Cmd { return textinput.Blink }

func (a *App) updateCard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if a.card.confirmDiscard {
			a.card.confirmDiscard = false
			if isYes(key) {
				a.mode = a.card.returnTo
			}
			return a, nil
		}
		switch key.String() {
		case "enter":
			if a.card.field != fieldDescription {
				a.saveCard()
				return a, nil
			}
		case "ctrl+s":
			a.saveCard()
			return a, nil
		case "esc":
			if a.card.dirty() {
				a.card.confirmDiscard = true
				a.feedback = "Drop what you typed? y: drop it   any other key: keep editing"
				return a, nil
			}
			a.mode = a.card.returnTo
			return a, nil
		case "tab", "shift+tab":
			step := cardField(1)
			if key.String() == "shift+tab" {
				step = fieldCount - 1
			}
			a.card.blurAll()
			a.card.field = (a.card.field + step) % fieldCount
			a.card.focusCurrent()
			return a, nil
		}
		if a.card.field == fieldPriority {
			switch key.String() {
			case "h", "left":
				a.card.priority = a.card.priority.Prev()
			case "l", "right":
				a.card.priority = a.card.priority.Next()
			}
			return a, nil
		}
	}

	var cmd tea.Cmd
	switch a.card.field {
	case fieldTitle:
		a.card.titleInput, cmd = a.card.titleInput.Update(msg)
	case fieldLabels:
		a.card.labelsInput, cmd = a.card.labelsInput.Update(msg)
	case fieldExternalID:
		a.card.externalIDInput, cmd = a.card.externalIDInput.Update(msg)
	case fieldDescription:
		a.card.descInput, cmd = a.card.descInput.Update(msg)
	}
	return a, cmd
}

func (c *cardModel) blurAll() {
	c.titleInput.Blur()
	c.labelsInput.Blur()
	c.externalIDInput.Blur()
	c.descInput.Blur()
}

func (c *cardModel) focusCurrent() {
	switch c.field {
	case fieldTitle:
		c.titleInput.Focus()
	case fieldLabels:
		c.labelsInput.Focus()
	case fieldExternalID:
		c.externalIDInput.Focus()
	case fieldDescription:
		c.descInput.Focus()
	}
}

// takeUntyped sets each field the form still holds as it was in old (one
// not typed in) to its value in fresh, the card as another change left
// it, so saving after a conflict replaces only what was typed.
func (c *cardModel) takeUntyped(old, fresh *model.Card) {
	now, loaded := c.values(), c.loaded
	if c.priority == old.Priority {
		c.priority = fresh.Priority
	}
	c.fill(fresh)
	// What was typed goes back in; it now differs from what was loaded, so
	// it is what a save writes.
	if now.title != loaded.title {
		c.titleInput.SetValue(now.title)
	}
	if now.labels != loaded.labels {
		c.labelsInput.SetValue(now.labels)
	}
	if now.ext != loaded.ext {
		c.externalIDInput.SetValue(now.ext)
	}
	if now.desc != loaded.desc {
		c.descInput.SetValue(now.desc)
	}
}

func splitLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// saveCard writes the form. A new card is written in one step, fields and
// all, so a failure never leaves half a card behind.
func (a *App) saveCard() {
	c := &a.card
	text := c.text()
	title := strings.TrimSpace(text.title)
	if title == "" && (c.card == nil || c.card.Title != "") {
		a.err = fmt.Errorf("the title cannot be empty")
		return
	}
	labels := strings.TrimSpace(text.labels)
	externalID := strings.TrimSpace(text.ext)

	var saved *model.Card
	if c.card == nil {
		force := c.confirmForce
		c.confirmForce = false
		card, err := a.db.AddCard(c.boardID, c.lane, title, fstore.CardFields{
			Description: text.desc, Priority: string(c.priority), Labels: splitLabels(labels), ExternalID: externalID,
		}, force)
		var wip *fstore.WIPLimitError
		if errors.As(err, &wip) {
			c.confirmForce = true
			a.err = fmt.Errorf("%w; Enter adds it anyway, Esc goes back", err)
			return
		}
		if a.fail(err) {
			return
		}
		saved = card
		a.feedback = fmt.Sprintf("Added %q to %s", truncate(card.Title, 30), card.ColumnID)
	} else {
		card := *c.card
		card.Title, card.Priority, card.Labels, card.ExternalID, card.Description = title, c.priority, labels, externalID, text.desc
		err := a.db.UpdateCard(&card)
		if errors.Is(err, fstore.ErrConflict) {
			// Keep what was typed, on top of the card as it is now: saving
			// again is the choice to replace the change made elsewhere.
			a.loadBoard()
			if fresh := a.findCard(c.card.ID); fresh != nil {
				c.takeUntyped(c.card, fresh)
				c.card = fresh
				a.cardView.card = fresh
				a.err = fmt.Errorf("the card changed on disk; save again to keep your edits, or Esc to drop them")
				return
			}
			err = fstore.ErrNotFound
		}
		if errors.Is(err, fstore.ErrNotFound) {
			// The card is gone from the file (deleted, or a card with no
			// ^id renamed elsewhere, which gives it another id). What was
			// typed is not lost: it can be added as a new card.
			a.loadBoard()
			c.card = nil
			c.returnTo = modeBoard
			a.err = fmt.Errorf("this card is no longer on the board (its file changed); Enter adds your text as a new card, Esc drops it")
			return
		}
		if a.fail(err) {
			return
		}
		saved = &card
		a.feedback = fmt.Sprintf("Saved %q", truncate(card.Title, 30))
	}
	a.mode = c.returnTo
	a.loadBoard()
	if a.mode == modeCardView {
		a.cardView.card = saved
	}
	for i, card := range a.focusedCards() {
		if card.ID == saved.ID {
			a.board.focusCard = i
		}
	}
}

func (a *App) viewCard() string {
	c := &a.card
	header := " New Card "
	if c.card != nil {
		header = " Edit Card "
	}
	const labelW = 14
	field := func(name string, active bool, value string) string {
		style := formLabelStyle
		if active {
			style = formLabelActiveStyle
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, style.Width(labelW).Align(lipgloss.Right).Render(name), "  ", value)
	}
	prio := priorityStyle(string(c.priority)).Render(string(c.priority))
	if c.field == fieldPriority {
		prio += helpStyle.Render("  < h/l >")
	}
	hints := " Tab/Shift+Tab: fields   h/l: priority   Enter: save (Ctrl+S in the description)   Esc: cancel"
	return a.frame(header, hints, nil, func(w, h int) string {
		dialogH := min(h, max(12, h*90/100))
		// The border and padding take 4 lines, the fields 5, the blank 1.
		c.descInput.SetHeight(max(3, dialogH-10))
		// The inputs fit the dialog: its width less the border, padding,
		// label column and gap.
		dialogW := min(c.formWidth, w-2)
		inputW := max(4, dialogW-4-labelW-3)
		c.titleInput.Width, c.labelsInput.Width, c.externalIDInput.Width = inputW, inputW, inputW
		c.descInput.SetWidth(inputW + 2)
		form := lipgloss.JoinVertical(lipgloss.Left,
			field("Title", c.field == fieldTitle, c.titleInput.View()),
			field("Column", false, formValueStyle.Render(c.lane)),
			field("Priority", c.field == fieldPriority, prio),
			field("Labels", c.field == fieldLabels, c.labelsInput.View()),
			field("External ID", c.field == fieldExternalID, c.externalIDInput.View()),
			"",
			field("Description", c.field == fieldDescription, c.descInput.View()),
		)
		dialog := dialogBoxStyle.Width(dialogW).MaxHeight(dialogH).Render(form)
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
	})
}
