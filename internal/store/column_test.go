package store

import (
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func createTestBoard(t *testing.T, db *DB) *model.Board {
	t.Helper()
	board, err := db.CreateBoard("test-board", "")
	if err != nil {
		t.Fatalf("CreateBoard failed: %v", err)
	}
	return board
}

func TestListColumnsOrder(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, err := db.ListColumns(board.ID)
	if err != nil {
		t.Fatalf("ListColumns failed: %v", err)
	}

	for i := 1; i < len(columns); i++ {
		if columns[i].Position <= columns[i-1].Position {
			t.Errorf("columns not ordered by position: [%d]=%d, [%d]=%d",
				i-1, columns[i-1].Position, i, columns[i].Position)
		}
	}
}

func TestCreateColumnAutoPosition(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	col, err := db.CreateColumn(board.ID, "QA")
	if err != nil {
		t.Fatalf("CreateColumn failed: %v", err)
	}

	if col.Position != 5 {
		t.Errorf("col.Position = %d, want 5", col.Position)
	}
	if col.Name != "QA" {
		t.Errorf("col.Name = %q, want %q", col.Name, "QA")
	}
}

func TestCreateColumnRejectsEmptyName(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	_, err := db.CreateColumn(board.ID, "")
	if err == nil {
		t.Error("CreateColumn with empty name should return error")
	}
}

func TestReorderColumns(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, _ := db.ListColumns(board.ID)
	reversed := make([]string, len(columns))
	for i, col := range columns {
		reversed[len(columns)-1-i] = col.ID
	}

	err := db.ReorderColumns(board.ID, reversed)
	if err != nil {
		t.Fatalf("ReorderColumns failed: %v", err)
	}

	reordered, _ := db.ListColumns(board.ID)
	if reordered[0].Name != "Done" {
		t.Errorf("first column after reverse should be Done, got %q", reordered[0].Name)
	}
	if reordered[len(reordered)-1].Name != "Backlog" {
		t.Errorf("last column after reverse should be Backlog, got %q", reordered[len(reordered)-1].Name)
	}
}

func TestDeleteColumn(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, _ := db.ListColumns(board.ID)
	target := columns[0]

	err := db.DeleteColumn(target.ID)
	if err != nil {
		t.Fatalf("DeleteColumn failed: %v", err)
	}

	remaining, _ := db.ListColumns(board.ID)
	if len(remaining) != len(columns)-1 {
		t.Errorf("got %d columns after delete, want %d", len(remaining), len(columns)-1)
	}
}

func TestDeleteColumnNotFound(t *testing.T) {
	db := testDB(t)

	err := db.DeleteColumn("nonexistent")
	if err == nil {
		t.Error("DeleteColumn with nonexistent ID should return error")
	}
}

func TestCountCardsInColumn(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, _ := db.ListColumns(board.ID)
	col := columns[0]

	count, err := db.CountCardsInColumn(col.ID)
	if err != nil {
		t.Fatalf("CountCardsInColumn failed: %v", err)
	}
	if count != 0 {
		t.Errorf("empty column count = %d, want 0", count)
	}

	db.CreateCard(col.ID, "Card 1", model.PriorityMedium)
	db.CreateCard(col.ID, "Card 2", model.PriorityHigh)

	count, _ = db.CountCardsInColumn(col.ID)
	if count != 2 {
		t.Errorf("count after adding 2 cards = %d, want 2", count)
	}
}

func TestCountCardsExcludesArchivedAndDeleted(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, _ := db.ListColumns(board.ID)
	col := columns[0]

	c1, _ := db.CreateCard(col.ID, "Active", model.PriorityMedium)
	c2, _ := db.CreateCard(col.ID, "To archive", model.PriorityMedium)
	c3, _ := db.CreateCard(col.ID, "To delete", model.PriorityMedium)

	db.ArchiveCard(c2.ID)
	db.DeleteCard(c3.ID)

	count, _ := db.CountCardsInColumn(col.ID)
	if count != 1 {
		t.Errorf("count should exclude archived/deleted: got %d, want 1 (only %q)", count, c1.Title)
	}
}

func TestUpdateColumnWIPLimit(t *testing.T) {
	db := testDB(t)
	board := createTestBoard(t, db)

	columns, _ := db.ListColumns(board.ID)
	col := columns[0]

	limit := 3
	err := db.UpdateColumnWIPLimit(col.ID, &limit)
	if err != nil {
		t.Fatalf("UpdateColumnWIPLimit failed: %v", err)
	}

	updated, _ := db.ListColumns(board.ID)
	if updated[0].WIPLimit == nil || *updated[0].WIPLimit != 3 {
		t.Error("WIP limit should be 3 after update")
	}

	err = db.UpdateColumnWIPLimit(col.ID, nil)
	if err != nil {
		t.Fatalf("clearing WIP limit failed: %v", err)
	}

	cleared, _ := db.ListColumns(board.ID)
	if cleared[0].WIPLimit != nil {
		t.Error("WIP limit should be nil after clearing")
	}
}
