package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/model"
)

type boardModel struct {
	board       *model.Board
	columns     []*model.Column
	cards       map[string][]*model.Card
	focusCol    int
	focusCard   int
	scrollCol   int
	filter      string
	filterInput string
	filtering   bool
	confirming   string
	moving       bool
	moveOrigCol  int
	moveOrigCard int
	moveCard     *model.Card
	showHelp     bool
	err          error
	feedback     string
}

type boardLoadedMsg struct {
	columns []*model.Column
	cards   map[string][]*model.Card
}

type cardMovedMsg struct{}
type cardArchivedMsg struct{}
type cardDeletedMsg struct{}

func (a *App) loadBoard() tea.Cmd {
	return func() tea.Msg {
		columns, err := a.db.ListColumns(a.board.board.ID)
		if err != nil {
			return errMsg{err}
		}

		cards := make(map[string][]*model.Card)
		for _, col := range columns {
			colCards, err := a.db.ListCards(col.ID)
			if err != nil {
				return errMsg{err}
			}
			cards[col.ID] = colCards
		}

		return boardLoadedMsg{columns: columns, cards: cards}
	}
}

func (a *App) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case boardLoadedMsg:
		a.board.columns = msg.columns
		a.board.cards = msg.cards
		a.board.err = nil
		a.clampCardSelection()
		a.adjustScroll()

	case cardMovedMsg:
		a.board.feedback = "Card moved"
		return a, a.loadBoard()
	case cardArchivedMsg:
		a.board.feedback = "Card archived"
		return a, a.loadBoard()
	case cardDeletedMsg:
		a.board.feedback = "Card deleted"
		return a, a.loadBoard()

	case errMsg:
		a.board.err = msg.err

	case tea.KeyMsg:
		a.board.feedback = ""

		if a.board.filtering {
			return a.updateBoardFiltering(msg)
		}

		if a.board.confirming != "" {
			return a.updateBoardConfirming(msg)
		}

		if a.board.moving {
			return a.updateBoardMoving(msg)
		}

		if a.board.showHelp {
			a.board.showHelp = false
			return a, nil
		}

		switch msg.String() {
		case "j", "down":
			a.moveSelectionDown()
		case "k", "up":
			a.moveSelectionUp()
		case "h", "left":
			a.focusColumnLeft()
		case "l", "right":
			a.focusColumnRight()
		case "H":
			a.startMoveMode(-1, 0)
		case "L":
			a.startMoveMode(1, 0)
		case "J":
			a.startMoveMode(0, 1)
		case "K":
			a.startMoveMode(0, -1)
		case "n":
			return a, a.newCardInCurrentColumn()
		case "enter":
			return a, a.viewSelectedCard()
		case "e":
			return a, a.editSelectedCard()
		case "d":
			if a.selectedCard() != nil {
				a.board.confirming = "archive"
			}
		case "D":
			if a.selectedCard() != nil {
				a.board.confirming = "delete"
			}
		case "esc":
			if a.board.filter != "" {
				a.board.filter = ""
				a.board.focusCard = 0
				a.clampCardSelection()
			}
		case "/":
			a.board.filtering = true
			a.board.filterInput = ""
		case "1":
			a.togglePriorityFilter("urgent")
		case "2":
			a.togglePriorityFilter("high")
		case "3":
			a.togglePriorityFilter("medium")
		case "4":
			a.togglePriorityFilter("low")
		case "b":
			a.mode = modePicker
			return a, a.initPicker()
		case "?":
			a.board.showHelp = !a.board.showHelp
		case "q":
			return a, tea.Quit
		}
	}

	return a, nil
}

