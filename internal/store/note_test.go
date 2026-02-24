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

	note, err := db.CreateNote("My First Note", "my-first-note", "Hello world")
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

	_, err := db.CreateNote("Note A", "same-slug", "body a")
	if err != nil {
		t.Fatalf("creating first note: %v", err)
	}
	_, err = db.CreateNote("Note B", "same-slug", "body b")
	if err == nil {
		t.Error("expected error for duplicate slug")
	}
}

func TestGetNoteBySlug(t *testing.T) {
	db := testDB(t)

	created, _ := db.CreateNote("Test Note", "test-note", "content")

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

	created, _ := db.CreateNote("Test", "test", "body")

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

	db.CreateNote("Note A", "note-a", "body a")
	db.CreateNote("Note B", "note-b", "body b")

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

	note, _ := db.CreateNote("Archived", "archived", "body")
	db.CreateNote("Active", "active", "body")
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

	note, _ := db.CreateNote("Original", "original", "old body")
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

	note, _ := db.CreateNote("Archive Me", "archive-me", "body")

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

	note, _ := db.CreateNote("Delete Me", "delete-me", "body")

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

	db.CreateNote("Go Patterns", "go-patterns", "Learn about Go concurrency")
	db.CreateNote("Rust Guide", "rust-guide", "Memory safety in Rust")
	db.CreateNote("Go Testing", "go-testing", "Table driven tests in Go")

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

	n1, _ := db.CreateNote("Tagged A", "tagged-a", "body")
	n1.Tags = "go,tools"
	db.UpdateNote(n1)

	n2, _ := db.CreateNote("Tagged B", "tagged-b", "body")
	n2.Tags = "go,pkm"
	db.UpdateNote(n2)

	n3, _ := db.CreateNote("Tagged C", "tagged-c", "body")
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

