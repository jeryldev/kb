package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
)

type boardModel struct {
	board *model.Board
	lanes []fstore.Lane
	// cards are each lane's cards, by lane name.
	cards     map[string][]*model.Card
	focusCol  int
	focusCard int
	scrollCol int

	// filter matches a card's title, description or labels; priority is
	// an exact priority. A card is shown when it matches both.
	filter      string
	priority    model.Priority
	filterInput string
	filtering   bool

	confirming   string // "archive", "delete" or "move"
	moving       bool
	moveOrigCol  int
	moveOrigCard int
	moveCard     *model.Card
	showHelp     bool

	// diskRev is the board file's content when it was last read; a key
	// press that finds the file changed reads the board again.
	diskRev string
	// forceMove makes the next confirmed move go over the column's limit.
	forceMove bool
}

// loadBoard reads the board as it is on disk now, keeping the selected
// card selected when it is still in the focused column.
func (a *App) loadBoard() {
	selected := ""
	if c := a.selectedCard(); c != nil {
		selected = c.ID
	}
	// The hash is taken before the read, so a write landing between them
	// shows on the next key instead of never.
	rev := a.boardFileRev()
	if !a.reload() {
		return
	}
	board, err := a.db.GetBoard(a.board.board.ID)
	if a.fail(err) {
		return
	}
	lanes, err := a.db.Lanes(board.ID)
	if a.fail(err) {
		return
	}
	cards := map[string][]*model.Card{}
	for _, l := range lanes {
		list, err := a.db.Cards(board.ID, l.Name)
		if a.fail(err) {
			return
		}
		cards[l.Name] = list
	}
	a.board.board, a.board.lanes, a.board.cards = board, lanes, cards
	a.board.diskRev = rev
	a.board.focusCol = max(0, min(a.board.focusCol, len(lanes)-1))
	for i, c := range a.focusedCards() {
		if c.ID == selected {
			a.board.focusCard = i
		}
	}
	a.clampCardSelection()
	a.adjustScroll()
}

// boardFileRev is a hash of the board file as it is now, or "" when it
// cannot be read without waiting (a file still in iCloud).
func (a *App) boardFileRev() string {
	if a.db == nil || a.board.board == nil {
		return ""
	}
	v := a.db.Vault()
	e, err := v.Stat(a.board.board.ID)
	if err != nil || e.Dataless {
		return ""
	}
	abs, err := v.Abs(a.board.board.ID)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// followDisk reads the board again when its file changed since kb last
// read it (an editor, the Kanban plugin or another kb wrote it), so what
// is on screen, and what an edit starts from, is the file as it is.
func (a *App) followDisk() {
	if a.board.board == nil || a.db == nil {
		return
	}
	if _, err := a.db.Vault().Stat(a.board.board.ID); err != nil {
		a.boardGone()
		return
	}
	if rev := a.boardFileRev(); rev != "" && rev != a.board.diskRev {
		a.loadBoard()
	}
}

// boardGone leaves a board whose file was removed or renamed elsewhere.
func (a *App) boardGone() {
	a.err = fmt.Errorf("board %q is gone: its file was moved, renamed or deleted", a.board.board.Name)
	a.board = boardModel{}
	a.backToWorkspace()
}

func (a *App) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	b := &a.board
	if !b.filtering && b.confirming == "" && !b.moving {
		a.followDisk()
	}
	switch {
	case b.filtering:
		return a.updateBoardFiltering(key)
	case b.confirming != "":
		return a.updateBoardConfirming(key)
	case b.moving:
		return a.updateBoardMoving(key)
	case b.showHelp:
		b.showHelp = false
		return a, nil
	}

	switch key.String() {
	case "j", "down":
		if b.focusCard < len(a.focusedCards())-1 {
			b.focusCard++
		}
	case "k", "up":
		if b.focusCard > 0 {
			b.focusCard--
		}
	case "h", "left":
		a.focusColumn(b.focusCol - 1)
	case "l", "right":
		a.focusColumn(b.focusCol + 1)
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
		a.viewSelectedCard()
	case "e":
		return a, a.editSelectedCard(modeBoard)
	case "d":
		if a.selectedCard() != nil {
			b.confirming = "archive"
		}
	case "D":
		if a.selectedCard() != nil {
			b.confirming = "delete"
		}
	case "esc":
		a.setFilter("", "")
	case "/":
		b.filtering = true
		b.filterInput = b.filter
	case "1":
		a.togglePriorityFilter(model.PriorityUrgent)
	case "2":
		a.togglePriorityFilter(model.PriorityHigh)
	case "3":
		a.togglePriorityFilter(model.PriorityMedium)
	case "4":
		a.togglePriorityFilter(model.PriorityLow)
	case "b":
		a.backToWorkspace()
	case "r":
		a.loadBoard()
		if a.err == nil {
			a.feedback = "Read the board again"
		}
	case "?":
		b.showHelp = true
	case "q":
		return a, tea.Quit
	}
	return a, nil
}