func (a *App) startMoveMode(colDir, cardDir int) {
	card := a.selectedCard()
	if card == nil || len(a.board.columns) == 0 {
		return
	}

	targetCol := a.board.focusCol + colDir
	if targetCol < 0 || targetCol >= len(a.board.columns) {
		return
	}

	a.board.moving = true
	a.board.moveOrigCol = a.board.focusCol
	a.board.moveOrigCard = a.board.focusCard
	a.board.moveCard = card

	if colDir != 0 {
		a.board.focusCol = targetCol
		col := a.board.columns[a.board.focusCol]
		cards := a.cardsForDisplay(col.ID)
		if a.board.focusCard > len(cards) {
			a.board.focusCard = len(cards)
		}
		a.adjustScroll()
	}

	if cardDir != 0 {
		col := a.board.columns[a.board.focusCol]
		cards := a.cardsForDisplay(col.ID)
		target := a.board.focusCard + cardDir
		if target < 0 || target >= len(cards) {
			a.board.moving = false
			a.board.moveCard = nil
			return
		}
		a.board.focusCard = target
	}
}

func (a *App) updateBoardMoving(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "h", "H", "left":
		if a.board.focusCol > 0 {
			a.board.focusCol--
			col := a.board.columns[a.board.focusCol]
			cards := a.cardsForDisplay(col.ID)
			if a.board.focusCard > len(cards) {
				a.board.focusCard = len(cards)
			}
			a.adjustScroll()
		}
	case "l", "L", "right":
		if a.board.focusCol < len(a.board.columns)-1 {
			a.board.focusCol++
			col := a.board.columns[a.board.focusCol]
			cards := a.cardsForDisplay(col.ID)
			if a.board.focusCard > len(cards) {
				a.board.focusCard = len(cards)
			}
			a.adjustScroll()
		}
	case "j", "J", "down":
		col := a.board.columns[a.board.focusCol]
		cards := a.cardsForDisplay(col.ID)
		if a.board.focusCard < len(cards)-1 {
			a.board.focusCard++
		}
	case "k", "K", "up":
		if a.board.focusCard > 0 {
			a.board.focusCard--
		}
	case "enter":
		if a.board.focusCol == a.board.moveOrigCol && a.board.focusCard == a.board.moveOrigCard {
			a.cancelMoving()
			return a, nil
		}
		a.board.confirming = "move"
	case "esc":
		a.cancelMoving()
	}
	return a, nil
}

func (a *App) cancelMoving() {
	a.board.focusCol = a.board.moveOrigCol
	a.board.focusCard = a.board.moveOrigCard
	a.board.moving = false
	a.board.moveCard = nil
	a.adjustScroll()
}

func (a *App) updateBoardFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		a.board.filter = strings.TrimSpace(a.board.filterInput)
		a.board.filtering = false
	case "esc":
		a.board.filtering = false
		a.board.filter = ""
	case "backspace":
		if len(a.board.filterInput) > 0 {
			a.board.filterInput = a.board.filterInput[:len(a.board.filterInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			a.board.filterInput += msg.String()
		}
	}
	return a, nil
}

func (a *App) updateBoardConfirming(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := a.board.confirming
		a.board.confirming = ""
		switch action {
		case "archive":
			card := a.selectedCard()
			if card == nil {
				return a, nil
			}
			return a, func() tea.Msg {
				if err := a.db.ArchiveCard(card.ID); err != nil {
					return errMsg{err}
				}
				return cardArchivedMsg{}
			}
		case "delete":
			card := a.selectedCard()
			if card == nil {
				return a, nil
			}
			return a, func() tea.Msg {
				if err := a.db.DeleteCard(card.ID); err != nil {
					return errMsg{err}
				}
				return cardDeletedMsg{}
			}
		case "move":
			return a, a.commitCardMove()
		}
	default:
		a.board.confirming = ""
		if a.board.moving {
			a.cancelMoving()
		}
	}
	return a, nil
}

func (a *App) filteredCards(columnID string) []*model.Card {
	cards := a.board.cards[columnID]
	if a.board.filter == "" {
		return cards
	}

	filter := strings.ToLower(a.board.filter)
	var result []*model.Card
	for _, card := range cards {
		if strings.Contains(strings.ToLower(card.Title), filter) ||
			strings.Contains(strings.ToLower(card.Description), filter) ||
			strings.Contains(strings.ToLower(string(card.Priority)), filter) ||
			card.HasLabel(a.board.filter) {
			result = append(result, card)
		}
	}
	return result
}

