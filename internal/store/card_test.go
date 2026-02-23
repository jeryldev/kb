package store

import (
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func createTestBoardWithColumn(t *testing.T, db *DB) (*model.Board, *model.Column) {
	t.Helper()
	board := createTestBoard(t, db)
	columns, err := db.ListColumns(board.ID)
	if err != nil || len(columns) == 0 {
		t.Fatal("board should have default columns")
	}
	return board, columns[0]
}

func TestCreateCard(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, err := db.CreateCard(col.ID, "Fix login bug", model.PriorityUrgent)
	if err != nil {
		t.Fatalf("CreateCard failed: %v", err)
	}
	if card.Title != "Fix login bug" {
		t.Errorf("card.Title = %q, want %q", card.Title, "Fix login bug")
	}
	if card.Priority != model.PriorityUrgent {
		t.Errorf("card.Priority = %q, want %q", card.Priority, model.PriorityUrgent)
	}
	if card.ColumnID != col.ID {
		t.Errorf("card.ColumnID = %q, want %q", card.ColumnID, col.ID)
	}
	if card.Position != 0 {
		t.Errorf("first card position = %d, want 0", card.Position)
	}
}

func TestCreateCardAutoPosition(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	db.CreateCard(col.ID, "First", model.PriorityMedium)
	second, _ := db.CreateCard(col.ID, "Second", model.PriorityMedium)
	third, _ := db.CreateCard(col.ID, "Third", model.PriorityMedium)

	if second.Position != 1 {
		t.Errorf("second card position = %d, want 1", second.Position)
	}
	if third.Position != 2 {
		t.Errorf("third card position = %d, want 2", third.Position)
	}
}

func TestCreateCardRejectsEmptyTitle(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	_, err := db.CreateCard(col.ID, "", model.PriorityMedium)
	if err == nil {
		t.Error("CreateCard with empty title should return error")
	}
}

func TestGetCard(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	created, _ := db.CreateCard(col.ID, "Test card", model.PriorityHigh)

	got, err := db.GetCard(created.ID)
	if err != nil {
		t.Fatalf("GetCard failed: %v", err)
	}
	if got.Title != "Test card" {
		t.Errorf("got.Title = %q, want %q", got.Title, "Test card")
	}
	if got.Priority != model.PriorityHigh {
		t.Errorf("got.Priority = %q, want %q", got.Priority, model.PriorityHigh)
	}
}

func TestGetCardNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetCard("nonexistent")
	if err == nil {
		t.Error("GetCard with nonexistent ID should return error")
	}
}

func TestGetCardExcludesDeleted(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "Will delete", model.PriorityMedium)
	db.DeleteCard(card.ID)

	_, err := db.GetCard(card.ID)
	if err == nil {
		t.Error("GetCard should not return soft-deleted card")
	}
}

func TestListCards(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	db.CreateCard(col.ID, "First", model.PriorityMedium)
	db.CreateCard(col.ID, "Second", model.PriorityHigh)
	db.CreateCard(col.ID, "Third", model.PriorityLow)

	cards, err := db.ListCards(col.ID)
	if err != nil {
		t.Fatalf("ListCards failed: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("got %d cards, want 3", len(cards))
	}

	for i := 1; i < len(cards); i++ {
		if cards[i].Position <= cards[i-1].Position {
			t.Errorf("cards not ordered by position: [%d]=%d, [%d]=%d",
				i-1, cards[i-1].Position, i, cards[i].Position)
		}
	}
}

func TestListCardsExcludesArchivedAndDeleted(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	db.CreateCard(col.ID, "Active", model.PriorityMedium)
	archived, _ := db.CreateCard(col.ID, "Archived", model.PriorityMedium)
	deleted, _ := db.CreateCard(col.ID, "Deleted", model.PriorityMedium)

	db.ArchiveCard(archived.ID)
	db.DeleteCard(deleted.ID)

	cards, _ := db.ListCards(col.ID)
	if len(cards) != 1 {
		t.Errorf("ListCards should exclude archived/deleted: got %d, want 1", len(cards))
	}
	if cards[0].Title != "Active" {
		t.Errorf("remaining card = %q, want %q", cards[0].Title, "Active")
	}
}

func TestUpdateCard(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "Original", model.PriorityLow)
	card.Title = "Updated"
	card.Priority = model.PriorityUrgent
	card.Labels = "bug, critical"
	card.Description = "This is important"

	err := db.UpdateCard(card)
	if err != nil {
		t.Fatalf("UpdateCard failed: %v", err)
	}

	got, _ := db.GetCard(card.ID)
	if got.Title != "Updated" {
		t.Errorf("title not updated: %q", got.Title)
	}
	if got.Priority != model.PriorityUrgent {
		t.Errorf("priority not updated: %q", got.Priority)
	}
	if got.Labels != "bug, critical" {
		t.Errorf("labels not updated: %q", got.Labels)
	}
	if got.Description != "This is important" {
		t.Errorf("description not updated: %q", got.Description)
	}
}

