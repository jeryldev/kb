package store

import (
	"testing"
)

func TestMigrate002CreatesNotesTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='notes'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected notes table to exist, got count=%d", count)
	}
}

func TestMigrate002CreatesLinksTable(t *testing.T) {
	db := testDB(t)

	var count int
	err := db.conn.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='links'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("querying sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected links table to exist, got count=%d", count)
	}
}

func TestCreateNote(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, err := db.CreateNote("My First Note", "my-first-note", "Hello world", wsID)
	if err != nil {
		t.Fatalf("creating note: %v", err)
	}
	if note.Title != "My First Note" {
		t.Errorf("title = %q, want 'My First Note'", note.Title)
	}
	if note.Slug != "my-first-note" {
		t.Errorf("slug = %q, want 'my-first-note'", note.Slug)
	}
	if note.Body != "Hello world" {
		t.Errorf("body = %q, want 'Hello world'", note.Body)
	}
	if note.ID == "" {
		t.Error("expected ID to be populated")
	}
}

func TestCreateNoteDuplicateSlug(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	_, err := db.CreateNote("Note A", "same-slug", "body a", wsID)
	if err != nil {
		t.Fatalf("creating first note: %v", err)
	}
	_, err = db.CreateNote("Note B", "same-slug", "body b", wsID)
	if err == nil {
		t.Error("expected error for duplicate slug")
	}
}

func TestGetNoteBySlug(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	created, _ := db.CreateNote("Test Note", "test-note", "content", wsID)

	note, err := db.GetNoteBySlug("test-note")
	if err != nil {
		t.Fatalf("getting note: %v", err)
	}
	if note.ID != created.ID {
		t.Errorf("ID = %q, want %q", note.ID, created.ID)
	}
}

func TestGetNoteBySlugNotFound(t *testing.T) {
	db := testDB(t)

	_, err := db.GetNoteBySlug("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent slug")
	}
}

func TestGetNote(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	created, _ := db.CreateNote("Test", "test", "body", wsID)

	note, err := db.GetNote(created.ID)
	if err != nil {
		t.Fatalf("getting note: %v", err)
	}
	if note.Slug != "test" {
		t.Errorf("slug = %q, want 'test'", note.Slug)
	}
}