func (a *App) totalFilteredCardCount() int {
	filter := strings.ToLower(a.board.filter)
	count := 0
	for _, cards := range a.board.cards {
		for _, card := range cards {
			if strings.Contains(strings.ToLower(card.Title), filter) ||
				strings.Contains(strings.ToLower(card.Description), filter) ||
				strings.Contains(strings.ToLower(string(card.Priority)), filter) ||
				card.HasLabel(a.board.filter) {
				count++
			}
		}
	}
	return count
}

func (a *App) cardsForDisplay(columnID string) []*model.Card {
	cards := a.filteredCards(columnID)

	if !a.board.moving || a.board.moveCard == nil {
		return cards
	}

	origCol := a.board.columns[a.board.moveOrigCol]
	targetCol := a.board.columns[a.board.focusCol]

	if columnID != origCol.ID && columnID != targetCol.ID {
		return cards
	}

	// Remove the moving card from the origin column's list
	without := make([]*model.Card, 0, len(cards))
	for _, c := range cards {
		if c.ID != a.board.moveCard.ID {
			without = append(without, c)
		}
	}

	if columnID == origCol.ID && columnID != targetCol.ID {
		return without
	}

	idx := a.board.focusCard
	if idx > len(without) {
		idx = len(without)
	}
	result := make([]*model.Card, 0, len(without)+1)
	result = append(result, without[:idx]...)
	result = append(result, a.board.moveCard)
	result = append(result, without[idx:]...)
	return result
}

func (a *App) selectedCard() *model.Card {
	if a.board.moving && a.board.moveCard != nil {
		return a.board.moveCard
	}
	if len(a.board.columns) == 0 {
		return nil
	}
	col := a.board.columns[a.board.focusCol]
	cards := a.filteredCards(col.ID)
	if a.board.focusCard >= len(cards) || a.board.focusCard < 0 {
		return nil
	}
	return cards[a.board.focusCard]
}

func (a *App) moveSelectionDown() {
	if len(a.board.columns) == 0 {
		return
	}
	col := a.board.columns[a.board.focusCol]
	cards := a.filteredCards(col.ID)
	if a.board.focusCard < len(cards)-1 {
		a.board.focusCard++
	}
}

func (a *App) moveSelectionUp() {
	if a.board.focusCard > 0 {
		a.board.focusCard--
	}
}

func (a *App) focusColumnLeft() {
	if a.board.focusCol > 0 {
		a.board.focusCol--
		a.clampCardSelection()
		a.adjustScroll()
	}
}

func (a *App) focusColumnRight() {
	if a.board.focusCol < len(a.board.columns)-1 {
		a.board.focusCol++
		a.clampCardSelection()
		a.adjustScroll()
	}
}

func (a *App) clampCardSelection() {
	if len(a.board.columns) == 0 {
		return
	}
	col := a.board.columns[a.board.focusCol]
	cards := a.filteredCards(col.ID)
	if a.board.focusCard >= len(cards) {
		a.board.focusCard = max(0, len(cards)-1)
	}
}

func (a *App) visibleColumnCount() int {
	numCols := len(a.board.columns)
	if numCols == 0 {
		return 0
	}
	w := a.width
	if w == 0 {
		w = 80
	}
	visible := w / 24
	return max(1, min(visible, numCols))
}

func (a *App) adjustScroll() {
	visible := a.visibleColumnCount()
	if visible == 0 {
		return
	}
	maxScroll := len(a.board.columns) - visible
	a.board.scrollCol = max(0, min(a.board.scrollCol, maxScroll))

	if a.board.focusCol < a.board.scrollCol {
		a.board.scrollCol = a.board.focusCol
	}
	if a.board.focusCol >= a.board.scrollCol+visible {
		a.board.scrollCol = a.board.focusCol - visible + 1
	}
}

