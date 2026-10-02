package store

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

func readVaultFile(t *testing.T, db *DB, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(db.Vault().Root(), rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

func writeVaultFile(t *testing.T, db *DB, rel, content string) {
	t.Helper()
	p := filepath.Join(db.Vault().Root(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Scans compare mtime and size; make every write look newer than the
	// last, as a real edit seconds later would.
	future := time.Now().Add(time.Duration(len(content)+1) * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
}

func scan(t *testing.T, db *DB) {
	t.Helper()
	if _, err := db.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestCreateNoteWritesAMarkdownFile(t *testing.T) {
	db := testDB(t)
	note, err := db.CreateNote("Dual Transformation", "dual-transformation", "Body [[other]]", testDefaultWSID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	// Named by its title, as Obsidian names notes.
	if note.Path != "Dual Transformation.md" || note.Slug != "dual-transformation" {
		t.Errorf("path = %q, slug = %q", note.Path, note.Slug)
	}
	content := readVaultFile(t, db, "Dual Transformation.md")
	for _, want := range []string{"id: " + note.ID, "created: ", "---\nBody [[other]]"} {
		if !strings.Contains(content, want) {
			t.Errorf("file lacks %q:\n%s", want, content)
		}
	}
	// The file name is the title, so neither the title nor the default
	// workspace is repeated inside it.
	for _, unwanted := range []string{"title:", "workspace:"} {
		if strings.Contains(content, unwanted) {
			t.Errorf("file has redundant %q:\n%s", unwanted, content)
		}
	}
}

func TestCreateNoteKeepsTitlesFileNamesCannotHold(t *testing.T) {
	db := testDB(t)
	note, err := db.CreateNote("C++: primer", "c-primer", "", testDefaultWSID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if note.Path != "C++ primer.md" || note.Title != "C++: primer" {
		t.Errorf("note = %+v", note)
	}
	if !strings.Contains(readVaultFile(t, db, "C++ primer.md"), `title: 'C++: primer'`) {
		t.Errorf("exact title not kept:\n%s", readVaultFile(t, db, "C++ primer.md"))
	}
}

func TestCreateNoteWithACustomSlugNamesTheFileAfterIt(t *testing.T) {
	db := testDB(t)
	note, err := db.CreateNote("Quarterly Planning Notes", "q3", "", testDefaultWSID(t, db))
	if err != nil {
		t.Fatal(err)
	}
	if note.Path != "q3.md" || note.Slug != "q3" || note.Title != "Quarterly Planning Notes" {
		t.Errorf("note = %+v", note)
	}
}

func TestCreateNoteRefusesAnExistingFile(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "taken.md", "---\ntitle: Something Else\n---\nmine")
	scan(t, db)
	// The index has slug "taken" for this file, so this is caught as a slug
	// clash before the file is touched.
	if _, err := db.CreateNote("Taken", "taken", "theirs", testDefaultWSID(t, db)); err == nil {
		t.Fatal("expected an error")
	}
	if got := readVaultFile(t, db, "taken.md"); !strings.HasSuffix(got, "mine") {
		t.Errorf("file overwritten: %q", got)
	}
}

func TestScanIndexesForeignFilesWithoutTouchingThem(t *testing.T) {
	db := testDB(t)
	original := "Plain Obsidian note with a [[Link]].\n"
	writeVaultFile(t, db, "3 Horizons of Growth.md", original)
	writeVaultFile(t, db, ".obsidian/app.md", "ignored")

	res, err := db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 {
		t.Errorf("added = %d", res.Added)
	}
	note, err := db.GetNoteBySlug("3-horizons-of-growth")
	if err != nil {
		t.Fatal(err)
	}
	if note.Title != "3 Horizons of Growth" || note.Body != original || note.Path != "3 Horizons of Growth.md" {
		t.Errorf("note = %+v", note)
	}
	if note.WorkspaceID != testDefaultWSID(t, db) {
		t.Errorf("workspace = %q", note.WorkspaceID)
	}
	if got := readVaultFile(t, db, "3 Horizons of Growth.md"); got != original {
		t.Errorf("scan modified the file: %q", got)
	}

	// The id is derived from the path, so it is stable across scans and
	// rebuilt indexes.
	again := testDBWithVault(t, db.Vault().Root())
	other, err := again.GetNoteBySlug("3-horizons-of-growth")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID != note.ID {
		t.Errorf("id changed between indexes: %s vs %s", note.ID, other.ID)
	}
}

func TestScanPicksUpEditsAndDeletions(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Edit Me", "edit-me", "v1", testDefaultWSID(t, db))

	content := strings.Replace(readVaultFile(t, db, "Edit Me.md"), "v1", "v2 edited in vim", 1)
	writeVaultFile(t, db, "Edit Me.md", content)
	res, err := db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Errorf("updated = %d", res.Updated)
	}
	got, _ := db.GetNote(note.ID)
	if got.Body != "v2 edited in vim" {
		t.Errorf("body = %q", got.Body)
	}

	os.Remove(filepath.Join(db.Vault().Root(), "Edit Me.md"))
	res, err = db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 {
		t.Errorf("removed = %d", res.Removed)
	}
	if _, err := db.GetNote(note.ID); err == nil {
		t.Error("note should be gone from the index")
	}
}

func TestScanSkipsUnchangedFiles(t *testing.T) {
	db := testDB(t)
	db.CreateNote("Same", "same", "x", testDefaultWSID(t, db))
	res, err := db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Added+res.Updated+res.Removed != 0 {
		t.Errorf("unchanged vault produced changes: %+v", res)
	}
}

func TestRenamedFileKeepsItsID(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Moving", "moving", "x", testDefaultWSID(t, db))
	root := db.Vault().Root()
	os.MkdirAll(filepath.Join(root, "archive"), 0o755)
	if err := os.Rename(filepath.Join(root, "Moving.md"), filepath.Join(root, "archive", "moved.md")); err != nil {
		t.Fatal(err)
	}
	scan(t, db)
	got, err := db.GetNote(note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "archive/moved.md" || got.Slug != "moved" {
		t.Errorf("path=%q slug=%q", got.Path, got.Slug)
	}
}

func TestCopiedFileGetsItsOwnID(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Original", "original", "x", testDefaultWSID(t, db))
	writeVaultFile(t, db, "copy.md", readVaultFile(t, db, "Original.md"))
	scan(t, db)
	notes, _ := db.ListNotes()
	if len(notes) != 2 {
		t.Fatalf("notes = %d", len(notes))
	}
	if notes[0].ID == notes[1].ID {
		t.Error("copy shares the original's id")
	}
	if got, _ := db.GetNote(note.ID); got.Path != "Original.md" {
		t.Errorf("original moved to %q", got.Path)
	}
}

func TestSlugCollisionsGetASuffix(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Foo Bar.md", "a")
	writeVaultFile(t, db, "sub/foo-bar.md", "b")
	writeVaultFile(t, db, "日本.md", "c")
	scan(t, db)
	notes, _ := db.ListNotes()
	slugs := map[string]bool{}
	for _, n := range notes {
		slugs[n.Slug] = true
	}
	if !slugs["foo-bar"] || !slugs["foo-bar-2"] || len(slugs) != 3 {
		t.Errorf("slugs = %v", slugs)
	}
}

func TestFrontmatterFieldsAreIndexed(t *testing.T) {
	db := testDB(t)
	db.CreateWorkspace("school", "area", "", "")
	writeVaultFile(t, db, "n.md", `---
title: Six Paths Framework
tags: [strategy, innovation]
pinned: true
workspace: school
created: 2026-05-13
---
body`)
	scan(t, db)
	note, err := db.GetNoteBySlug("n")
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := db.GetWorkspaceByName("school")
	if note.Title != "Six Paths Framework" || note.Tags != "strategy,innovation" || !note.Pinned || note.WorkspaceID != ws.ID {
		t.Errorf("note = %+v", note)
	}
	if note.CreatedAt.Format("2006-01-02") != "2026-05-13" {
		t.Errorf("created = %v", note.CreatedAt)
	}
}

func TestUpdateNoteRewritesTheFileAndKeepsUnknownKeys(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Foreign.md", "---\ncssclass: wide\n---\nold body")
	scan(t, db)
	note, _ := db.GetNoteBySlug("foreign")
	note.Body = "new body"
	note.Tags = "a, b"
	note.Pinned = true
	if err := db.UpdateNote(note); err != nil {
		t.Fatal(err)
	}
	content := readVaultFile(t, db, "Foreign.md")
	for _, want := range []string{"cssclass: wide", "id: " + note.ID, "tags: [a, b]", "pinned: true", "---\nnew body"} {
		if !strings.Contains(content, want) {
			t.Errorf("file lacks %q:\n%s", want, content)
		}
	}
	// The title is the file name, so writing it would only add noise.
	if strings.Contains(content, "title:") {
		t.Errorf("redundant title written:\n%s", content)
	}
}

func TestArchiveNoteMarksTheFileAndHidesIt(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Old", "old", "x", testDefaultWSID(t, db))
	if err := db.ArchiveNote(note.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readVaultFile(t, db, "Old.md"), "archived: ") {
		t.Error("file not marked archived")
	}
	notes, _ := db.ListNotes()
	if len(notes) != 0 {
		t.Errorf("archived note still listed")
	}
	// It stays archived after the index is rebuilt from the files.
	again := testDBWithVault(t, db.Vault().Root())
	if notes, _ := again.ListNotes(); len(notes) != 0 {
		t.Error("archive lost on rebuild")
	}
}

func TestDeleteNoteRemovesFileLinksAndPublishLog(t *testing.T) {
	db := testDB(t)
	wsID := testDefaultWSID(t, db)
	note, _ := db.CreateNote("Published", "published", "see [[other]]", wsID)
	db.SyncNoteLinks(note)
	target, err := db.CreatePublishTarget("blog", model.EngineJekyll, t.TempDir(), "_posts", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePublishLog(note.ID, target.ID, "_posts/x.md", ""); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteNote(note.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(db.Vault().Root(), "Published.md")); !os.IsNotExist(err) {
		t.Error("file still exists")
	}
	if links, _ := db.GetForwardLinks("note", note.ID); len(links) != 0 {
		t.Errorf("links left: %d", len(links))
	}
}

func TestLegacyNotesAreExportedOnOpen(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "kb.db")
	vaultDir := filepath.Join(dir, "notes")

	db, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	wsID := testDefaultWSID(t, db)
	// A note as the old SQLite-only kb stored it: no file, no path.
	if _, err := db.conn.Exec(
		`INSERT INTO notes (id, title, slug, body, tags, pinned, workspace_id, created_at, updated_at)
		 VALUES ('legacy-1', 'Old Note', 'old-note', 'kept body', 'x,y', 1, ?, '2026-03-06 10:00:00', '2026-03-06 10:00:00')`,
		wsID,
	); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db, err = OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	content := readVaultFile(t, db, "Old Note.md")
	for _, want := range []string{"id: legacy-1", "tags: [x, y]", "pinned: true", "created: 2026-03-06T10:00:00Z", "---\nkept body"} {
		if !strings.Contains(content, want) {
			t.Errorf("exported file lacks %q:\n%s", want, content)
		}
	}
	note, err := db.GetNote("legacy-1")
	if err != nil || note.Path != "Old Note.md" {
		t.Fatalf("note = %+v err = %v", note, err)
	}
	// The file carries the note's last-modified time, so lists keep their
	// order and a rebuilt index agrees.
	if got := note.UpdatedAt.UTC().Format(time.DateTime); got != "2026-03-06 10:00:00" {
		t.Errorf("updated = %s", got)
	}
}

func TestBadFrontmatterStillIndexesTheNote(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "broken.md", "---\n: : [\n---\ntext")
	res, err := db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 1 || !strings.Contains(res.Problems[0], "broken.md") {
		t.Errorf("problems = %v", res.Problems)
	}
	if _, err := db.GetNoteBySlug("broken"); err != nil {
		t.Errorf("broken note not indexed: %v", err)
	}
}

func TestRenamingAWorkspaceRewritesItsNotes(t *testing.T) {
	db := testDB(t)
	ws, _ := db.CreateWorkspace("school", "area", "", "")
	db.CreateNote("Lecture", "lecture", "x", ws.ID)
	ws.Name = "university"
	if err := db.UpdateWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readVaultFile(t, db, "Lecture.md"), "workspace: university") {
		t.Error("note still names the old workspace")
	}
}

func TestTwoProcessesCanWriteAtOnce(t *testing.T) {
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "kb.db")
	vaultDir := filepath.Join(dir, "notes")
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

	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := range 20 {
		for _, db := range []*DB{a, b} {
			wg.Add(1)
			go func(db *DB, i int) {
				defer wg.Done()
				slug := filepath.Base(dir) + "-" + string(rune('a'+i))
				if db == b {
					slug += "-b"
				}
				_, err := db.CreateNote(slug, slug, "x", wsID)
				errs <- err
			}(db, i)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent write: %v", err)
		}
	}
}

func TestStaleSaveDoesNotOverwriteAnOutsideEdit(t *testing.T) {
	db := testDB(t)
	stale, _ := db.CreateNote("Shared", "shared", "v1", testDefaultWSID(t, db))
	// Edited in another editor after kb read it, with no scan in between.
	edited := strings.Replace(readVaultFile(t, db, "Shared.md"), "v1", "edited elsewhere", 1)
	writeVaultFile(t, db, "Shared.md", edited)

	stale.Body = "kb's stale copy"
	if err := db.UpdateNote(stale); err == nil {
		t.Fatal("expected a conflict error")
	}
	if got := readVaultFile(t, db, "Shared.md"); !strings.Contains(got, "edited elsewhere") {
		t.Fatalf("outside edit lost:\n%s", got)
	}
	// The conflict refreshed the index, so a re-read sees the edit.
	fresh, _ := db.GetNote(stale.ID)
	if fresh.Body != "edited elsewhere" {
		t.Errorf("index not refreshed: %q", fresh.Body)
	}
	fresh.Body = "now it saves"
	if err := db.UpdateNote(fresh); err != nil {
		t.Fatalf("fresh save: %v", err)
	}
}

func TestMoveToWorkspaceKeepsAnOutsideEdit(t *testing.T) {
	db := testDB(t)
	ws, _ := db.CreateWorkspace("school", "area", "", "")
	note, _ := db.CreateNote("Moving", "moving-ws", "v1", testDefaultWSID(t, db))
	edited := strings.Replace(readVaultFile(t, db, "moving-ws.md"), "v1", "edited elsewhere", 1)
	writeVaultFile(t, db, "moving-ws.md", edited)

	if err := db.SetNoteWorkspace(note.ID, ws.ID); err != nil {
		t.Fatal(err)
	}
	got := readVaultFile(t, db, "moving-ws.md")
	if !strings.Contains(got, "edited elsewhere") || !strings.Contains(got, "workspace: school") {
		t.Errorf("file:\n%s", got)
	}
}

func TestDeleteRefusesAFileThatIsNowAnotherNote(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Old", "swap", "x", testDefaultWSID(t, db))
	writeVaultFile(t, db, "swap.md", "---\nid: someone-else\n---\nother note")
	if err := db.DeleteNote(note.ID); err == nil {
		t.Fatal("expected an error")
	}
	if got := readVaultFile(t, db, "swap.md"); !strings.Contains(got, "other note") {
		t.Errorf("the other note's file was removed or changed: %q", got)
	}
}

func TestBrokenFrontmatterKeepsTheNotesIdentity(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Mid Edit", "mid-edit", "x", testDefaultWSID(t, db))
	writeVaultFile(t, db, "mid-edit.md", "---\nid: "+note.ID+"\ntitle: [unclosed\n---\nx")
	scan(t, db)
	if _, err := db.GetNote(note.ID); err != nil {
		t.Errorf("note lost its id while its YAML was broken: %v", err)
	}
}

func TestAnEmptyVaultDoesNotWipeTheIndex(t *testing.T) {
	db := testDB(t)
	note, _ := db.CreateNote("Safe", "safe", "x", testDefaultWSID(t, db))
	db.CreateNote("Also Safe", "also-safe", "x", testDefaultWSID(t, db))
	os.Remove(filepath.Join(db.Vault().Root(), "Safe.md"))
	os.Remove(filepath.Join(db.Vault().Root(), "Also Safe.md"))
	res, err := db.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 0 || len(res.Problems) == 0 {
		t.Errorf("res = %+v", res)
	}
	if _, err := db.GetNote(note.ID); err != nil {
		t.Errorf("index wiped: %v", err)
	}
}

func TestAnUnreadableFolderDoesNotDropItsNotes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a folder with no permissions, so nothing is unreadable")
	}
	db := testDB(t)
	db.CreateNote("Keep", "keep", "x", testDefaultWSID(t, db))
	writeVaultFile(t, db, "locked/inner.md", "y")
	scan(t, db)
	locked := filepath.Join(db.Vault().Root(), "locked")
	os.Chmod(locked, 0o000)
	defer os.Chmod(locked, 0o755)

	res, err := db.Scan()
	if err != nil {
		t.Fatalf("scan failed outright: %v", err)
	}
	if res.Removed != 0 || len(res.Problems) == 0 {
		t.Errorf("res = %+v", res)
	}
	if _, err := db.GetNoteBySlug("inner"); err != nil {
		t.Errorf("note in unreadable folder dropped: %v", err)
	}
}

func TestLegacyExportAdoptsAFileItAlreadyWrote(t *testing.T) {
	dir := t.TempDir()
	dbFile, vaultDir := filepath.Join(dir, "kb.db"), filepath.Join(dir, "notes")
	db, err := OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	db.conn.Exec(`INSERT INTO notes (id, title, slug, body, workspace_id, created_at, updated_at)
		VALUES ('legacy-2', 'Half Done', 'half-done', 'b', ?, '2026-03-06 10:00:00', '2026-03-06 10:00:00')`,
		testDefaultWSID(t, db))
	db.Close()
	// A crash after the file was written but before the index recorded it.
	os.WriteFile(filepath.Join(vaultDir, "Half Done.md"), []byte("---\nid: legacy-2\ntitle: Half Done\n---\nb"), 0o644)

	db, err = OpenWithPath(dbFile, vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, _ := filepath.Glob(filepath.Join(vaultDir, "Half Done*.md"))
	if len(files) != 1 {
		t.Errorf("files = %v", files)
	}
}

func TestANewFileAtARenamedNotesOldPathIsANewNote(t *testing.T) {
	db := testDB(t)
	writeVaultFile(t, db, "Foo.md", "the original")
	scan(t, db)
	orig, _ := db.GetNoteBySlug("foo")
	if _, _, err := db.RenameNote(orig.ID, "Zzz"); err != nil {
		t.Fatal(err)
	}
	writeVaultFile(t, db, "Foo.md", "someone new")
	scan(t, db)

	renamed, err := db.GetNoteBySlug("zzz")
	if err != nil || renamed.ID != orig.ID || renamed.Body != "the original" {
		t.Fatalf("renamed note lost its identity: %+v, %v", renamed, err)
	}
	newcomer, err := db.GetNoteBySlug("foo")
	if err != nil || newcomer.ID == orig.ID || newcomer.Body != "someone new" {
		t.Fatalf("newcomer = %+v, %v", newcomer, err)
	}

	// A rebuilt index agrees, though "Foo.md" is walked before "Zzz.md":
	// the id written in a file wins over one derived from a path.
	again := testDBWithVault(t, db.Vault().Root())
	if r, _ := again.GetNoteBySlug("zzz"); r == nil || r.ID != orig.ID {
		t.Errorf("after rebuild the renamed note has id %v, want %s", r, orig.ID)
	}
	if n, _ := again.GetNoteBySlug("foo"); n == nil || n.ID != newcomer.ID {
		t.Errorf("after rebuild the newcomer has id %v, want %s", n, newcomer.ID)
	}
}
