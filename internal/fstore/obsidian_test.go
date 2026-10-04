package fstore

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
	"golang.org/x/text/unicode/norm"
)

// datalessFor makes kb read the named files as iCloud placeholders until
// the test ends.
func datalessFor(t *testing.T, names ...string) {
	t.Helper()
	orig := vault.IsDataless
	vault.IsDataless = func(fi fs.FileInfo) bool {
		for _, n := range names {
			if fi.Name() == n {
				return true
			}
		}
		return false
	}
	t.Cleanup(func() { vault.IsDataless = orig })
}

// A trash folder kb cannot use is an error, not an endless search for a
// free name.
func TestTrashingIntoAnUnusableTrashFolderFails(t *testing.T) {
	s := testStore(t)
	put(t, s, "sub/note.md", "hello\n")
	put(t, s, ".trash/sub", "a file where the folder would be\n")
	reload(t, s)
	n, err := s.GetNoteByPath("sub/note.md")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.TrashNote(n.ID); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TrashNote did not return")
	}
	if got := read(t, s, "sub/note.md"); got != "hello\n" {
		t.Errorf("the note changed: %q", got)
	}
}

// A link names a file by its name, as in Obsidian; a frontmatter title
// another file has does not take the link away from it.
func TestALinkNamesTheFileBeforeATitle(t *testing.T) {
	s := testStore(t)
	put(t, s, "a.md", "---\ntitle: Foo\n---\nA\n")
	put(t, s, "Foo.md", "---\ntitle: Bar\n---\nF\n")
	put(t, s, "src.md", "see [[Foo]]\n")
	reload(t, s)
	got, err := s.ResolveNoteRef("Foo")
	if err != nil || got.Path != "Foo.md" {
		t.Fatalf("[[Foo]] resolves to %v (%v), want Foo.md", got, err)
	}
	a, _ := s.GetNoteByPath("a.md")
	if _, _, err := s.RenameNote(a.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, "src.md"); got != "see [[Foo]]\n" {
		t.Errorf("a link to Foo.md was rewritten: %q", got)
	}
	// A title with no file of that name still finds its note.
	if n, err := s.ResolveNoteRef("Bar"); err != nil || n.Path != "Foo.md" {
		t.Errorf("[[Bar]] = %v, %v", n, err)
	}
}

// A note can be renamed to any title it could be created with.
func TestANoteCanBeRenamedToANonLatinTitle(t *testing.T) {
	s := testStore(t)
	n, err := s.CreateNote("Hello", "", "body", "")
	if err != nil {
		t.Fatal(err)
	}
	renamed, _, err := s.RenameNote(n.ID, "Мир")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Path != "Мир.md" || renamed.Title != "Мир" {
		t.Errorf("renamed = %s %q", renamed.Path, renamed.Title)
	}
}

// A rename moves the file: a symlink stays a symlink, the mode stays.
func TestARenameMovesTheFileItself(t *testing.T) {
	s := testStore(t)
	outside := filepath.Join(t.TempDir(), "real.md")
	os.WriteFile(outside, []byte("linked body\n"), 0o644)
	root := s.Vault().Root()
	if err := os.Symlink(outside, filepath.Join(root, "Link.md")); err != nil {
		t.Fatal(err)
	}
	put(t, s, "Private.md", "secret\n")
	os.Chmod(filepath.Join(root, "Private.md"), 0o600)
	reload(t, s)

	link, _ := s.GetNoteByPath("Link.md")
	if _, _, err := s.RenameNote(link.ID, "Moved Link"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(filepath.Join(root, "Moved Link.md")); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the symlink became a copy: %v %v", fi, err)
	}
	priv, _ := s.GetNoteByPath("Private.md")
	if _, _, err := s.RenameNote(priv.ID, "Still Private"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(root, "Still Private.md")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
}

// A rename that changes only letter case, or only the Unicode form, renames
// the file rather than failing or writing a title over it.
func TestARenameOfCaseOrUnicodeFormRenamesTheFile(t *testing.T) {
	s := testStore(t)
	put(t, s, "foo.md", "body\n")
	nfd := norm.NFD.String("Café")
	put(t, s, nfd+".md", "coffee\n")
	reload(t, s)
	foo, _ := s.GetNoteByPath("foo.md")
	renamed, _, err := s.RenameNote(foo.ID, "Foo")
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(s.Vault().Root())
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if renamed.Path != "Foo.md" || !strings.Contains(strings.Join(names, ","), "Foo.md") {
		t.Errorf("path %s, files %v", renamed.Path, names)
	}
	if got := read(t, s, "Foo.md"); strings.Contains(got, "title:") {
		t.Errorf("a title was written instead of a rename: %q", got)
	}
	cafe, err := s.GetNoteByPath(nfd + ".md")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RenameNote(cafe.ID, "café"); err != nil {
		t.Errorf("rename between Unicode forms: %v", err)
	}
}

// Renaming onto another file's name is refused and changes nothing.
func TestARenameNeverReplacesAnotherFile(t *testing.T) {
	s := testStore(t)
	put(t, s, "a.md", "A\n")
	put(t, s, "sub/b.md", "B\n")
	reload(t, s)
	b, _ := s.GetNoteByPath("sub/b.md")
	// "a" is free in sub/, but a file there appears just before the rename.
	put(t, s, "sub/a.md", "someone else's\n")
	if _, _, err := s.RenameNote(b.ID, "a"); err == nil {
		t.Error("want an error")
	}
	if got := read(t, s, "sub/a.md"); got != "someone else's\n" {
		t.Errorf("the other file was replaced: %q", got)
	}
}

// A note edit reads the real file first, which downloads an iCloud
// placeholder; one that turns out to be a board is left alone.
func TestAPlaceholderIsNeverWrittenAsANote(t *testing.T) {
	s := testStore(t)
	put(t, s, "Boards/Work.md", "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] a\n")
	datalessFor(t, "Work.md")
	reload(t, s)
	before := read(t, s, "Boards/Work.md")
	n, err := s.GetNoteByPath("Boards/Work.md")
	if err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"archive":   func() error { return s.ArchiveNote(n.ID) },
		"workspace": func() error { return s.SetNoteWorkspace(n.ID, s.DefaultWorkspace().ID) },
		"rename":    func() error { _, _, err := s.RenameNote(n.ID, "Other"); return err },
		"publish":   func() error { return s.RecordPublish(n.ID, "blog", "_posts/x.md", false) },
	} {
		if err := write(); !errors.Is(err, ErrIsBoard) {
			t.Errorf("%s: err = %v, want ErrIsBoard", name, err)
		}
	}
	if got := read(t, s, "Boards/Work.md"); got != before {
		t.Errorf("the placeholder was written:\n%s", got)
	}
}

