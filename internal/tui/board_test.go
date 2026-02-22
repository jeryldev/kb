package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jeryldev/kb/internal/model"
)

func testApp(columns []*model.Column, cards map[string][]*model.Card) *App {
	return &App{
		board: boardModel{
			board:   &model.Board{ID: "board-1", Name: "Test"},
			columns: columns,
			cards:   cards,
		},
		width:  120,
		height: 40,
	}
}

func testColumns() []*model.Column {
	return []*model.Column{
		{ID: "col-1", Name: "Backlog", Position: 0},
		{ID: "col-2", Name: "Todo", Position: 1},
		{ID: "col-3", Name: "Done", Position: 2},
	}
}

func testCards() map[string][]*model.Card {
	return map[string][]*model.Card{
		"col-1": {
			{ID: "c1", ColumnID: "col-1", Title: "Card 1", Priority: model.PriorityMedium, Position: 0},
			{ID: "c2", ColumnID: "col-1", Title: "Card 2", Priority: model.PriorityHigh, Position: 1},
			{ID: "c3", ColumnID: "col-1", Title: "Card 3", Priority: model.PriorityLow, Position: 2},
		},
		"col-2": {
			{ID: "c4", ColumnID: "col-2", Title: "Card 4", Priority: model.PriorityUrgent, Position: 0},
			{ID: "c5", ColumnID: "col-2", Title: "Card 5", Priority: model.PriorityMedium, Position: 1},
		},
		"col-3": {},
	}
}

func TestStartMoveModeRight(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 1

	app.startMoveMode(1, 0)

	if !app.board.moving {
		t.Fatal("expected moving to be true")
	}
	if app.board.moveOrigCol != 0 {
		t.Errorf("moveOrigCol = %d, want 0", app.board.moveOrigCol)
	}
	if app.board.moveOrigCard != 1 {
		t.Errorf("moveOrigCard = %d, want 1", app.board.moveOrigCard)
	}
	if app.board.focusCol != 1 {
		t.Errorf("focusCol = %d, want 1", app.board.focusCol)
	}
	if app.board.moveCard.ID != "c2" {
		t.Errorf("moveCard = %q, want c2", app.board.moveCard.ID)
	}
}

func TestStartMoveModeLeft(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 1
	app.board.focusCard = 0

	app.startMoveMode(-1, 0)

	if !app.board.moving {
		t.Fatal("expected moving to be true")
	}
	if app.board.focusCol != 0 {
		t.Errorf("focusCol = %d, want 0", app.board.focusCol)
	}
}

func TestStartMoveModeDown(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(0, 1)

	if !app.board.moving {
		t.Fatal("expected moving to be true")
	}
	if app.board.moveOrigCol != 0 {
		t.Errorf("moveOrigCol = %d, want 0", app.board.moveOrigCol)
	}
	if app.board.moveOrigCard != 0 {
		t.Errorf("moveOrigCard = %d, want 0", app.board.moveOrigCard)
	}
	if app.board.focusCard != 1 {
		t.Errorf("focusCard = %d, want 1", app.board.focusCard)
	}
}

func TestStartMoveModeUp(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 2

	app.startMoveMode(0, -1)

	if !app.board.moving {
		t.Fatal("expected moving to be true")
	}
	if app.board.focusCard != 1 {
		t.Errorf("focusCard = %d, want 1", app.board.focusCard)
	}
}

func TestStartMoveModeAtBoundary(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(-1, 0)

	if app.board.moving {
		t.Fatal("should not enter move mode when at left boundary")
	}

	app.board.focusCol = 2
	app.startMoveMode(1, 0)

	if app.board.moving {
		t.Fatal("should not enter move mode when at right boundary")
	}
}

func TestStartMoveModeNoCard(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 2
	app.board.focusCard = 0

	app.startMoveMode(0, 1)

	if app.board.moving {
		t.Fatal("should not enter move mode with no card selected")
	}
}

func TestStartMoveModeAtCardBoundary(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(0, -1)
	if app.board.moving {
		t.Fatal("K on first card should not enter move mode")
	}

	app.board.focusCard = 2
	app.startMoveMode(0, 1)
	if app.board.moving {
		t.Fatal("J on last card should not enter move mode")
	}
}

