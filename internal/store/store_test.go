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
