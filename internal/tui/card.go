package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

type cardViewModel struct {
	card       *model.Card
	colName    string
	formWidth  int
	confirming string
}

func (a *App) updateCardView(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cardArchivedMsg, cardDeletedMsg:
		a.mode = modeBoard
		return a, a.loadBoard()

	case errMsg:
		a.board.err = msg.err
		a.mode = modeBoard
		return a, nil

	case tea.KeyMsg:
		if a.cardView.confirming != "" {
			return a.updateCardViewConfirming(msg)
		}

		switch msg.String() {
		case "e":
			return a, a.editSelectedCard()
		case "d":
			a.cardView.confirming = "archive"
		case "D":
			a.cardView.confirming = "delete"
		case "esc", "q":
			a.mode = modeBoard
			return a, nil
		}
	}
	return a, nil
}

func (a *App) updateCardViewConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := a.cardView.confirming
		a.cardView.confirming = ""
		card := a.cardView.card
		if card == nil {
			return a, nil
		}
		switch action {
		case "archive":
			return a, func() tea.Msg {
				if err := a.db.ArchiveCard(card.ID); err != nil {
					return errMsg{err}
				}
				return cardArchivedMsg{}
			}
		case "delete":
			return a, func() tea.Msg {
				if err := a.db.DeleteCard(card.ID); err != nil {
					return errMsg{err}
				}
				return cardDeletedMsg{}
			}
		}
	default:
		a.cardView.confirming = ""
	}
	return a, nil
}

func (a *App) viewCardReadonly() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	card := a.cardView.card
	fw := a.cardView.formWidth
	labelW := 14

	titleBar := titleBarStyle.Width(w).Render(" View Card ")
	statusBar := statusBarStyle.Width(w).Render(" e: edit   d: archive   D: delete   Esc: back")

	fieldLabel := func(name string) string {
		return formLabelStyle.Width(labelW).Align(lipgloss.Right).Render(name)
	}

	var rows []string

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Title"),
			"  ",
			lipgloss.NewStyle().Bold(true).Render(card.Title),
		))

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Column"),
			"  ",
			formValueStyle.Render(a.cardView.colName),
		))

	pStyle := priorityStyle(string(card.Priority))
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Priority"),
			"  ",
			pStyle.Render(string(card.Priority)),
		))

	if card.Labels != "" {
		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top,
				fieldLabel("Labels"),
				"  ",
				labelStyle.Render(card.Labels),
			))
	}

	if card.ExternalID != "" {
		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top,
				fieldLabel("External ID"),
				"  ",
				formValueStyle.Render(card.ExternalID),
			))
	}

	created := card.CreatedAt.Format("02 Jan 2006")
	updated := card.UpdatedAt.Format("02 Jan 2006")
	rows = append(rows, "")
	meta := helpStyle.Render(fmt.Sprintf("Created: %s   Updated: %s", created, updated))
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(labelW).Render(""),
			"  ",
			meta,
		))

	dialogH := h * 80 / 100

	if card.Description != "" {
		rows = append(rows, "")
		descWidth := fw - labelW - 4
		rendered := formValueStyle.Width(descWidth).Render(card.Description)
		descLines := strings.Split(rendered, "\n")

		// border(2) + padding(2) + current rows + blank before desc
		overhead := 4 + len(rows) + 1
		maxDescLines := dialogH - overhead
		if maxDescLines < 1 {
			maxDescLines = 1
		}
		if len(descLines) > maxDescLines {
			descLines = append(descLines[:maxDescLines-1], helpStyle.Render("... (truncated)"))
		}

		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top,
				fieldLabel("Description"),
				"  ",
				strings.Join(descLines, "\n"),
			))
	}

	form := lipgloss.JoinVertical(lipgloss.Left, rows...)
	dialog := dialogBoxStyle.Width(fw).Height(dialogH).Render(form)

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1

	var content string
	if a.cardView.confirming != "" {
		content = a.renderCardViewConfirmDialog(w, contentHeight)
	} else {
		content = lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)
	}

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}