func (a *App) commitCardMove() tea.Cmd {
	card := a.board.moveCard
	targetColIdx := a.board.focusCol
	origColIdx := a.board.moveOrigCol

	if card == nil || len(a.board.columns) == 0 {
		a.board.moving = false
		a.board.moveCard = nil
		return nil
	}

	targetCol := a.board.columns[targetColIdx]
	sameColumn := targetColIdx == origColIdx

	if !sameColumn && targetCol.WIPLimit != nil {
		count, err := a.db.CountCardsInColumn(targetCol.ID)
		if err != nil {
			a.board.err = fmt.Errorf("checking WIP limit: %w", err)
			a.cancelMoving()
			return nil
		}
		if count >= *targetCol.WIPLimit {
			a.board.err = fmt.Errorf("column %s is at WIP limit", targetCol.Name)
			a.cancelMoving()
			return nil
		}
	}

	displayCards := a.cardsForDisplay(targetCol.ID)
	cardIDs := make([]string, len(displayCards))
	for i, c := range displayCards {
		cardIDs[i] = c.ID
	}

	a.board.moving = false
	a.board.moveCard = nil

	return func() tea.Msg {
		if !sameColumn {
			if err := a.db.MoveCard(card.ID, targetCol.ID); err != nil {
				return errMsg{err}
			}
		}
		if err := a.db.ReorderCardsInColumn(targetCol.ID, cardIDs); err != nil {
			return errMsg{err}
		}
		return cardMovedMsg{}
	}
}

func (a *App) newCardInCurrentColumn() tea.Cmd {
	if len(a.board.columns) == 0 {
		return nil
	}
	col := a.board.columns[a.board.focusCol]
	a.mode = modeCardEdit
	a.card = newCardModel(nil, col.ID, a.board.columns, a.width)
	return a.card.Init()
}

func (a *App) viewSelectedCard() tea.Cmd {
	card := a.selectedCard()
	if card == nil {
		return nil
	}
	col := a.board.columns[a.board.focusCol]
	a.cardView = cardViewModel{
		card:      card,
		colName:   col.Name,
		formWidth: a.cardFormWidth(),
	}
	a.mode = modeCardView
	return nil
}

func (a *App) editSelectedCard() tea.Cmd {
	card := a.selectedCard()
	if card == nil {
		return nil
	}
	a.mode = modeCardEdit
	a.card = newCardModel(card, card.ColumnID, a.board.columns, a.width)
	return a.card.Init()
}

func (a *App) cardFormWidth() int {
	return max(50, min(a.width*80/100, 100))
}

func (a *App) togglePriorityFilter(priority string) {
	if a.board.filter == priority {
		a.board.filter = ""
	} else {
		a.board.filter = priority
	}
}