func TestListNotes(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	db.CreateNote("Note A", "note-a", "body a", wsID)
	db.CreateNote("Note B", "note-b", "body b", wsID)

	notes, err := db.ListNotes()
	if err != nil {
		t.Fatalf("listing notes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
}

func TestListNotesExcludesArchived(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Archived", "archived", "body", wsID)
	db.CreateNote("Active", "active", "body", wsID)
	db.ArchiveNote(note.ID)

	notes, err := db.ListNotes()
	if err != nil {
		t.Fatalf("listing notes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note (excluding archived), got %d", len(notes))
	}
	if notes[0].Slug != "active" {
		t.Errorf("expected 'active', got %q", notes[0].Slug)
	}
}

func TestUpdateNote(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Original", "original", "old body", wsID)
	note.Title = "Updated"
	note.Body = "new body"
	note.Tags = "go,pkm"

	if err := db.UpdateNote(note); err != nil {
		t.Fatalf("updating note: %v", err)
	}

	got, _ := db.GetNote(note.ID)
	if got.Title != "Updated" {
		t.Errorf("title = %q, want 'Updated'", got.Title)
	}
	if got.Body != "new body" {
		t.Errorf("body = %q, want 'new body'", got.Body)
	}
	if got.Tags != "go,pkm" {
		t.Errorf("tags = %q, want 'go,pkm'", got.Tags)
	}
}

func TestArchiveNote(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Archive Me", "archive-me", "body", wsID)

	if err := db.ArchiveNote(note.ID); err != nil {
		t.Fatalf("archiving note: %v", err)
	}

	_, err := db.GetNote(note.ID)
	if err == nil {
		t.Error("expected archived note to not be found by GetNote")
	}
}

func TestDeleteNote(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	note, _ := db.CreateNote("Delete Me", "delete-me", "body", wsID)

	if err := db.DeleteNote(note.ID); err != nil {
		t.Fatalf("deleting note: %v", err)
	}

	notes, _ := db.ListNotes()
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after delete, got %d", len(notes))
	}
}

func TestSearchNotes(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	db.CreateNote("Go Patterns", "go-patterns", "Learn about Go concurrency", wsID)
	db.CreateNote("Rust Guide", "rust-guide", "Memory safety in Rust", wsID)
	db.CreateNote("Go Testing", "go-testing", "Table driven tests in Go", wsID)

	notes, err := db.SearchNotes("go")
	if err != nil {
		t.Fatalf("searching notes: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes matching 'go', got %d", len(notes))
	}
}

func TestListNotesByTag(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	n1, _ := db.CreateNote("Tagged A", "tagged-a", "body", wsID)
	n1.Tags = "go,tools"
	db.UpdateNote(n1)

	n2, _ := db.CreateNote("Tagged B", "tagged-b", "body", wsID)
	n2.Tags = "go,pkm"
	db.UpdateNote(n2)

	n3, _ := db.CreateNote("Tagged C", "tagged-c", "body", wsID)
	n3.Tags = "rust"
	db.UpdateNote(n3)

	notes, err := db.ListNotesByTag("go")
	if err != nil {
		t.Fatalf("listing by tag: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes with tag 'go', got %d", len(notes))
	}
}

func TestSyncNoteLinks(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	source, _ := db.CreateNote("Source", "source", "Links to [[target-a]] and [[target-b]]", wsID)
	db.CreateNote("Target A", "target-a", "body", wsID)
	db.CreateNote("Target B", "target-b", "body", wsID)

	if err := db.SyncNoteLinks(source); err != nil {
		t.Fatalf("syncing links: %v", err)
	}

	links, err := db.GetForwardLinks("note", source.ID)
	if err != nil {
		t.Fatalf("getting forward links: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 forward links, got %d", len(links))
	}
}

func TestGetBacklinks(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	target, _ := db.CreateNote("Target", "target", "I am the target", wsID)
	source1, _ := db.CreateNote("Source 1", "source-1", "See [[target]] for info", wsID)
	source2, _ := db.CreateNote("Source 2", "source-2", "Also links to [[target]]", wsID)

	db.SyncNoteLinks(source1)
	db.SyncNoteLinks(source2)

	links, err := db.GetBacklinks("note", target.ID)
	if err != nil {
		t.Fatalf("getting backlinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 backlinks, got %d", len(links))
	}
}

func TestSyncNoteLinksUpdatesOnChange(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	source, _ := db.CreateNote("Source", "source", "Links to [[old-target]]", wsID)
	db.CreateNote("Old Target", "old-target", "body", wsID)
	db.CreateNote("New Target", "new-target", "body", wsID)

	db.SyncNoteLinks(source)

	links, _ := db.GetForwardLinks("note", source.ID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link before update, got %d", len(links))
	}

	source.Body = "Now links to [[new-target]]"
	db.UpdateNote(source)
	db.SyncNoteLinks(source)

	links, _ = db.GetForwardLinks("note", source.ID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after update, got %d", len(links))
	}

	newTarget, _ := db.GetNoteBySlug("new-target")
	if links[0].TargetID != newTarget.ID {
		t.Errorf("expected link to new-target, got target_id=%q", links[0].TargetID)
	}
}

func TestSyncNoteLinksHandlesBrokenLinks(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)

	source, _ := db.CreateNote("Source", "source", "Links to [[nonexistent]]", wsID)

	if err := db.SyncNoteLinks(source); err != nil {
		t.Fatalf("syncing links with broken target: %v", err)
	}

	links, _ := db.GetForwardLinks("note", source.ID)
	if len(links) != 1 {
		t.Fatalf("expected 1 link (broken), got %d", len(links))
	}
	if links[0].TargetID != "nonexistent" {
		t.Errorf("expected target_id to be the slug for broken links, got %q", links[0].TargetID)
	}
}
