package tui

import (
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

func (a *App) viewCardReadonly() string {
	card := a.cardView.card
	fw := a.cardFormWidth()
	const labelW = 14
	field := func(name, value string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, formLabelStyle.Width(labelW).Align(lipgloss.Right).Render(name), "  ", value)
	}
	rows := []string{
		field("Title", lipgloss.NewStyle().Bold(true).Render(card.Title)),
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
		dialogH := max(8, h*90/100)
		all := rows
		if card.Description != "" {
			desc := strings.Split(formValueStyle.Width(fw-labelW-6).Render(card.Description), "\n")
			// The border and padding take 4 lines, the blank line 1.
			room := max(1, dialogH-4-len(rows)-1)
			if len(desc) > room {
				desc = append(desc[:room-1], helpStyle.Render("... (press e to see it all)"))
			}
			all = append(append([]string{}, rows...), "", field("Description", strings.Join(desc, "\n")))
		}
		dialog := dialogBoxStyle.Width(min(fw, w)).MaxHeight(dialogH).Render(lipgloss.JoinVertical(lipgloss.Left, all...))
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

	formWidth int
}

func newCardModel(card *model.Card, boardID, lane string, termWidth int, returnTo mode) cardModel {
	formW := max(50, min(termWidth*80/100, 100))
	inputWidth := formW - 22

	ti := textinput.New()
	ti.Placeholder = "Card title"
	ti.Focus()
	ti.CharLimit = 200
	ti.Width = inputWidth

	li := textinput.New()
	li.Placeholder = "label1, label2"
	li.CharLimit = 200
	li.Width = inputWidth

	ei := textinput.New()
	ei.Placeholder = "e.g. linear:DEV-42"
	ei.CharLimit = 100
	ei.Width = inputWidth

	di := textarea.New()
	di.Placeholder = "Description..."
	di.SetWidth(inputWidth + 2)
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
		cm.titleInput.SetValue(card.Title)
		cm.labelsInput.SetValue(strings.Join(card.LabelList(), ", "))
		cm.externalIDInput.SetValue(card.ExternalID)
		cm.descInput.SetValue(card.Description)
	}
	return cm
}

func (c cardModel) Init() tea.Cmd { return textinput.Blink }

func (a *App) updateCard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
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
	title := strings.TrimSpace(c.titleInput.Value())
	if title == "" {
		a.err = fmt.Errorf("the title cannot be empty")
		return
	}
	labels := strings.TrimSpace(c.labelsInput.Value())
	externalID := strings.TrimSpace(c.externalIDInput.Value())
	description := c.descInput.Value()

	var saved *model.Card
	if c.card == nil {
		card, err := a.db.AddCard(c.boardID, c.lane, title, fstore.CardFields{
			Description: description, Priority: string(c.priority), Labels: splitLabels(labels), ExternalID: externalID,
		}, false)
		if a.fail(err) {
			return
		}
		saved = card
		a.feedback = fmt.Sprintf("Added %q to %s", truncate(card.Title, 30), card.ColumnID)
	} else {
		card := *c.card
		card.Title, card.Priority, card.Labels, card.ExternalID, card.Description = title, c.priority, labels, externalID, description
		if a.fail(a.db.UpdateCard(&card)) {
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
		dialogH := max(12, h*90/100)
		// The border and padding take 4 lines, the fields 5, the blank 1.
		c.descInput.SetHeight(max(3, dialogH-10))
		form := lipgloss.JoinVertical(lipgloss.Left,
			field("Title", c.field == fieldTitle, c.titleInput.View()),
			field("Column", false, formValueStyle.Render(c.lane)),
			field("Priority", c.field == fieldPriority, prio),
			field("Labels", c.field == fieldLabels, c.labelsInput.View()),
			field("External ID", c.field == fieldExternalID, c.externalIDInput.View()),
			"",
			field("Description", c.field == fieldDescription, c.descInput.View()),
		)
		dialog := dialogBoxStyle.Width(min(c.formWidth, w)).MaxHeight(dialogH).Render(form)
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, dialog)
	})
}