func (a *App) renderCardViewConfirmDialog(totalWidth, contentHeight int) string {
	card := a.cardView.card
	if card == nil {
		return renderCenteredConfirm(totalWidth, contentHeight, "No card selected")
	}

	verb := a.cardView.confirming
	if len(verb) > 0 {
		verb = strings.ToUpper(verb[:1]) + verb[1:]
	}
	prompt := fmt.Sprintf("%s %q?", verb, truncate(card.Title, 30))

	return renderCenteredConfirm(totalWidth, contentHeight, prompt)
}

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
	card       *model.Card
	isNew      bool
	columnID   string
	columns    []*model.Column
	colIndex   int
	field      cardField
	priority   model.Priority

	titleInput      textinput.Model
	labelsInput     textinput.Model
	externalIDInput textinput.Model
	descInput       textarea.Model

	formWidth int
	err       error
}

type cardSavedMsg struct {
	card *model.Card
}

func newCardModel(card *model.Card, columnID string, columns []*model.Column, termWidth int) cardModel {
	formW := termWidth * 80 / 100
	if formW > 100 {
		formW = 100
	}
	if formW < 50 {
		formW = 50
	}

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

	colIdx := 0
	for i, col := range columns {
		if col.ID == columnID {
			colIdx = i
			break
		}
	}

	cm := cardModel{
		columnID:        columnID,
		columns:         columns,
		colIndex:        colIdx,
		field:           fieldTitle,
		priority:        model.PriorityMedium,
		titleInput:      ti,
		labelsInput:     li,
		externalIDInput: ei,
		descInput:       di,
		formWidth:       formW,
		isNew:           card == nil,
	}

	if card != nil {
		cm.card = card
		cm.priority = card.Priority
		cm.titleInput.SetValue(card.Title)
		cm.labelsInput.SetValue(card.Labels)
		cm.externalIDInput.SetValue(card.ExternalID)
		cm.descInput.SetValue(card.Description)

		for i, col := range columns {
			if col.ID == card.ColumnID {
				cm.colIndex = i
				break
			}
		}
	}

	return cm
}

func (c cardModel) Init() tea.Cmd {
	return textinput.Blink
}

