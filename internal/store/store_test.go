package store

import (
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenWithPath(":memory:", t.TempDir())
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

// testDBWithVault opens a fresh index over an existing vault, as a rebuilt
// kb.db would.
func testDBWithVault(t *testing.T, vaultDir string) *DB {
	t.Helper()
	db, err := OpenWithPath(":memory:", vaultDir)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