// A workspace rename or delete waits until no file kb cannot read is in
// the vault: one of them may name the workspace.
func TestAWorkspaceRenameWaitsForPlaceholders(t *testing.T) {
	s := testStore(t)
	put(t, s, "a.md", "---\nworkspace: Work\n---\nA\n")
	put(t, s, "b.md", "---\nworkspace: Work\n---\nB\n")
	datalessFor(t, "b.md")
	reload(t, s)
	ws, err := s.GetWorkspaceByName("Work")
	if err != nil {
		t.Fatal(err)
	}
	renamed := *ws
	renamed.Name = "Job"
	if err := s.UpdateWorkspace(&renamed); err == nil || !strings.Contains(err.Error(), "b.md") {
		t.Errorf("rename: err = %v, want one naming b.md", err)
	}
	if got := read(t, s, "a.md"); !strings.Contains(got, "workspace: Work") {
		t.Errorf("a.md was renamed anyway:\n%s", got)
	}
	other, err := s.CreateWorkspace("Other", model.KindArea, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteWorkspace(other.ID); err == nil || !strings.Contains(err.Error(), "b.md") {
		t.Errorf("delete: err = %v, want one naming b.md", err)
	}
}

// Editing workspaces changes only their entries in .kb/workspaces.yml:
// comments, keys kb does not know, kinds it does not know and entries it
// skips stay, and workspaces only notes name are not written into it.
func TestWorkspaceEditsKeepTheRestOfTheFile(t *testing.T) {
	s := testStore(t)
	put(t, s, workspacesFile, "# my notes about workspaces\nworkspaces:\n  - id: 11111111-1111-1111-1111-111111111111\n    name: Work\n    kind: project\n    color: red\n  - name: work\n    kind: area\n    description: dup\n  - name: Weird\n    kind: someday\n")
	put(t, s, "a.md", "---\nworkspace: Scratch\n---\nA\n")
	reload(t, s)
	if _, err := s.CreateWorkspace("New", model.KindArea, "", ""); err != nil {
		t.Fatal(err)
	}
	work, _ := s.GetWorkspaceByName("Work")
	edited := *work
	edited.Description = "the day job"
	if err := s.UpdateWorkspace(&edited); err != nil {
		t.Fatal(err)
	}
	weird, _ := s.GetWorkspaceByName("Weird")
	edited = *weird
	edited.Description = "odd kind"
	if err := s.UpdateWorkspace(&edited); err != nil {
		t.Fatal(err)
	}
	file := read(t, s, workspacesFile)
	for _, want := range []string{"# my notes about workspaces", "color: red", "description: dup", "kind: someday", "name: New", "description: the day job", "description: odd kind"} {
		if !strings.Contains(file, want) {
			t.Errorf("lacks %q:\n%s", want, file)
		}
	}
	for _, unwanted := range []string{"name: Default", "name: Scratch"} {
		if strings.Contains(file, unwanted) {
			t.Errorf("has %q, a workspace only notes name:\n%s", unwanted, file)
		}
	}
}

// Default is Default in any case.
func TestDefaultCannotBeRenamedInAnyCase(t *testing.T) {
	s := testStore(t)
	put(t, s, workspacesFile, "workspaces:\n  - name: default\n    kind: area\n")
	reload(t, s)
	ws, err := s.GetWorkspaceByName("Default")
	if err != nil {
		t.Fatal(err)
	}
	renamed := *ws
	renamed.Name = "Inbox"
	if err := s.UpdateWorkspace(&renamed); err == nil {
		t.Error("want an error")
	}
	if err := s.DeleteWorkspace(ws.ID); err == nil {
		t.Error("delete: want an error")
	}
}

// A note is not created in a folder that is a symlink: kb does not read
// such folders, so it could not find the note again.
func TestANoteIsNotCreatedInASymlinkedFolder(t *testing.T) {
	s := testStore(t)
	real := t.TempDir()
	if err := os.Symlink(real, filepath.Join(s.Vault().Root(), "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNoteAt("linked/x.md", "x", "body", ""); err == nil {
		t.Error("want an error")
	}
	if _, err := os.Stat(filepath.Join(real, "x.md")); err == nil {
		t.Error("the file was written")
	}
}