func TestMovingHJKLNavigation(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.board.focusCard != 1 {
		t.Errorf("after j: focusCard = %d, want 1", app.board.focusCard)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.board.focusCard != 0 {
		t.Errorf("after k: focusCard = %d, want 0", app.board.focusCard)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if app.board.focusCol != 2 {
		t.Errorf("after l: focusCol = %d, want 2", app.board.focusCol)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if app.board.focusCol != 1 {
		t.Errorf("after h: focusCol = %d, want 1", app.board.focusCol)
	}
}

func TestMovingEscRestoresPosition(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 1

	app.startMoveMode(1, 0)

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyEscape})

	if app.board.moving {
		t.Fatal("expected moving to be false after esc")
	}
	if app.board.focusCol != 0 {
		t.Errorf("focusCol = %d, want 0 (original)", app.board.focusCol)
	}
	if app.board.focusCard != 1 {
		t.Errorf("focusCard = %d, want 1 (original)", app.board.focusCard)
	}
}

func TestMovingEnterSamePositionCancels(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 1

	app.startMoveMode(1, 0)
	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if app.board.focusCol != 0 || app.board.focusCard != 1 {
		t.Fatal("should be back at original position")
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyEnter})

	if app.board.moving {
		t.Fatal("enter at original position should cancel move mode")
	}
}

func TestMovingEnterDifferentPositionConfirms(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyEnter})

	if !app.board.moving {
		t.Fatal("should still be in moving mode during confirmation")
	}
	if app.board.confirming != "move" {
		t.Errorf("confirming = %q, want %q", app.board.confirming, "move")
	}
}

func TestCardsForDisplayCrossColumn(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	originCards := app.cardsForDisplay("col-1")
	if len(originCards) != 2 {
		t.Fatalf("origin column should have 2 cards (card removed), got %d", len(originCards))
	}
	for _, c := range originCards {
		if c.ID == "c1" {
			t.Fatal("moved card should not appear in origin column")
		}
	}

	targetCards := app.cardsForDisplay("col-2")
	if len(targetCards) != 3 {
		t.Fatalf("target column should have 3 cards (card inserted), got %d", len(targetCards))
	}
	if targetCards[0].ID != "c1" {
		t.Errorf("moved card should be at position 0 in target, got %q", targetCards[0].ID)
	}
}

func TestCardsForDisplaySameColumn(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(0, 1)

	cards := app.cardsForDisplay("col-1")
	if len(cards) != 3 {
		t.Fatalf("same column should still have 3 cards, got %d", len(cards))
	}
	if cards[0].ID != "c2" {
		t.Errorf("cards[0] = %q, want c2 (c1 moved down)", cards[0].ID)
	}
	if cards[1].ID != "c1" {
		t.Errorf("cards[1] = %q, want c1 (moved to position 1)", cards[1].ID)
	}
	if cards[2].ID != "c3" {
		t.Errorf("cards[2] = %q, want c3", cards[2].ID)
	}
}

func TestCardsForDisplayNotMoving(t *testing.T) {
	app := testApp(testColumns(), testCards())

	cards := app.cardsForDisplay("col-1")
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}
	if cards[0].ID != "c1" {
		t.Errorf("cards[0] = %q, want c1", cards[0].ID)
	}
}

func TestCardsForDisplayUnrelatedColumn(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	cards := app.cardsForDisplay("col-3")
	if len(cards) != 0 {
		t.Fatalf("unrelated column should be unchanged, got %d cards", len(cards))
	}
}

func TestCardsForDisplayMoveToEmptyColumn(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)
	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	cards := app.cardsForDisplay("col-3")
	if len(cards) != 1 {
		t.Fatalf("empty target column should have 1 card, got %d", len(cards))
	}
	if cards[0].ID != "c1" {
		t.Errorf("card = %q, want c1", cards[0].ID)
	}
}

