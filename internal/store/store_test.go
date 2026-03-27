package store

import (
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenWithPath(":memory:")
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testDefaultWSID(t *testing.T, db *DB) string {
	t.Helper()
	ws, err := db.GetDefaultWorkspace()
	if err != nil {
		t.Fatalf("getting default workspace: %v", err)
	}
	return ws.ID
}
