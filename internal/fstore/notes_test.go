package fstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/vault"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return openAt(t, t.TempDir())
}

func openAt(t *testing.T, vaultDir string) *Store {
	t.Helper()
	s, err := Open(vaultDir, Options{ConfigDir: t.TempDir(), LockDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// sibling opens the same vault as s with the same lock and config
// directories: another kb process on the same machine.
func sibling(t *testing.T, s *Store) *Store {
	t.Helper()
	other, err := Open(s.Vault().Root(), s.opts)
	if err != nil {
		t.Fatal(err)
	}
	return other
}

func put(t *testing.T, s *Store, rel, content string) {
	t.Helper()
	p := filepath.Join(s.Vault().Root(), filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, s *Store, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.Vault().Root(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

func reload(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
}

func slugs(notes []*Note) []string {
	var out []string
	for _, n := range notes {
		out = append(out, n.Slug)
	}
	sort.Strings(out)
	return out
}

func TestCreateNoteNamesTheFileAfterTheTitle(t *testing.T) {
	s := testStore(t)
	n, err := s.CreateNote("Dual Transformation", "", "Body [[other]]", "")
	if err != nil {
		t.Fatal(err)
	}
	if n.Path != "Dual Transformation.md" || n.Slug != "dual-transformation" {
		t.Errorf("note = %+v", n)
	}
	content := read(t, s, "Dual Transformation.md")
	if !strings.Contains(content, "id: "+n.ID) || strings.Contains(content, "title:") || !strings.HasSuffix(content, "Body [[other]]") {
		t.Errorf("file:\n%s", content)
	}
	if c, _ := s.CreateNote("C++: primer", "", "", ""); c.Path != "C++ primer.md" || c.Title != "C++: primer" {
		t.Errorf("sanitised note = %+v", c)
	}
	if q, _ := s.CreateNote("Quarterly Planning", "q3", "", ""); q.Path != "q3.md" {
		t.Errorf("custom slug note = %+v", q)
	}
	if _, err := s.CreateNote("Dual Transformation", "", "", ""); err == nil {
		t.Error("a second note with the same name should be refused")
	}
}

func TestReadingNeverChangesAFile(t *testing.T) {
	s := testStore(t)
	original := "Plain note with a [[Link]].\n"
	put(t, s, "3 Horizons of Growth.md", original)
	put(t, s, ".obsidian/app.md", "not a note")
	reload(t, s)
	n, err := s.GetNoteBySlug("3-horizons-of-growth")
	if err != nil || n.Title != "3 Horizons of Growth" || n.Body != original {
		t.Fatalf("note = %+v, %v", n, err)
	}
	if read(t, s, "3 Horizons of Growth.md") != original {
		t.Error("loading modified the file")
	}
	again := openAt(t, s.Vault().Root())
	if m, _ := again.GetNoteBySlug("3-horizons-of-growth"); m == nil || m.ID != n.ID {
		t.Error("a path-derived id must be the same on every load")
	}
}

func TestAnIcloudConflictCopyDoesNotStealTheID(t *testing.T) {
	s := testStore(t)
	n, _ := s.CreateNote("Note", "", "original", "")
	// iCloud names a conflicting copy "Note 2.md", which sorts first.
	put(t, s, "Note 2.md", read(t, s, "Note.md"))
	reload(t, s)
	orig, err := s.GetNote(n.ID)
	if err != nil || orig.Path != "Note.md" {
		t.Fatalf("id went to %+v (%v)", orig, err)
	}
	if !strings.Contains(strings.Join(s.Problems(), "\n"), "Note 2.md") {
		t.Errorf("duplicate id not reported: %v", s.Problems())
	}
	// The copy is a note of its own, and kb can edit it.
	copyNote, err := s.GetNoteByPath("Note 2.md")
	if err != nil || copyNote.ID == n.ID {
		t.Fatalf("copy = %+v, %v", copyNote, err)
	}
	copyNote.Body = "the copy, edited"
	if err := s.UpdateNote(copyNote); err != nil {
		t.Errorf("editing the copy: %v", err)
	}
	if !strings.Contains(read(t, s, "Note.md"), "original") {
		t.Error("editing the copy changed the original")
	}
}

func TestSlugsAreTheSameWhateverTheOrder(t *testing.T) {
	s := testStore(t)
	put(t, s, "sub/deeper/foo-bar.md", "c")
	put(t, s, "Foo Bar.md", "a")
	put(t, s, "sub/Foo Bar.md", "b")
	reload(t, s)
	short, _ := s.GetNoteByPath("Foo Bar.md")
	if short.Slug != "foo-bar" {
		t.Errorf("the shortest path should keep the plain slug, got %q", short.Slug)
	}
	if got := strings.Join(slugs(s.ListNotes()), ","); got != "foo-bar,foo-bar-2,foo-bar-3" {
		t.Errorf("slugs = %s", got)
	}
}

func TestOutsideEditsMovesAndDeletesAreSeenOnReload(t *testing.T) {
	s := testStore(t)
	n, _ := s.CreateNote("Edit Me", "", "v1", "")
	put(t, s, "Edit Me.md", strings.Replace(read(t, s, "Edit Me.md"), "v1", "v2 from vim", 1))
	reload(t, s)
	if got, _ := s.GetNote(n.ID); got.Body != "v2 from vim" {
		t.Errorf("body = %q", got.Body)
	}
	os.MkdirAll(filepath.Join(s.Vault().Root(), "archive"), 0o755)
	os.Rename(filepath.Join(s.Vault().Root(), "Edit Me.md"), filepath.Join(s.Vault().Root(), "archive", "Moved.md"))
	reload(t, s)
	if got, err := s.GetNote(n.ID); err != nil || got.Path != "archive/Moved.md" {
		t.Errorf("after a move: %+v, %v", got, err)
	}
	os.Remove(filepath.Join(s.Vault().Root(), "archive", "Moved.md"))
	reload(t, s)
	if _, err := s.GetNote(n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after a delete: %v", err)
	}
}

func TestFrontmatterIsRead(t *testing.T) {
	s := testStore(t)
	put(t, s, "n.md", "---\ntitle: Six Paths\ntags: [strategy, innovation]\npinned: true\nworkspace: school\naliases: Six Paths Framework\ncreated: 2026-05-13\n---\nbody")
	reload(t, s)
	n, _ := s.GetNoteBySlug("n")
	ws, err := s.GetWorkspaceByName("school")
	if err != nil {
		t.Fatalf("a workspace a note names should exist: %v", err)
	}
	if n.Title != "Six Paths" || n.Tags != "strategy,innovation" || !n.Pinned || n.WorkspaceID != ws.ID {
		t.Errorf("note = %+v", n)
	}
	if n.CreatedAt.In(time.Local).Format("2006-01-02") != "2026-05-13" {
		t.Errorf("created = %v", n.CreatedAt)
	}
}

func TestEditingKeepsKeysKbDoesNotOwn(t *testing.T) {
	s := testStore(t)
	put(t, s, "Foreign.md", "---\ncssclass: wide\nworkspace: nowhere-known\n---\nold body")
	reload(t, s)
	n, _ := s.GetNoteBySlug("foreign")
	n.Body = "new body"
	n.Tags = "a, b"
	if err := s.UpdateNote(n); err != nil {
		t.Fatal(err)
	}
	content := read(t, s, "Foreign.md")
	for _, want := range []string{"cssclass: wide", "workspace: nowhere-known", "tags: [a, b]", "---\nnew body"} {
		if !strings.Contains(content, want) {
			t.Errorf("file lacks %q:\n%s", want, content)
		}
	}
}

func TestAStaleSaveIsRefused(t *testing.T) {
	s := testStore(t)
	n, _ := s.CreateNote("Shared", "", "v1", "")
	other := sibling(t, s)
	put(t, other, "Shared.md", strings.Replace(read(t, s, "Shared.md"), "v1", "edited elsewhere", 1))
	reload(t, other)

	n.Body = "stale copy"
	if err := s.UpdateNote(n); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	if !strings.Contains(read(t, s, "Shared.md"), "edited elsewhere") {
		t.Error("the outside edit was overwritten")
	}
	fresh, _ := s.GetNote(n.ID)
	fresh.Body = "now it saves"
	if err := s.UpdateNote(fresh); err != nil {
		t.Errorf("a fresh save: %v", err)
	}
}

func TestArchiveAndTrash(t *testing.T) {
	s := testStore(t)
	old, _ := s.CreateNote("Old", "", "x", "")
	if err := s.ArchiveNote(old.ID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, s, "Old.md"), "archived: ") || len(s.ListNotes()) != 0 {
		t.Error("archive not written or note still listed")
	}
	put(t, s, "Flag.md", "---\narchived: true\n---\n")
	reload(t, s)
	if len(s.ListNotes()) != 0 {
		t.Error("archived: true should hide the note")
	}

	gone, _ := s.CreateNote("Gone", "", "x", "")
	where, err := s.TrashNote(gone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if where != ".trash/Gone.md" || read(t, s, ".trash/Gone.md") == "" {
		t.Errorf("trashed to %q", where)
	}
	if _, err := s.GetNote(gone.ID); !errors.Is(err, ErrNotFound) {
		t.Error("a trashed note is still listed")
	}
	again, _ := s.CreateNote("Gone", "", "y", "")
	if where, _ := s.TrashNote(again.ID); where != ".trash/Gone 2.md" {
		t.Errorf("second trash went to %q", where)
	}
}

func TestANoteWithBrokenYAMLIsListedAndCanBeTrashed(t *testing.T) {
	s := testStore(t)
	put(t, s, "broken.md", "---\n: : [\n---\ntext")
	reload(t, s)
	if !strings.Contains(strings.Join(s.Problems(), "\n"), "broken.md") {
		t.Errorf("problems = %v", s.Problems())
	}
	n, err := s.GetNoteBySlug("broken")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TrashNote(n.ID); err != nil {
		t.Errorf("trash: %v", err)
	}
}

func TestResolveNoteTriesNamesBeforeIDPrefixes(t *testing.T) {
	s := testStore(t)
	put(t, s, "Coffee.md", "---\naliases: [cafe]\n---\n")
	put(t, s, "Other.md", "---\nid: cafe1234-0000-0000-0000-000000000000\n---\n")
	reload(t, s)
	n, err := s.ResolveNote("cafe")
	if err != nil || n.Slug != "coffee" {
		t.Errorf("ResolveNote(cafe) = %+v, %v", n, err)
	}
	if n, _ := s.ResolveNote("cafe1234"); n == nil || n.Slug != "other" {
		t.Errorf("id prefix: %+v", n)
	}
}

func TestSearchMatchesWordPrefixesAccentsTagsAndAllWords(t *testing.T) {
	s := testStore(t)
	put(t, s, "Cafe.md", "Meeting at the café about strategic foresight")
	put(t, s, "Go.md", "---\ntags: [tools]\n---\nTable driven tests in Go")
	put(t, s, "Catalog.md", "a catalog of things")
	put(t, s, "Hidden.md", "---\narchived: 2026-01-01\n---\nstrategic plans")
	reload(t, s)
	for query, want := range map[string]string{
		"cafe":       "cafe",
		"strat fore": "cafe",
		"tests go":   "go",
		"tools":      "go",
		"log":        "",
		"go nothing": "",
		`"(* OR -`:   "",
	} {
		got := strings.Join(slugs(s.SearchNotes(query)), ",")
		if got != want {
			t.Errorf("SearchNotes(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestTags(t *testing.T) {
	s := testStore(t)
	put(t, s, "a.md", "---\ntags: [go, tools]\n---\n")
	put(t, s, "b.md", "---\ntags: [Go]\n---\n")
	reload(t, s)
	tags := s.ListTags()
	if len(tags) != 2 || tags[0].Tag != "go" || tags[0].Count != 2 {
		t.Errorf("tags = %+v", tags)
	}
}

// kb writes the workspace key only when the note's workspace changes, so
// a file keeps the spelling it uses for a workspace's name.
func TestEditingKeepsTheFilesSpellingOfItsWorkspace(t *testing.T) {
	s := testStore(t)
	if _, err := s.CreateWorkspace("School", "area", "", ""); err != nil {
		t.Fatal(err)
	}
	put(t, s, "Lecture.md", "---\nworkspace: school\n---\nv1")
	reload(t, s)
	n, _ := s.GetNoteBySlug("lecture")
	ws, _ := s.GetWorkspaceByName("School")
	if n.WorkspaceID != ws.ID {
		t.Fatalf("workspace = %q, want School's", n.WorkspaceID)
	}
	n.Body = "v2"
	if err := s.UpdateNote(n); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, s, "Lecture.md"), "workspace: school") {
		t.Errorf("the file's spelling was rewritten:\n%s", read(t, s, "Lecture.md"))
	}
}

// A note iCloud has not downloaded is read as its name only, with an empty
// body; saving that body would wipe the note once the file arrives. Whole
// saves refuse, and edits that change one key work on the real file.
func TestANoteInICloudIsNeverOverwrittenWithItsPlaceholder(t *testing.T) {
	s := testStore(t)
	put(t, s, "Remote.md", "---\ntags: [a]\n---\nthe real text\n")
	vault.IsDataless = func(info fs.FileInfo) bool { return info.Name() == "Remote.md" }
	t.Cleanup(func() { vault.IsDataless = func(fs.FileInfo) bool { return false } })
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	note, err := s.GetNoteByPath("Remote.md")
	if err != nil {
		t.Fatal(err)
	}
	note.Tags = "b"
	if err := s.UpdateNote(note); !errors.Is(err, ErrNotDownloaded) {
		t.Errorf("err = %v, want ErrNotDownloaded", err)
	}
	if got := read(t, s, "Remote.md"); !strings.Contains(got, "the real text") {
		t.Errorf("file = %q", got)
	}
	if err := s.ArchiveNote(note.ID); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, "Remote.md"); !strings.Contains(got, "the real text") || !strings.Contains(got, "archived:") {
		t.Errorf("file = %q", got)
	}
}

// A note created with tags is written once, tags and all: no window in
// which it exists without them.
func TestANoteIsCreatedWithItsTagsInOneWrite(t *testing.T) {
	s := testStore(t)
	before := s.reloads
	n, err := s.CreateNote("Tagged", "", "body", "", "go", "notes")
	if err != nil {
		t.Fatal(err)
	}
	if n.Tags != "go,notes" {
		t.Errorf("tags = %q", n.Tags)
	}
	if got := s.reloads - before; got != 1 {
		t.Errorf("the vault was read %d times, want once", got)
	}
}

// CJK text has no spaces between words, so a word inside a sentence is
// still found.
func TestSearchFindsAWordInsideCJKText(t *testing.T) {
	s := testStore(t)
	put(t, s, "trip.md", "明日は東京タワーに行く\n")
	put(t, s, "other.md", "something else\n")
	reload(t, s)
	for _, q := range []string{"東京", "タワー", "東京タワー"} {
		if got := strings.Join(slugs(s.SearchNotes(q)), ","); got != "trip" {
			t.Errorf("SearchNotes(%q) = %q, want trip", q, got)
		}
	}
}