func (a *App) updateCard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cardSavedMsg:
		a.mode = modeBoard
		return a, a.loadBoard()

	case errMsg:
		a.card.err = msg.err
		return a, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if a.card.field != fieldDescription {
				return a, a.saveCard()
			}
		case "esc":
			a.mode = modeBoard
			return a, nil
		case "tab":
			a.card.blurAll()
			a.card.field = (a.card.field + 1) % fieldCount
			a.card.focusCurrent()
			return a, nil
		case "shift+tab":
			a.card.blurAll()
			a.card.field = (a.card.field - 1 + fieldCount) % fieldCount
			a.card.focusCurrent()
			return a, nil
		}

		if a.card.field == fieldPriority {
			switch msg.String() {
			case "h", "left":
				a.card.priority = a.card.priority.Prev()
				return a, nil
			case "l", "right":
				a.card.priority = a.card.priority.Next()
				return a, nil
			}
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

func (a *App) saveCard() tea.Cmd {
	title := strings.TrimSpace(a.card.titleInput.Value())
	if title == "" {
		a.card.err = fmt.Errorf("title cannot be empty")
		return nil
	}

	columnID := a.card.columnID
	if a.card.colIndex < len(a.card.columns) {
		columnID = a.card.columns[a.card.colIndex].ID
	}

	isNew := a.card.isNew
	priority := a.card.priority
	labels := strings.TrimSpace(a.card.labelsInput.Value())
	externalID := strings.TrimSpace(a.card.externalIDInput.Value())
	description := a.card.descInput.Value()

	if !isNew {
		card := *a.card.card
		card.Title = title
		card.ColumnID = columnID
		card.Priority = priority
		card.Labels = labels
		card.ExternalID = externalID
		card.Description = description

		return func() tea.Msg {
			if err := a.db.UpdateCard(&card); err != nil {
				return errMsg{err}
			}
			return cardSavedMsg{&card}
		}
	}

	return func() tea.Msg {
		card, err := a.db.CreateCard(columnID, title, priority)
		if err != nil {
			return errMsg{err}
		}
		card.Labels = labels
		card.ExternalID = externalID
		card.Description = description
		if err := a.db.UpdateCard(card); err != nil {
			return errMsg{err}
		}
		return cardSavedMsg{card}
	}
}

func (a *App) viewCard() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	header := "New Card"
	if !a.card.isNew {
		header = "Edit Card"
	}

	titleBar := titleBarStyle.Width(w).Render(fmt.Sprintf(" %s ", header))
	statusBar := statusBarStyle.Width(w).Render(
		" Tab/Shift+Tab: fields   h/l: cycle priority   Enter: save   Esc: cancel")

	fw := a.card.formWidth
	labelW := 14

	fieldLabel := func(name string, active bool) string {
		style := formLabelStyle.Width(labelW).Align(lipgloss.Right)
		if active {
			style = formLabelActiveStyle.Width(labelW).Align(lipgloss.Right)
		}
		return style.Render(name)
	}

	var rows []string

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Title", a.card.field == fieldTitle),
			"  ",
			a.card.titleInput.View(),
		))

	colName := ""
	if a.card.colIndex < len(a.card.columns) {
		colName = a.card.columns[a.card.colIndex].Name
	}
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Column", false),
			"  ",
			formValueStyle.Render(colName),
		))

	active := a.card.field == fieldPriority
	pStyle := priorityStyle(string(a.card.priority))
	prioValue := pStyle.Render(string(a.card.priority))
	if active {
		prioValue += helpStyle.Render("  < h/l >")
	}
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Priority", active),
			"  ",
			prioValue,
		))

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Labels", a.card.field == fieldLabels),
			"  ",
			a.card.labelsInput.View(),
		))

	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("External ID", a.card.field == fieldExternalID),
			"  ",
			a.card.externalIDInput.View(),
		))

	dialogH := h * 80 / 100
	// border(2) + padding(2) + fields(5) + blank(1) = 10 lines overhead
	descHeight := dialogH - 10
	if a.card.card != nil {
		descHeight -= 2 // blank + metadata
	}
	if a.card.err != nil {
		descHeight -= 2 // blank + error
	}
	if descHeight < 4 {
		descHeight = 4
	}
	a.card.descInput.SetHeight(descHeight)

	rows = append(rows, "")
	rows = append(rows,
		lipgloss.JoinHorizontal(lipgloss.Top,
			fieldLabel("Description", a.card.field == fieldDescription),
			"  ",
			a.card.descInput.View(),
		))

	if a.card.card != nil {
		created := a.card.card.CreatedAt.Format("02 Jan 2006")
		updated := a.card.card.UpdatedAt.Format("02 Jan 2006")
		rows = append(rows, "")
		meta := helpStyle.Render(fmt.Sprintf("Created: %s   Updated: %s", created, updated))
		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top,
				lipgloss.NewStyle().Width(labelW).Render(""),
				"  ",
				meta,
			))
	}

	if a.card.err != nil {
		rows = append(rows, "")
		rows = append(rows,
			lipgloss.JoinHorizontal(lipgloss.Top,
				lipgloss.NewStyle().Width(labelW).Render(""),
				"  ",
				errorStyle.Render(fmt.Sprintf("! %s", a.card.err)),
			))
	}

	form := lipgloss.JoinVertical(lipgloss.Left, rows...)

	dialog := dialogBoxStyle.Width(fw).Height(dialogH).Render(form)

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}