func (a *App) focusedLane() (fstore.Lane, bool) {
	if a.board.focusCol < 0 || a.board.focusCol >= len(a.board.lanes) {
		return fstore.Lane{}, false
	}
	return a.board.lanes[a.board.focusCol], true
}

// focusedCards are the cards shown in the focused column.
func (a *App) focusedCards() []*model.Card {
	lane, ok := a.focusedLane()
	if !ok {
		return nil
	}
	return a.cardsForDisplay(lane.Name)
}

func (a *App) focusColumn(col int) {
	if col < 0 || col >= len(a.board.lanes) {
		return
	}
	a.board.focusCol = col
	a.clampCardSelection()
	a.adjustScroll()
}

func (a *App) startMoveMode(colDir, cardDir int) {
	b := &a.board
	card := a.selectedCard()
	if card == nil {
		return
	}
	targetCol := b.focusCol + colDir
	if targetCol < 0 || targetCol >= len(b.lanes) {
		return
	}
	if cardDir != 0 {
		target := b.focusCard + cardDir
		if target < 0 || target >= len(a.focusedCards()) {
			return
		}
	}
	b.moving = true
	b.moveOrigCol = b.focusCol
	b.moveOrigCard = b.focusCard
	b.moveCard = card
	if colDir != 0 {
		a.moveFocusColumn(targetCol)
	}
	b.focusCard += cardDir
}

// moveFocusColumn moves the card being moved to another column, keeping
// its position as far as that column allows.
func (a *App) moveFocusColumn(col int) {
	if col < 0 || col >= len(a.board.lanes) {
		return
	}
	a.board.focusCol = col
	// While moving, the card is shown in the focused column, so it can go
	// one past the column's other cards.
	a.board.focusCard = min(a.board.focusCard, len(a.focusedCards())-1)
	a.adjustScroll()
}

func (a *App) updateBoardMoving(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &a.board
	switch key.String() {
	case "h", "H", "left":
		a.moveFocusColumn(b.focusCol - 1)
	case "l", "L", "right":
		a.moveFocusColumn(b.focusCol + 1)
	case "j", "J", "down":
		if b.focusCard < len(a.focusedCards())-1 {
			b.focusCard++
		}
	case "k", "K", "up":
		if b.focusCard > 0 {
			b.focusCard--
		}
	case "enter":
		if b.focusCol == b.moveOrigCol && b.focusCard == b.moveOrigCard {
			a.cancelMoving()
			return a, nil
		}
		b.confirming = "move"
	case "esc":
		a.cancelMoving()
	}
	return a, nil
}

func (a *App) cancelMoving() {
	b := &a.board
	b.focusCol, b.focusCard = b.moveOrigCol, b.moveOrigCard
	b.moving, b.moveCard = false, nil
	a.adjustScroll()
}

func (a *App) updateBoardFiltering(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &a.board
	switch key.String() {
	case "enter":
		b.filtering = false
		a.setFilter(strings.TrimSpace(b.filterInput), b.priority)
	case "esc":
		// Esc stops typing; the filter already applied stays.
		b.filtering = false
	case "backspace":
		if runes := []rune(b.filterInput); len(runes) > 0 {
			b.filterInput = string(runes[:len(runes)-1])
		}
	default:
		if text, ok := typedText(key); ok {
			b.filterInput += text
		}
	}
	return a, nil
}

func (a *App) updateBoardConfirming(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &a.board
	action := b.confirming
	b.confirming = ""
	if !isYes(key) {
		if b.moving {
			a.cancelMoving()
		}
		return a, nil
	}
	switch action {
	case "archive", "delete":
		card := a.selectedCard()
		if card == nil {
			return a, nil
		}
		a.archiveOrDelete(action, card)
		a.loadBoard()
	case "move":
		a.commitCardMove()
	}
	return a, nil
}