func TestSelectedCardDuringMove(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 1

	app.startMoveMode(1, 0)

	card := app.selectedCard()
	if card == nil {
		t.Fatal("selectedCard should return the moving card")
	}
	if card.ID != "c2" {
		t.Errorf("selectedCard = %q, want c2", card.ID)
	}
}

func TestCancelMoving(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 1

	app.startMoveMode(1, 0)

	app.cancelMoving()

	if app.board.moving {
		t.Fatal("moving should be false")
	}
	if app.board.moveCard != nil {
		t.Fatal("moveCard should be nil")
	}
	if app.board.focusCol != 0 {
		t.Errorf("focusCol = %d, want 0", app.board.focusCol)
	}
	if app.board.focusCard != 1 {
		t.Errorf("focusCard = %d, want 1", app.board.focusCard)
	}
}

func TestMovingUppercaseKeysWork(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	if app.board.focusCard != 1 {
		t.Errorf("after J: focusCard = %d, want 1", app.board.focusCard)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	if app.board.focusCard != 0 {
		t.Errorf("after K: focusCard = %d, want 0", app.board.focusCard)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if app.board.focusCol != 2 {
		t.Errorf("after L: focusCol = %d, want 2", app.board.focusCol)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	if app.board.focusCol != 1 {
		t.Errorf("after H: focusCol = %d, want 1", app.board.focusCol)
	}
}

func TestMovingBoundaryClamp(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	if app.board.focusCol != 2 {
		t.Errorf("focusCol = %d, want 2 (clamped at right boundary)", app.board.focusCol)
	}

	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.board.focusCard != 0 {
		t.Errorf("focusCard = %d, want 0 (clamped at top)", app.board.focusCard)
	}
}

// --- Board confirming tests ---

func TestBoardArchiveConfirm(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.updateBoard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if app.board.confirming != "archive" {
		t.Errorf("confirming = %q, want %q", app.board.confirming, "archive")
	}
}

func TestBoardDeleteConfirm(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.updateBoard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})

	if app.board.confirming != "delete" {
		t.Errorf("confirming = %q, want %q", app.board.confirming, "delete")
	}
}

func TestBoardConfirmNoCardIgnored(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 2
	app.board.focusCard = 0

	app.updateBoard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if app.board.confirming != "" {
		t.Error("d on empty column should not enter confirming")
	}
}

func TestBoardConfirmCancelRestoresState(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0
	app.board.confirming = "archive"

	app.updateBoardConfirming(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if app.board.confirming != "" {
		t.Error("non-y key should cancel confirming")
	}
}

func TestBoardConfirmCancelDuringMoveAlsoCancelsMove(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 0

	app.startMoveMode(1, 0)
	app.updateBoardMoving(tea.KeyMsg{Type: tea.KeyEnter})

	if app.board.confirming != "move" {
		t.Fatal("should be in move confirming state")
	}

	app.updateBoardConfirming(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if app.board.confirming != "" {
		t.Error("confirming should be cleared")
	}
	if app.board.moving {
		t.Error("moving should be cancelled")
	}
	if app.board.focusCol != 0 {
		t.Errorf("focusCol = %d, want 0 (restored)", app.board.focusCol)
	}
}

// --- Card viewer confirming tests ---

func TestCardViewArchiveConfirm(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.cardView = cardViewModel{
		card:    testCards()["col-1"][0],
		colName: "Backlog",
	}

	app.updateCardView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if app.cardView.confirming != "archive" {
		t.Errorf("confirming = %q, want %q", app.cardView.confirming, "archive")
	}
}

func TestCardViewDeleteConfirm(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.cardView = cardViewModel{
		card:    testCards()["col-1"][0],
		colName: "Backlog",
	}

	app.updateCardView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})

	if app.cardView.confirming != "delete" {
		t.Errorf("confirming = %q, want %q", app.cardView.confirming, "delete")
	}
}

func TestCardViewConfirmCancel(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.cardView = cardViewModel{
		card:       testCards()["col-1"][0],
		colName:    "Backlog",
		confirming: "archive",
	}

	app.updateCardViewConfirming(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if app.cardView.confirming != "" {
		t.Error("non-y key should cancel confirming")
	}
}

func TestCardViewEscReturnsToBoard(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.mode = modeCardView
	app.cardView = cardViewModel{
		card:    testCards()["col-1"][0],
		colName: "Backlog",
	}

	app.updateCardView(tea.KeyMsg{Type: tea.KeyEscape})

	if app.mode != modeBoard {
		t.Errorf("mode = %d, want modeBoard (%d)", app.mode, modeBoard)
	}
}

// --- Picker tests ---

func TestPickerDeleteConfirm(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.boards = []*model.Board{
		{ID: "b1", Name: "Board 1"},
		{ID: "b2", Name: "Board 2"},
	}
	app.picker.cursor = 1

	app.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if app.picker.confirming != "delete" {
		t.Errorf("confirming = %q, want %q", app.picker.confirming, "delete")
	}
}

func TestPickerDeleteConfirmNoBoards(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.boards = nil

	app.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if app.picker.confirming != "" {
		t.Error("d with no boards should not enter confirming")
	}
}

func TestPickerConfirmCancel(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.boards = []*model.Board{{ID: "b1", Name: "Board 1"}}
	app.picker.confirming = "delete"

	app.updatePickerConfirming(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})

	if app.picker.confirming != "" {
		t.Error("non-y key should cancel confirming")
	}
}

func TestPickerAutoSelectTrue(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.autoSelect = true

	boards := []*model.Board{{ID: "b1", Name: "Board 1"}}
	app.updatePicker(boardsLoadedMsg{boards: boards})

	if app.mode != modeBoard {
		t.Errorf("autoSelect=true with 1 board should switch to board mode, got mode=%d", app.mode)
	}
}

func TestPickerAutoSelectFalse(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.autoSelect = false

	boards := []*model.Board{{ID: "b1", Name: "Board 1"}}
	app.updatePicker(boardsLoadedMsg{boards: boards})

	if app.mode == modeBoard {
		t.Error("autoSelect=false with 1 board should stay in picker")
	}
	if len(app.picker.boards) != 1 {
		t.Errorf("boards not loaded: got %d, want 1", len(app.picker.boards))
	}
}

func TestPickerCursorClampAfterDelete(t *testing.T) {
	app := &App{width: 80, height: 40}
	app.picker.cursor = 2

	boards := []*model.Board{
		{ID: "b1", Name: "Board 1"},
		{ID: "b2", Name: "Board 2"},
	}
	app.updatePicker(boardsLoadedMsg{boards: boards})

	if app.picker.cursor >= len(app.picker.boards) {
		t.Errorf("cursor = %d, should be clamped to < %d", app.picker.cursor, len(app.picker.boards))
	}
}

func TestBoardFocusCardClampAfterReload(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.board.focusCol = 0
	app.board.focusCard = 2 // last card in col-1

	// Simulate board reload with fewer cards (one was deleted)
	app.updateBoard(boardLoadedMsg{
		columns: testColumns(),
		cards: map[string][]*model.Card{
			"col-1": {
				{ID: "c1", ColumnID: "col-1", Title: "Card 1", Priority: model.PriorityMedium, Position: 0},
				{ID: "c2", ColumnID: "col-1", Title: "Card 2", Priority: model.PriorityHigh, Position: 1},
			},
			"col-2": testCards()["col-2"],
			"col-3": {},
		},
	})

	if app.board.focusCard >= 2 {
		t.Errorf("focusCard = %d, should be clamped to < 2 after card deletion", app.board.focusCard)
	}
}

// --- Truncate tests ---

func TestTruncateEdgeCases(t *testing.T) {
	if got := truncate("Hello", 0); got != "" {
		t.Errorf("truncate(s, 0) = %q, want empty", got)
	}
	if got := truncate("Hello", 1); got != "H" {
		t.Errorf("truncate(s, 1) = %q, want %q", got, "H")
	}
	if got := truncate("Hi", 5); got != "Hi" {
		t.Errorf("truncate short = %q, want %q", got, "Hi")
	}
	if got := truncate("Hello World", 5); got != "Hell…" {
		t.Errorf("truncate long = %q, want %q", got, "Hell…")
	}
}