func TestUpdateCardRejectsEmptyTitle(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "Valid", model.PriorityMedium)
	card.Title = ""

	err := db.UpdateCard(card)
	if err == nil {
		t.Error("UpdateCard with empty title should return error")
	}
}

func TestMoveCard(t *testing.T) {
	db := testDB(t)
	board, col := createTestBoardWithColumn(t, db)

	columns, _ := db.ListColumns(board.ID)
	targetCol := columns[1]

	card, _ := db.CreateCard(col.ID, "Moving card", model.PriorityMedium)

	err := db.MoveCard(card.ID, targetCol.ID)
	if err != nil {
		t.Fatalf("MoveCard failed: %v", err)
	}

	got, _ := db.GetCard(card.ID)
	if got.ColumnID != targetCol.ID {
		t.Errorf("card.ColumnID = %q, want %q", got.ColumnID, targetCol.ID)
	}
}

func TestArchiveCard(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "To archive", model.PriorityMedium)

	err := db.ArchiveCard(card.ID)
	if err != nil {
		t.Fatalf("ArchiveCard failed: %v", err)
	}

	cards, _ := db.ListCards(col.ID)
	for _, c := range cards {
		if c.ID == card.ID {
			t.Error("archived card should not appear in ListCards")
		}
	}
}

func TestArchiveCardIdempotent(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "Archive twice", model.PriorityMedium)
	db.ArchiveCard(card.ID)

	err := db.ArchiveCard(card.ID)
	if err == nil {
		t.Error("archiving already-archived card should return error")
	}
}

func TestDeleteCard(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "To delete", model.PriorityMedium)

	err := db.DeleteCard(card.ID)
	if err != nil {
		t.Fatalf("DeleteCard failed: %v", err)
	}

	_, err = db.GetCard(card.ID)
	if err == nil {
		t.Error("deleted card should not be returned by GetCard")
	}
}

func TestDeleteCardIdempotent(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	card, _ := db.CreateCard(col.ID, "Delete twice", model.PriorityMedium)
	db.DeleteCard(card.ID)

	err := db.DeleteCard(card.ID)
	if err == nil {
		t.Error("deleting already-deleted card should return error")
	}
}

func TestReorderCardsInColumn(t *testing.T) {
	db := testDB(t)
	_, col := createTestBoardWithColumn(t, db)

	c1, _ := db.CreateCard(col.ID, "First", model.PriorityMedium)
	c2, _ := db.CreateCard(col.ID, "Second", model.PriorityMedium)
	c3, _ := db.CreateCard(col.ID, "Third", model.PriorityMedium)

	err := db.ReorderCardsInColumn(col.ID, []string{c3.ID, c2.ID, c1.ID})
	if err != nil {
		t.Fatalf("ReorderCardsInColumn failed: %v", err)
	}

	cards, _ := db.ListCards(col.ID)
	if cards[0].Title != "Third" {
		t.Errorf("first card after reorder = %q, want %q", cards[0].Title, "Third")
	}
	if cards[2].Title != "First" {
		t.Errorf("last card after reorder = %q, want %q", cards[2].Title, "First")
	}
}

func TestListBoardCards(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)

	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Backlog card", model.PriorityLow)
	db.CreateCard(columns[1].ID, "Todo card", model.PriorityMedium)
	db.CreateCard(columns[2].ID, "In progress card", model.PriorityHigh)

	cards, err := db.ListBoardCards(board.ID)
	if err != nil {
		t.Fatalf("ListBoardCards failed: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("got %d cards, want 3", len(cards))
	}

	if cards[0].Title != "Backlog card" {
		t.Errorf("first card = %q, want %q", cards[0].Title, "Backlog card")
	}
	if cards[2].Title != "In progress card" {
		t.Errorf("last card = %q, want %q", cards[2].Title, "In progress card")
	}
}

