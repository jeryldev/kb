package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameNoteMovesTheFileAndRewritesLinksToIt(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "ideas/Old Name.md", "---\ncssclass: wide\n---\nthe note")
	writeVaultFile(t, db, "a.md", "see [[Old Name]] and [[old name#Part 2|that part]], not [[Old Names]]")
	writeVaultFile(t, db, "b.md", "also [[ideas/Old Name^blk]]\nand `[[Old Name]]` in code stays a link too")
	writeVaultFile(t, db, "c.md", "unrelated [[Elsewhere]]")
	scan(t, db)
	note, _ := db.GetNoteBySlug("old-name")
	cBefore := readVaultFile(t, db, "c.md")

	renamed, changed, err := db.RenameNote(note.ID, "New Name")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != note.ID || renamed.Path != "ideas/New Name.md" || renamed.Title != "New Name" || renamed.Slug != "new-name" {
		t.Errorf("renamed = %+v", renamed)
	}
	if changed != 2 {
		t.Errorf("changed = %d, want 2", changed)
	}
	if _, err := os.Stat(filepath.Join(db.Vault().Root(), "ideas", "Old Name.md")); !os.IsNotExist(err) {
		t.Error("old file still there")
	}
	moved := readVaultFile(t, db, "ideas/New Name.md")
	if !strings.Contains(moved, "cssclass: wide") || strings.Contains(moved, "title:") || !strings.HasSuffix(moved, "the note") {
		t.Errorf("moved file:\n%s", moved)
	}
	if got := readVaultFile(t, db, "a.md"); got != "see [[New Name]] and [[New Name#Part 2|that part]], not [[Old Names]]" {
		t.Errorf("a.md = %q", got)
	}
	if got := readVaultFile(t, db, "b.md"); !strings.HasPrefix(got, "also [[New Name^blk]]\nand `[[New Name]]`") {
		t.Errorf("b.md = %q", got)
	}
	if readVaultFile(t, db, "c.md") != cBefore {
		t.Error("unrelated note rewritten")
	}
	assertLinked(t, db, "new-name", "a", "b")
}

func TestRenameRefusesATakenName(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)
	note, _ := db.CreateNote("First", "first", "x", wsID)
	db.CreateNote("Second", "second", "y", wsID)
	if _, _, err := db.RenameNote(note.ID, "Second"); err == nil {
		t.Fatal("expected an error")
	}
	if got := readVaultFile(t, db, "First.md"); !strings.HasSuffix(got, "x") {
		t.Errorf("first.md changed: %q", got)
	}
}

func TestRenameToTheSameSlugOnlyRetitles(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("draft", "draft", "x", testDefaultWSID(t, db))
	renamed, _, err := db.RenameNote(note.ID, "Draft")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Path != "draft.md" || renamed.Title != "Draft" {
		t.Errorf("renamed = %+v", renamed)
	}
}

func TestCreateNoteAtAPath(t *testing.T) {
	db := testDB(t)
	note, err := db.CreateNoteAt("daily/2026-10-02.md", "2026-10-02", "", testDefaultWSID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if note.Slug != "2026-10-02" || note.Path != "daily/2026-10-02.md" {
		t.Errorf("note = %+v", note)
	}
	// The title is the file name, so it is not repeated in the file.
	if got := readVaultFile(t, db, "daily/2026-10-02.md"); strings.Contains(got, "title:") {
		t.Errorf("file:\n%s", got)
	}
	if _, err := db.CreateNoteAt("daily/2026-10-02.md", "again", "", testDefaultWSID(t, db)); err == nil {
		t.Error("expected an error for an existing file")
	}
	if _, err := db.CreateNoteAt("../outside.md", "x", "", testDefaultWSID(t, db)); err == nil {
		t.Error("expected an error for a path outside the vault")
	}
}

func TestResolveNoteRefByAnyName(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "dt.md", "---\ntitle: Dual Transformation\naliases: [DT]\n---\n")
	scan(t, db)
	for _, ref := range []string{"dt", "Dual Transformation", "dual transformation", "DT", "dt.md"} {
		note, err := db.ResolveNoteRef(ref)
		if err != nil || note.Slug != "dt" {
			t.Errorf("ResolveNoteRef(%q) = %v, %v", ref, note, err)
		}
	}
	if _, err := db.ResolveNoteRef("nothing like it"); err == nil {
		t.Error("expected an error")
	}
}

func TestListTagsCountsNotes(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "a.md", "---\ntags: [go, tools]\n---\n")
	writeVaultFile(t, db, "b.md", "---\ntags: [Go]\n---\n")
	writeVaultFile(t, db, "c.md", "---\ntags: [rust]\narchived: 2026-01-01\n---\n")
	scan(t, db)
	tags, err := db.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, tc := range tags {
		got[tc.Tag] = tc.Count
	}
	if len(got) != 2 || got["go"] != 2 || got["tools"] != 1 {
		t.Errorf("tags = %+v", tags)
	}
	if tags[0].Tag != "go" {
		t.Errorf("most used first, got %+v", tags)
	}
}

func TestRenameRefusesATitleAnotherNoteAnswersTo(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "foo.md", "---\ntitle: Plan\n---\n")
	writeVaultFile(t, db, "aliased.md", "---\naliases: [Roadmap]\n---\n")
	writeVaultFile(t, db, "bar.md", "x")
	scan(t, db)
	bar, _ := db.GetNoteBySlug("bar")
	for _, title := range []string{"plan", "Roadmap"} {
		if _, _, err := db.RenameNote(bar.ID, title); err == nil {
			t.Errorf("rename to %q should fail: another note already answers to it", title)
		}
	}
}

func TestRenameLeavesAliasAndCardLinksAlone(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Old Name.md", "---\naliases: [ON]\n---\n")
	writeVaultFile(t, db, "card-foo.md", "")
	writeVaultFile(t, db, "src.md", "[[Old Name]] [[ON]] [[card:foo]]")
	scan(t, db)
	note, _ := db.GetNoteBySlug("old-name")
	if _, _, err := db.RenameNote(note.ID, "New Name"); err != nil {
		t.Fatal(err)
	}
	if got := readVaultFile(t, db, "src.md"); got != "[[New Name]] [[ON]] [[card:foo]]" {
		t.Errorf("src.md = %q", got)
	}
}

func TestRenameToADifferentCaseOfTheSameName(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Draft.md", "body")
	scan(t, db)
	note, _ := db.GetNoteBySlug("draft")
	renamed, _, err := db.RenameNote(note.ID, "draft")
	if err != nil {
		t.Fatalf("title-only rename failed: %v", err)
	}
	if renamed.Title != "draft" || !strings.HasSuffix(readVaultFile(t, db, renamed.Path), "body") {
		t.Errorf("renamed = %+v", renamed)
	}
	if notes, _ := db.ListNotes(); len(notes) != 1 {
		t.Errorf("notes = %d, want 1", len(notes))
	}
}
