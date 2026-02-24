package store

import (
	"testing"
)

func TestMigrate003CreatesWorkspacesTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='workspaces'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspaces table to exist, got count=%d", count)
	}
}

func TestMigrate003AddsWorkspaceIDToBoards(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('boards') WHERE name='workspace_id'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pragma: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspace_id column on boards, got count=%d", count)
	}
}

func TestMigrate003AddsWorkspaceIDToNotes(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info('notes') WHERE name='workspace_id'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying pragma: %v", err)
	}
	if count != 1 {
		t.Errorf("expected workspace_id column on notes, got count=%d", count)
	}
}