func (a *App) viewBoard() string {
	if a.board.showHelp {
		return a.viewBoardHelp()
	}

	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	totalCards := 0
	for _, cards := range a.board.cards {
		totalCards += len(cards)
	}
	doneCards := 0
	if len(a.board.columns) > 0 {
		lastCol := a.board.columns[len(a.board.columns)-1]
		doneCards = len(a.board.cards[lastCol.ID])
	}

	titleText := fmt.Sprintf(" kb: %s ", a.board.board.Name)
	if totalCards > 0 {
		bar := progressBar(doneCards, totalCards, 10)
		titleText = fmt.Sprintf(" kb: %s  %s %d/%d ",
			a.board.board.Name, bar, doneCards, totalCards)
	}
	titleBar := titleBarStyle.Width(w).Render(titleText)

	statusText := " hjkl: navigate   HJKL: move/reorder   n: new   Enter: view   e: edit   d: archive   b: boards   ?: help   q: quit"
	if a.board.moving && a.board.confirming == "" {
		statusText = fmt.Sprintf(" Moving %q — h/l: column  j/k: position  Enter: confirm  Esc: cancel",
			truncate(a.board.moveCard.Title, 25))
	}
	statusBar := statusBarStyle.Width(w).Render(statusText)

	filterBar := ""
	if a.board.filtering {
		filterBar = filterBarStyle.Render(fmt.Sprintf(" / %s", a.board.filterInput)) + "█"
	} else if a.board.filter != "" {
		filteredCount := a.totalFilteredCardCount()
		filterBar = filterBarStyle.Render(fmt.Sprintf(" filter: %s (%d cards)", a.board.filter, filteredCount)) +
			helpStyle.Render("  (/ to change, esc clears)")
	}

	errBar := ""
	if a.board.err != nil {
		errBar = errorStyle.Render(fmt.Sprintf(" Error: %s", a.board.err))
	} else if a.board.feedback != "" {
		errBar = helpStyle.Render(fmt.Sprintf(" %s", a.board.feedback))
	}

	scrollBar := ""
	numCols := len(a.board.columns)
	visibleCols := a.visibleColumnCount()
	if numCols > 0 && visibleCols < numCols {
		var leftIndicator, rightIndicator string
		if a.board.scrollCol > 0 {
			leftIndicator = fmt.Sprintf("< %d more", a.board.scrollCol)
		}
		if hiddenRight := numCols - a.board.scrollCol - visibleCols; hiddenRight > 0 {
			rightIndicator = fmt.Sprintf("%d more >", hiddenRight)
		}
		padding := max(1, w-len(leftIndicator)-len(rightIndicator))
		scrollBar = helpStyle.Render(leftIndicator + strings.Repeat(" ", padding) + rightIndicator)
	}

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	if scrollBar != "" {
		contentHeight--
	}
	if filterBar != "" {
		contentHeight--
	}
	if errBar != "" {
		contentHeight--
	}

	var columnContent string
	if a.board.confirming != "" {
		columnContent = a.renderConfirmDialog(w, contentHeight)
	} else {
		columnContent = a.renderColumns(w, contentHeight)
	}

	var sections []string
	sections = append(sections, titleBar)
	if scrollBar != "" {
		sections = append(sections, scrollBar)
	}
	sections = append(sections, columnContent)
	if errBar != "" {
		sections = append(sections, errBar)
	}
	if filterBar != "" {
		sections = append(sections, filterBar)
	}
	sections = append(sections, statusBar)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (a *App) renderConfirmDialog(totalWidth, contentHeight int) string {
	card := a.selectedCard()
	if card == nil {
		return renderCenteredConfirm(totalWidth, contentHeight, "No card selected")
	}

	var prompt string
	switch a.board.confirming {
	case "move":
		origCol := a.board.columns[a.board.moveOrigCol]
		targetCol := a.board.columns[a.board.focusCol]
		if origCol.ID == targetCol.ID {
			prompt = fmt.Sprintf("Reorder %q in %s?", truncate(card.Title, 25), targetCol.Name)
		} else {
			prompt = fmt.Sprintf("Move %q from %s to %s?", truncate(card.Title, 20), origCol.Name, targetCol.Name)
		}
	default:
		verb := a.board.confirming
		if len(verb) > 0 {
			verb = strings.ToUpper(verb[:1]) + verb[1:]
		}
		prompt = fmt.Sprintf("%s %q?", verb, truncate(card.Title, 30))
	}

	return renderCenteredConfirm(totalWidth, contentHeight, prompt)
}

func (a *App) renderColumns(totalWidth, maxHeight int) string {
	numCols := len(a.board.columns)
	if numCols == 0 {
		msg := lipgloss.Place(totalWidth, maxHeight, lipgloss.Center, lipgloss.Center,
			helpStyle.Render("No columns found."))
		return msg
	}

	startCol := a.board.scrollCol
	endCol := min(startCol+a.visibleColumnCount(), numCols)
	displayCount := endCol - startCol
	availableWidth := totalWidth - (displayCount-1)
	colWidth := max(availableWidth/displayCount, 16)

	var renderedCols []string
	for i := startCol; i < endCol; i++ {
		renderedCols = append(renderedCols, a.renderSingleColumn(a.board.columns[i], i, colWidth, maxHeight))
	}

	divStyle := lipgloss.NewStyle().
		Faint(true).
		Height(maxHeight)

	divStr := divStyle.Render(strings.Repeat("│\n", maxHeight-1) + "│")

	var parts []string
	for i, col := range renderedCols {
		parts = append(parts, col)
		if i < len(renderedCols)-1 {
			parts = append(parts, divStr)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (a *App) renderSingleColumn(col *model.Column, colIdx, width, maxHeight int) string {
	cards := a.cardsForDisplay(col.ID)

	countStr := fmt.Sprintf("%d", len(cards))
	if col.WIPLimit != nil {
		countStr = fmt.Sprintf("%d/%d", len(cards), *col.WIPLimit)
	}

	headerText := fmt.Sprintf("%s (%s)", col.Name, countStr)

	hStyle := columnHeaderStyle
	if colIdx == a.board.focusCol {
		hStyle = columnHeaderActiveStyle
	}
	if col.WIPLimit != nil && len(cards) >= *col.WIPLimit {
		hStyle = hStyle.Reverse(true)
	}

	header := hStyle.Width(width).Padding(0, 1).Render(headerText)

	separator := lipgloss.NewStyle().
		Faint(true).
		Width(width).
		Padding(0, 1).
		Render(strings.Repeat("─", width-2))

	var cardLines []string
	cardLines = append(cardLines, header)
	cardLines = append(cardLines, separator)

	if len(cards) == 0 {
		empty := emptyColumnStyle.
			Width(width).
			Padding(0, 1).
			Render("no cards")
		cardLines = append(cardLines, empty)
	}

	cardInnerWidth := max(width-4, 10)

	for cardIdx, card := range cards {
		isSelected := colIdx == a.board.focusCol && cardIdx == a.board.focusCard

		style := cardNormalBorder.Width(width - 2)
		if isSelected {
			style = cardSelectedBorder.Width(width - 2)
		}

		prefix := " "
		if isSelected {
			prefix = "▸"
		}

		pStyle := priorityStyle(string(card.Priority))
		titleLine := fmt.Sprintf("%s%s", prefix, truncate(card.Title, cardInnerWidth-1))
		prioLine := " " + pStyle.Render(string(card.Priority))

		content := titleLine + "\n" + prioLine
		if card.Labels != "" {
			content += "\n " + labelStyle.Render(truncate(card.Labels, cardInnerWidth-1))
		}

		rendered := style.Render(content)
		cardLines = append(cardLines, rendered)
	}

	body := lipgloss.JoinVertical(lipgloss.Left, cardLines...)

	return lipgloss.NewStyle().
		Width(width).
		Height(maxHeight).
		Render(body)
}

func (a *App) viewBoardHelp() string {
	w := a.width
	if w == 0 {
		w = 80
	}
	h := a.height
	if h == 0 {
		h = 24
	}

	titleBar := titleBarStyle.Width(w).Render(fmt.Sprintf(" kb: %s ", a.board.board.Name))
	statusBar := statusBarStyle.Width(w).Render(" Press any key to close help")

	entries := []struct{ key, desc string }{
		{"h / l", "Focus previous/next column"},
		{"j / k", "Select card up/down"},
		{"H / L", "Move card across columns"},
		{"J / K", "Reorder card within column"},
		{"", "  (then h/l/j/k to position, Enter to confirm)"},
		{"n", "New card"},
		{"Enter", "View card details"},
		{"e", "Edit card"},
		{"d", "Archive card"},
		{"D", "Delete card"},
		{"/", "Filter by label or priority"},
		{"1-4", "Filter by priority"},
		{"b", "Switch board"},
		{"?", "Toggle this help"},
		{"q", "Quit"},
	}

	var helpLines []string
	for _, e := range entries {
		line := fmt.Sprintf("  %s  %s",
			formLabelActiveStyle.Width(12).Render(e.key),
			e.desc)
		helpLines = append(helpLines, line)
	}

	helpContent := lipgloss.JoinVertical(lipgloss.Left, helpLines...)
	dialog := dialogBoxStyle.Render(helpContent)

	contentHeight := h - lipgloss.Height(titleBar) - lipgloss.Height(statusBar) - 1
	content := lipgloss.Place(w, contentHeight, lipgloss.Center, lipgloss.Center, dialog)

	return lipgloss.JoinVertical(lipgloss.Left, titleBar, content, statusBar)
}

func progressBar(done, total, width int) string {
	if total == 0 {
		return ""
	}
	filled := width * done / total
	return strings.Repeat("━", filled) + strings.Repeat("░", width-filled)
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
