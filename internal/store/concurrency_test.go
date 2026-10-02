package store

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

// Several kb processes started at once on a database none of them has
// migrated yet (the first run after an upgrade) must all open it.
func TestConcurrentOpensMigrateOnce(t *testing.T) {
	dir := t.TempDir()
	dbFile, vaultDir := filepath.Join(dir, "kb.db"), filepath.Join(dir, "notes")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := OpenWithPath(dbFile, vaultDir)
			if err == nil {
				db.Close()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("open: %v", err)
		}
	}
	db, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	db.conn.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode = %q", mode)
	}
}

// Moves into the same column from two processes must not hand two cards
// the same position.
func TestConcurrentMovesGetDistinctPositions(t *testing.T) {
	dir := t.TempDir()
	dbFile, vaultDir := filepath.Join(dir, "kb.db"), filepath.Join(dir, "notes")
	a, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	board, err := a.CreateBoard("race", "", testDefaultWSID(t, a))
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := a.ListColumns(board.ID)
	from, to := cols[0].ID, cols[1].ID
	var cards []*model.Card
	for i := range 20 {
		c, err := a.CreateCard(from, string(rune('a'+i)), model.PriorityMedium)
		if err != nil {
			t.Fatal(err)
		}
		cards = append(cards, c)
	}

	var wg sync.WaitGroup
	for i, c := range cards {
		db := a
		if i%2 == 1 {
			db = b
		}
		wg.Add(1)
		go func(db *DB, id string) {
			defer wg.Done()
			if err := db.MoveCard(id, to); err != nil {
				t.Errorf("move: %v", err)
			}
		}(db, c.ID)
	}
	wg.Wait()

	moved, _ := a.ListCards(to)
	seen := map[int]bool{}
	for _, c := range moved {
		if seen[c.Position] {
			t.Errorf("two cards at position %d", c.Position)
		}
		seen[c.Position] = true
	}
	if len(moved) != 20 {
		t.Errorf("moved %d cards", len(moved))
	}
}

func TestConcurrentCreatesGetDistinctPositions(t *testing.T) {
	dir := t.TempDir()
	dbFile, vaultDir := filepath.Join(dir, "kb.db"), filepath.Join(dir, "notes")
	a, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	wsID := testDefaultWSID(t, a)
	board, err := a.CreateBoard("race", "", wsID)
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := a.ListColumns(board.ID)

	var wg sync.WaitGroup
	for i := range 20 {
		db := a
		if i%2 == 1 {
			db = b
		}
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := db.CreateCard(cols[0].ID, "card", model.PriorityLow); err != nil {
				t.Errorf("card: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := db.CreateColumn(board.ID, "col"+string(rune('a'+i))); err != nil {
				t.Errorf("column: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := db.CreateWorkspace("ws"+string(rune('a'+i)), model.KindArea, "", ""); err != nil {
				t.Errorf("workspace: %v", err)
			}
		}()
	}
	wg.Wait()

	positions := func(what string, ps []int) {
		seen := map[int]bool{}
		for _, p := range ps {
			if seen[p] {
				t.Errorf("two %s at position %d", what, p)
			}
			seen[p] = true
		}
	}
	cards, _ := a.ListCards(cols[0].ID)
	var ps []int
	for _, c := range cards {
		ps = append(ps, c.Position)
	}
	positions("cards", ps)
	ps = nil
	allCols, _ := a.ListColumns(board.ID)
	for _, c := range allCols {
		ps = append(ps, c.Position)
	}
	positions("columns", ps)
	ps = nil
	wss, _ := a.ListWorkspaces()
	for _, w := range wss {
		ps = append(ps, w.Position)
	}
	positions("workspaces", ps)
}