func TestListBoardCardsExcludesArchivedAndDeleted(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)

	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Active", model.PriorityMedium)
	archived, _ := db.CreateCard(columns[0].ID, "Archived", model.PriorityMedium)
	deleted, _ := db.CreateCard(columns[1].ID, "Deleted", model.PriorityMedium)

	db.ArchiveCard(archived.ID)
	db.DeleteCard(deleted.ID)

	cards, _ := db.ListBoardCards(board.ID)
	if len(cards) != 1 {
		t.Errorf("ListBoardCards should exclude archived/deleted: got %d, want 1", len(cards))
	}
}

func TestListBoardCardsFilteredByPriority(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Low task", model.PriorityLow)
	db.CreateCard(columns[0].ID, "High task", model.PriorityHigh)
	db.CreateCard(columns[1].ID, "Another high", model.PriorityHigh)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Priority: "high"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("got %d cards, want 2", len(cards))
	}
	for _, c := range cards {
		if c.Priority != model.PriorityHigh {
			t.Errorf("card %q has priority %q, want high", c.Title, c.Priority)
		}
	}
}

func TestListBoardCardsFilteredByColumn(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Backlog card", model.PriorityMedium)
	db.CreateCard(columns[1].ID, "Todo card", model.PriorityMedium)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Column: "todo"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1", len(cards))
	}
	if cards[0].Title != "Todo card" {
		t.Errorf("card title = %q, want %q", cards[0].Title, "Todo card")
	}
}

func TestListBoardCardsFilteredBySearch(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	c1, _ := db.CreateCard(columns[0].ID, "Auth login fix", model.PriorityMedium)
	_ = c1
	c2, _ := db.CreateCard(columns[0].ID, "Dashboard update", model.PriorityMedium)
	c2.Description = "Needs auth token refresh"
	db.UpdateCard(c2)
	db.CreateCard(columns[0].ID, "Unrelated task", model.PriorityMedium)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Search: "auth"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("got %d cards, want 2 (title match + description match)", len(cards))
	}
}

func TestListBoardCardsFilteredByLabel(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	c1, _ := db.CreateCard(columns[0].ID, "Bug card", model.PriorityMedium)
	c1.Labels = "bug,frontend"
	db.UpdateCard(c1)

	c2, _ := db.CreateCard(columns[0].ID, "Debugging card", model.PriorityMedium)
	c2.Labels = "debugging"
	db.UpdateCard(c2)

	db.CreateCard(columns[0].ID, "No labels", model.PriorityMedium)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Label: "bug"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1 (exact label match only)", len(cards))
	}
	if cards[0].Title != "Bug card" {
		t.Errorf("card = %q, want %q", cards[0].Title, "Bug card")
	}
}

func TestListBoardCardsFilteredCombined(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Backlog urgent", model.PriorityUrgent)
	db.CreateCard(columns[1].ID, "Todo urgent", model.PriorityUrgent)
	db.CreateCard(columns[1].ID, "Todo medium", model.PriorityMedium)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Priority: "urgent", Column: "Todo"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("got %d cards, want 1 (urgent + Todo)", len(cards))
	}
	if cards[0].Title != "Todo urgent" {
		t.Errorf("card = %q, want %q", cards[0].Title, "Todo urgent")
	}
}

func TestListBoardCardsFilteredEmpty(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Card A", model.PriorityMedium)
	db.CreateCard(columns[1].ID, "Card B", model.PriorityHigh)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("empty filter should return all cards: got %d, want 2", len(cards))
	}
}

func TestListBoardCardsFilteredNoResults(t *testing.T) {
	db := testDB(t)
	board, _ := createTestBoardWithColumn(t, db)
	columns, _ := db.ListColumns(board.ID)

	db.CreateCard(columns[0].ID, "Card A", model.PriorityMedium)

	cards, err := db.ListBoardCardsFiltered(board.ID, CardFilter{Priority: "urgent"})
	if err != nil {
		t.Fatalf("ListBoardCardsFiltered failed: %v", err)
	}
	if len(cards) != 0 {
		t.Fatalf("expected empty result, got %d cards", len(cards))
	}
}
