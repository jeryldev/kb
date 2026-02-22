package store

import (
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func TestCreateBoard(t *testing.T) {
	db := testDB(t)

	board, err := db.CreateBoard("test-project", "A test board")
	if err != nil {
		t.Fatalf("CreateBoard failed: %v", err)
	}
	if board.Name != "test-project" {
		t.Errorf("board.Name = %q, want %q", board.Name, "test-project")
	}
	if board.Description != "A test board" {
		t.Errorf("board.Description = %q, want %q", board.Description, "A test board")
	}
	if board.ID == "" {
		t.Error("board.ID should not be empty")
	}
}

func TestCreateBoardCreatesDefaultColumns(t *testing.T) {
	db := testDB(t)

	board, err := db.CreateBoard("test-project", "")
	if err != nil {
		t.Fatalf("CreateBoard failed: %v", err)
	}

	columns, err := db.ListColumns(board.ID)
	if err != nil {
		t.Fatalf("ListColumns failed: %v", err)
	}
	if len(columns) != len(model.DefaultColumns) {
		t.Fatalf("got %d columns, want %d", len(columns), len(model.DefaultColumns))
	}
	for i, col := range columns {
		if col.Name != model.DefaultColumns[i] {
			t.Errorf("column[%d].Name = %q, want %q", i, col.Name, model.DefaultColumns[i])
		}
		if col.Position != i {
			t.Errorf("column[%d].Position = %d, want %d", i, col.Position, i)
		}
	}
}

func TestCreateBoardRejectsEmptyName(t *testing.T) {
	db := testDB(t)

	_, err := db.CreateBoard("", "desc")
	if err == nil {
		t.Error("CreateBoard with empty name should return error")
	}
}

func TestCreateBoardRejectsDuplicateName(t *testing.T) {
	db := testDB(t)

	_, err := db.CreateBoard("dup", "first")
	if err != nil {
		t.Fatalf("first CreateBoard failed: %v", err)
	}

	_, err = db.CreateBoard("dup", "second")
	if err == nil {
		t.Error("duplicate board name should return error")
	}
}

func TestGetBoard(t *testing.T) {
	db := testDB(t)

	created, _ := db.CreateBoard("test", "desc")

	got, err := db.GetBoard(created.ID)
	if err != nil {
		t.Fatalf("GetBoard failed: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("got.Name = %q, want %q", got.Name, "test")
	}
}

func TestGetBoardNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetBoard("nonexistent-id")
	if err == nil {
		t.Error("GetBoard with nonexistent ID should return error")
	}
}

func TestGetBoardByName(t *testing.T) {
	db := testDB(t)

	created, _ := db.CreateBoard("my-board", "")

	got, err := db.GetBoardByName("my-board")
	if err != nil {
		t.Fatalf("GetBoardByName failed: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got.ID = %q, want %q", got.ID, created.ID)
	}
}

func TestGetBoardByNameNotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetBoardByName("nonexistent")
	if err != nil {
		t.Fatalf("GetBoardByName returned error: %v", err)
	}
	if got != nil {
		t.Error("GetBoardByName for nonexistent board should return nil")
	}
}

func TestListBoards(t *testing.T) {
	db := testDB(t)

	db.CreateBoard("alpha", "")
	db.CreateBoard("beta", "")
	db.CreateBoard("gamma", "")

	boards, err := db.ListBoards()
	if err != nil {
		t.Fatalf("ListBoards failed: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("got %d boards, want 3", len(boards))
	}
	if boards[0].Name != "alpha" || boards[1].Name != "beta" || boards[2].Name != "gamma" {
		t.Errorf("boards not sorted by name: %q, %q, %q", boards[0].Name, boards[1].Name, boards[2].Name)
	}
}

func TestDeleteBoard(t *testing.T) {
	db := testDB(t)

	board, _ := db.CreateBoard("to-delete", "")

	err := db.DeleteBoard(board.ID)
	if err != nil {
		t.Fatalf("DeleteBoard failed: %v", err)
	}

	_, err = db.GetBoard(board.ID)
	if err == nil {
		t.Error("board should not exist after deletion")
	}
}

func TestDeleteBoardCascadesColumns(t *testing.T) {
	db := testDB(t)

	board, _ := db.CreateBoard("cascade-test", "")

	columns, _ := db.ListColumns(board.ID)
	if len(columns) == 0 {
		t.Fatal("board should have default columns")
	}

	db.DeleteBoard(board.ID)

	columns, _ = db.ListColumns(board.ID)
	if len(columns) != 0 {
		t.Errorf("columns should be deleted after board deletion, got %d", len(columns))
	}
}

func TestDeleteBoardNotFound(t *testing.T) {
	db := testDB(t)

	err := db.DeleteBoard("nonexistent")
	if err == nil {
		t.Error("DeleteBoard with nonexistent ID should return error")
	}
}