func (a *App) archiveOrDelete(action string, card *model.Card) bool {
	if action == "archive" {
		if a.fail(a.db.ArchiveCard(card.BoardID, card.ID)) {
			return false
		}
		a.feedback = fmt.Sprintf("Archived %q", truncate(card.Title, 40))
		return true
	}
	if a.fail(a.db.DeleteCard(card.BoardID, card.ID)) {
		return false
	}
	a.feedback = fmt.Sprintf("Deleted %q", truncate(card.Title, 40))
	return true
}

func (a *App) cardMatches(c *model.Card) bool {
	b := &a.board
	if b.priority != "" && c.Priority != b.priority {
		return false
	}
	if b.filter == "" {
		return true
	}
	f := strings.ToLower(b.filter)
	return strings.Contains(strings.ToLower(c.Title), f) ||
		strings.Contains(strings.ToLower(c.Description), f) ||
		c.HasLabel(strings.TrimPrefix(b.filter, "#"))
}

func (a *App) filtering() bool { return a.board.filter != "" || a.board.priority != "" }

func (a *App) filteredCards(lane string) []*model.Card {
	cards := a.board.cards[lane]
	if !a.filtering() {
		return cards
	}
	var out []*model.Card
	for _, c := range cards {
		if a.cardMatches(c) {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) totalFilteredCardCount() int {
	n := 0
	for _, l := range a.board.lanes {
		n += len(a.filteredCards(l.Name))
	}
	return n
}

// cardsForDisplay are a column's shown cards; while a card is being moved,
// it is shown at its new place in the focused column instead of its old.
func (a *App) cardsForDisplay(lane string) []*model.Card {
	b := &a.board
	cards := a.filteredCards(lane)
	if !b.moving || b.moveCard == nil {
		return cards
	}
	without := make([]*model.Card, 0, len(cards))
	for _, c := range cards {
		if c.ID != b.moveCard.ID {
			without = append(without, c)
		}
	}
	if lane != b.lanes[b.focusCol].Name {
		return without
	}
	idx := max(0, min(b.focusCard, len(without)))
	out := make([]*model.Card, 0, len(without)+1)
	out = append(out, without[:idx]...)
	out = append(out, b.moveCard)
	return append(out, without[idx:]...)
}

func (a *App) selectedCard() *model.Card {
	if a.board.moving && a.board.moveCard != nil {
		return a.board.moveCard
	}
	cards := a.focusedCards()
	if a.board.focusCard < 0 || a.board.focusCard >= len(cards) {
		return nil
	}
	return cards[a.board.focusCard]
}

func (a *App) clampCardSelection() {
	a.board.focusCard = max(0, min(a.board.focusCard, len(a.focusedCards())-1))
}

func (a *App) visibleColumnCount() int {
	n := len(a.board.lanes)
	if n == 0 {
		return 0
	}
	w, _ := a.size()
	return max(1, min(w/24, n))
}

func (a *App) adjustScroll() {
	visible := a.visibleColumnCount()
	if visible == 0 {
		return
	}
	b := &a.board
	b.scrollCol = max(0, min(b.scrollCol, len(b.lanes)-visible))
	if b.focusCol < b.scrollCol {
		b.scrollCol = b.focusCol
	}
	if b.focusCol >= b.scrollCol+visible {
		b.scrollCol = b.focusCol - visible + 1
	}
}

// moveTarget is where the moved card goes, as the card it goes above (""
// for the end of the column). Only the shown cards were in view, so the
// card goes right above the shown card after it or, when it was put
// last, right below the shown card before it; cards the filter hides keep
// their places around it.
func (a *App) moveTarget() string {
	b := &a.board
	lane := b.lanes[b.focusCol].Name
	shown := a.cardsForDisplay(lane)
	idx := -1
	for i, c := range shown {
		if c.ID == b.moveCard.ID {
			idx = i
		}
	}
	if idx+1 < len(shown) {
		return shown[idx+1].ID
	}
	if idx <= 0 {
		return ""
	}
	prev := shown[idx-1].ID
	all := b.cards[lane]
	for i, c := range all {
		if c.ID == prev {
			for _, next := range all[i+1:] {
				if next.ID != b.moveCard.ID {
					return next.ID
				}
			}
		}
	}
	return ""
}

func (a *App) commitCardMove() {
	b := &a.board
	card := b.moveCard
	if card == nil || len(b.lanes) == 0 {
		b.moving, b.moveCard = false, nil
		return
	}
	lane := b.lanes[b.focusCol].Name
	before := a.moveTarget()
	force := b.forceMove
	b.forceMove = false
	err := a.db.MoveCard(b.board.ID, card.ID, lane, before, force)
	var wip *fstore.WIPLimitError
	if errors.As(err, &wip) {
		// Ask, with the move still on screen; y moves it anyway.
		b.forceMove = true
		b.confirming = "move"
		a.err = fmt.Errorf("%w; move it anyway?", err)
		return
	}
	if err != nil {
		a.cancelMoving()
		a.err = err
		a.loadBoard()
		return
	}
	b.moving, b.moveCard = false, nil
	a.loadBoard()
	// Keep the moved card selected.
	for i, c := range a.focusedCards() {
		if c.ID == card.ID {
			b.focusCard = i
		}
	}
	a.feedback = fmt.Sprintf("Moved %q to %s", truncate(card.Title, 30), lane)
}

func (a *App) newCardInCurrentColumn() tea.Cmd {
	lane, ok := a.focusedLane()
	if !ok {
		return nil
	}
	a.mode = modeCardEdit
	a.card = newCardModel(nil, a.board.board.ID, lane.Name, a.width, modeBoard)
	return a.card.Init()
}

func (a *App) viewSelectedCard() {
	card := a.selectedCard()
	if card == nil {
		return
	}
	a.cardView = cardViewModel{card: card}
	a.mode = modeCardView
}

func (a *App) editSelectedCard(returnTo mode) tea.Cmd {
	card := a.selectedCard()
	if returnTo == modeCardView {
		card = a.cardView.card
	}
	if card == nil {
		return nil
	}
	a.mode = modeCardEdit
	a.card = newCardModel(card, card.BoardID, card.ColumnID, a.width, returnTo)
	return a.card.Init()
}

func (a *App) cardFormWidth() int {
	return formWidth(a.width)
}

// formWidth is the width of the card dialogs on a terminal termW wide;
// they are drawn narrower when the terminal is.
func formWidth(termW int) int {
	return max(50, min(termW*80/100, 100))
}

func (a *App) togglePriorityFilter(p model.Priority) {
	if a.board.priority == p {
		p = ""
	}
	a.setFilter(a.board.filter, p)
}

// setFilter changes which cards are shown while keeping the same card
// selected; if the filter hides it, the column's first match is selected.
func (a *App) setFilter(filter string, priority model.Priority) {
	selected := a.selectedCard()
	a.board.filter, a.board.priority = filter, priority
	a.board.focusCard = 0
	if selected != nil {
		for i, c := range a.focusedCards() {
			if c.ID == selected.ID {
				a.board.focusCard = i
			}
		}
	}
	a.clampCardSelection()
}

func (a *App) boardTitle() string {
	label := a.board.board.Name
	if a.wsContent.workspace != nil {
		label = a.wsContent.workspace.Name + " > " + label
	}
	total, done := 0, 0
	for _, cards := range a.board.cards {
		total += len(cards)
	}
	if n := len(a.board.lanes); n > 0 {
		done = len(a.board.cards[a.board.lanes[n-1].Name])
	}
	if total == 0 {
		return fmt.Sprintf(" kb: %s ", label)
	}
	return fmt.Sprintf(" kb: %s  %s %d/%d ", label, progressBar(done, total, 10), done, total)
}

func (a *App) viewBoard() string {
	if a.board.showHelp {
		return a.viewBoardHelp()
	}
	b := &a.board
	hints := " hjkl: navigate   HJKL: move   n: new   Enter: view   e: edit   d: archive   /: filter   b: back   ?: help   q: quit"
	if b.moving && b.confirming == "" {
		hints = fmt.Sprintf(" Moving %q — h/l: column  j/k: position  Enter: confirm  Esc: cancel", truncate(b.moveCard.Title, 25))
	}
	w, _ := a.size()
	var extra []string
	if bar := a.scrollIndicator(w); bar != "" {
		extra = append(extra, bar)
	}
	if b.filtering {
		// A filter longer than the bar shows its end, where the cursor is.
		input := " / " + b.filterInput
		if room := w - 1; ansi.StringWidth(input) > room {
			input = ansi.TruncateLeft(input, ansi.StringWidth(input)-room+1, "…")
		}
		extra = append(extra, filterBarStyle.Render(input)+"█")
	} else if a.filtering() {
		var parts []string
		if b.filter != "" {
			parts = append(parts, fmt.Sprintf("%q", b.filter))
		}
		if b.priority != "" {
			parts = append(parts, "priority "+string(b.priority))
		}
		extra = append(extra, filterBarStyle.Render(fmt.Sprintf(" filter: %s (%d cards)", strings.Join(parts, ", "), a.totalFilteredCardCount()))+
			helpStyle.Render("  (/ to change, esc clears)"))
	}
	return a.frame(a.boardTitle(), hints, extra, func(w, h int) string {
		if b.confirming != "" {
			return a.renderConfirmDialog(w, h)
		}
		return a.renderColumns(w, h)
	})
}

func (a *App) scrollIndicator(w int) string {
	b := &a.board
	n, visible := len(b.lanes), a.visibleColumnCount()
	if n == 0 || visible >= n {
		return ""
	}
	var left, right string
	if b.scrollCol > 0 {
		left = fmt.Sprintf("< %d more", b.scrollCol)
	}
	if hidden := n - b.scrollCol - visible; hidden > 0 {
		right = fmt.Sprintf("%d more >", hidden)
	}
	return helpStyle.Render(left + strings.Repeat(" ", max(1, w-len(left)-len(right))) + right)
}

func (a *App) renderConfirmDialog(w, h int) string {
	b := &a.board
	card := a.selectedCard()
	if card == nil {
		return renderCenteredConfirm(w, h, "No card selected")
	}
	var prompt string
	switch b.confirming {
	case "move":
		from, to := b.lanes[b.moveOrigCol].Name, b.lanes[b.focusCol].Name
		if b.forceMove {
			prompt = fmt.Sprintf("%s is full. Move %q there anyway?", to, truncate(card.Title, 20))
		} else if from == to {
			prompt = fmt.Sprintf("Reorder %q in %s?", truncate(card.Title, 25), to)
		} else {
			prompt = fmt.Sprintf("Move %q from %s to %s?", truncate(card.Title, 20), from, to)
		}
	case "archive":
		prompt = fmt.Sprintf("Archive %q?", truncate(card.Title, 30))
	default:
		prompt = fmt.Sprintf("Delete %q from the board?", truncate(card.Title, 30))
	}
	return renderCenteredConfirm(w, h, prompt)
}

func (a *App) renderColumns(totalWidth, maxHeight int) string {
	b := &a.board
	n := len(b.lanes)
	if n == 0 {
		return lipgloss.Place(totalWidth, maxHeight, lipgloss.Center, lipgloss.Center, helpStyle.Render("No columns. Add one with: kb column add <name>"))
	}
	start := b.scrollCol
	end := min(start+a.visibleColumnCount(), n)
	shown := end - start
	colWidth := max((totalWidth-(shown-1))/shown, 16)
	divider := lipgloss.NewStyle().Faint(true).Render(strings.Repeat("│\n", maxHeight-1) + "│")
	var parts []string
	for i := start; i < end; i++ {
		if i > start {
			parts = append(parts, divider)
		}
		parts = append(parts, a.renderSingleColumn(b.lanes[i], i, colWidth, maxHeight))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func labelText(c *model.Card) string {
	labels := c.LabelList()
	if len(labels) == 0 {
		return ""
	}
	return "#" + strings.Join(labels, " #")
}

func (a *App) renderSingleColumn(lane fstore.Lane, colIdx, width, maxHeight int) string {
	cards := a.cardsForDisplay(lane.Name)
	// The limit counts every card in the column, shown or not.
	total := len(a.board.cards[lane.Name])
	count := fmt.Sprintf("%d", len(cards))
	if lane.WIPLimit != nil {
		count = fmt.Sprintf("%d/%d", total, *lane.WIPLimit)
	}
	hStyle := columnHeaderStyle
	if colIdx == a.board.focusCol {
		hStyle = columnHeaderActiveStyle
	}
	if lane.WIPLimit != nil && total >= *lane.WIPLimit {
		hStyle = hStyle.Reverse(true)
	}
	lines := []string{
		hStyle.Width(width).Padding(0, 1).Render(truncate(fmt.Sprintf("%s (%s)", lane.Name, count), width-2)),
		lipgloss.NewStyle().Faint(true).Width(width).Padding(0, 1).Render(strings.Repeat("─", max(0, width-2))),
	}
	if len(cards) == 0 {
		lines = append(lines, emptyColumnStyle.Width(width).Padding(0, 1).Render("no cards"))
	}

	inner := max(width-4, 10)
	var rendered []string
	var heights []int
	for i, card := range cards {
		selected := colIdx == a.board.focusCol && i == a.board.focusCard
		style, prefix := cardNormalBorder.Width(width-2), " "
		if selected {
			style, prefix = cardSelectedBorder.Width(width-2), "▸"
		}
		content := prefix + truncate(card.Title, inner-1)
		content += "\n " + priorityStyle(string(card.Priority)).Render(string(card.Priority))
		if l := labelText(card); l != "" {
			content += "\n " + labelStyle.Render(truncate(l, inner-1))
		}
		r := style.Render(content)
		rendered = append(rendered, r)
		heights = append(heights, lipgloss.Height(r))
	}

	// Show the cards around the selected one that fit, with a line for
	// those above and below.
	focus := 0
	if colIdx == a.board.focusCol {
		focus = a.board.focusCard
	}
	// Make room for the "more" lines the window needs; making room can
	// shift the window and call for the other one too.
	avail := maxHeight - len(lines)
	reserve := 0
	var start, end int
	for {
		start, end = window(heights, focus, max(1, avail-reserve))
		need := 0
		if start > 0 {
			need++
		}
		if end < len(rendered) {
			need++
		}
		if need <= reserve {
			break
		}
		reserve = need
	}
	if start > 0 {
		lines = append(lines, helpStyle.Width(width).Padding(0, 1).Render(fmt.Sprintf("↑ %d more", start)))
	}
	lines = append(lines, rendered[start:end]...)
	if end < len(rendered) {
		lines = append(lines, helpStyle.Width(width).Padding(0, 1).Render(fmt.Sprintf("↓ %d more", len(rendered)-end)))
	}
	return lipgloss.NewStyle().Width(width).Height(maxHeight).MaxHeight(maxHeight).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (a *App) viewBoardHelp() string {
	entries := []struct{ key, desc string }{
		{"h / l", "Focus previous/next column"},
		{"j / k", "Select card down/up"},
		{"H / L", "Move card across columns"},
		{"J / K", "Reorder card within column"},
		{"", "  (then h/l/j/k to position, Enter to confirm)"},
		{"n", "New card"},
		{"Enter", "View card"},
		{"e", "Edit card"},
		{"d", "Archive card"},
		{"D", "Delete card"},
		{"/", "Filter by text or label"},
		{"1-4", "Show only urgent/high/medium/low"},
		{"Esc", "Clear the filters"},
		{"r", "Read the board again from its file"},
		{"b", "Back to the workspace"},
		{"?", "This help"},
		{"q", "Quit"},
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("  %s  %s", formLabelActiveStyle.Width(12).Render(e.key), e.desc))
	}
	return a.frame(a.boardTitle(), " Press any key to close help", nil, func(w, h int) string {
		help := lipgloss.JoinVertical(lipgloss.Left, lines...)
		boxed := dialogBoxStyle.Render(help)
		// Too small for the box: the lines alone, from the top.
		if lipgloss.Width(boxed) > w || lipgloss.Height(boxed) > h {
			return help
		}
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, boxed)
	})
}

func progressBar(done, total, width int) string {
	if total == 0 {
		return ""
	}
	filled := width * done / total
	return strings.Repeat("━", filled) + strings.Repeat("░", width-filled)
}

// truncate cuts s to at most width terminal cells, ending in "…" when it
// cuts. Cells, not runes: a CJK character or an emoji takes two.
func truncate(s string, width int) string {
	if width <= 1 {
		return ansi.Truncate(s, max(0, width), "")
	}
	return ansi.Truncate(s, width, "…")
}
