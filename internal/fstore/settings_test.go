package fstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

// A vault reached through a symlink (~/notes -> an iCloud folder) is the
// folder it points to.
func TestAVaultReachedThroughASymlinkIsRead(t *testing.T) {
	real := t.TempDir()
	os.WriteFile(filepath.Join(real, "a.md"), []byte("body"), 0o644)
	link := filepath.Join(t.TempDir(), "notes")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	s := openAt(t, link)
	if len(s.ListNotes()) != 1 {
		t.Fatalf("notes = %d, want 1; problems %v", len(s.ListNotes()), s.Problems())
	}
	if _, err := s.CreateNote("b", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if len(s.ListNotes()) != 2 {
		t.Errorf("notes = %d after a create, want 2", len(s.ListNotes()))
	}
}

// Two kb processes that name one vault differently (a symlink, or another
// case on a case-insensitive disk) still take the same lock for a file.
func TestOneFileHasOneLockHoweverTheVaultIsNamed(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	root := s.Vault().Root()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	names := []string{root, link}
	if upper := strings.ToUpper(root); caseInsensitive(t, root, upper) {
		names = append(names, upper)
	}
	var stores []*Store
	for _, n := range names {
		other, err := Open(n, s.opts)
		if err != nil {
			t.Fatal(err)
		}
		stores = append(stores, other)
	}
	var wg sync.WaitGroup
	for i, st := range stores {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := st.AddCard(b.ID, "Backlog", fmt.Sprintf("card %d-%d", i, j), CardFields{}, false); err != nil {
					t.Error(err)
				}
			}
		}(i, st)
	}
	wg.Wait()
	reload(t, s)
	cards, err := s.Cards(b.ID, "Backlog")
	if err != nil {
		t.Fatal(err)
	}
	if want := 20 * len(stores); len(cards) != want {
		t.Errorf("cards = %d, want %d", len(cards), want)
	}
}

func caseInsensitive(t *testing.T, a, b string) bool {
	t.Helper()
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

// Every change to .kb/workspaces.yml starts from the file as it is, so a
// workspace another kb added since this one read the vault is kept.
func TestWorkspacesAddedByTwoProcessesAreBothKept(t *testing.T) {
	a := testStore(t)
	b := sibling(t, a)
	if _, err := a.CreateWorkspace("Alpha", model.KindArea, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateWorkspace("Beta", model.KindArea, "", ""); err != nil {
		t.Fatal(err)
	}
	ws, err := b.GetWorkspaceByName("Alpha")
	if err != nil {
		t.Fatalf("Alpha lost: %v\n%s", err, read(t, a, workspacesFile))
	}
	updated := *ws
	updated.Description = "edited"
	if err := a.UpdateWorkspace(&updated); err != nil {
		t.Fatal(err)
	}
	file := read(t, a, workspacesFile)
	for _, want := range []string{"Alpha", "Beta", "edited"} {
		if !strings.Contains(file, want) {
			t.Errorf("workspaces.yml lacks %s:\n%s", want, file)
		}
	}
}

func TestManyProcessesAddingWorkspacesLoseNone(t *testing.T) {
	s := testStore(t)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		other := sibling(t, s)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := other.CreateWorkspace(fmt.Sprintf("ws%d", i), model.KindArea, "", ""); err != nil {
				t.Error(err)
			}
			if _, err := other.CreatePublishTarget(fmt.Sprintf("site%d", i), model.EngineJekyll, "/tmp/site", "", "", ""); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	reload(t, s)
	if n := len(s.ListWorkspaces()); n != 11 {
		t.Errorf("workspaces = %d, want 11 (Default and 10)", n)
	}
	if n := len(s.ListPublishTargets()); n != 10 {
		t.Errorf("publish targets = %d, want 10", n)
	}
}

// A settings file kb cannot read is reported and never rewritten, since a
// rewrite would keep only what kb could read: nothing.
func TestASettingsFileKbCannotReadIsNeverOverwritten(t *testing.T) {
	s := testStore(t)
	broken := "workspaces:\n  - id: 11111111-1111-1111-1111-111111111111\n    name: Client Work\n  - name: [broken\n"
	put(t, s, workspacesFile, broken)
	reload(t, s)
	if _, err := s.CreateWorkspace("New", model.KindArea, "", ""); err == nil || !strings.Contains(err.Error(), "workspaces.yml") {
		t.Errorf("create workspace: err = %v, want one naming workspaces.yml", err)
	}
	if got := read(t, s, workspacesFile); got != broken {
		t.Errorf("workspaces.yml was rewritten:\n%s", got)
	}

	brokenTargets := "targets:\n  - name: blog\n    path: [\n"
	os.MkdirAll(s.opts.ConfigDir, 0o755)
	targets := filepath.Join(s.opts.ConfigDir, "publish.yml")
	os.WriteFile(targets, []byte(brokenTargets), 0o644)
	reload(t, s)
	if !strings.Contains(strings.Join(s.Problems(), "\n"), "publish.yml") {
		t.Errorf("problems %v do not name publish.yml", s.Problems())
	}
	if _, err := s.CreatePublishTarget("site2", model.EngineJekyll, "/tmp/site2", "", "", ""); err == nil {
		t.Error("create target: want an error")
	}
	if err := s.DeletePublishTarget("blog"); err == nil {
		t.Error("delete target: want an error")
	}
	if data, _ := os.ReadFile(targets); string(data) != brokenTargets {
		t.Errorf("publish.yml was rewritten:\n%s", data)
	}

	paths := filepath.Join(s.opts.ConfigDir, "workspace-paths.yml")
	os.WriteFile(paths, []byte("{broken"), 0o644)
	os.Remove(filepath.Join(s.Vault().Root(), filepath.FromSlash(workspacesFile)))
	reload(t, s)
	if _, err := s.CreateWorkspace("Pathed", model.KindArea, "", "/tmp/x"); err == nil {
		t.Error("create with a path: want an error")
	}
	if data, _ := os.ReadFile(paths); string(data) != "{broken" {
		t.Errorf("workspace-paths.yml was rewritten:\n%s", data)
	}
}

// A slug with a number suffix never takes the slug of another file, so a
// slug names one note however kb looks it up.
func TestSlugSuffixesSkipSlugsOtherFilesHave(t *testing.T) {
	s := testStore(t)
	put(t, s, "foo.md", "a")
	put(t, s, "sub/foo.md", "b")
	put(t, s, "foo-2.md", "c")
	reload(t, s)
	seen := map[string]string{}
	for _, n := range s.ListNotes() {
		if other, ok := seen[n.Slug]; ok {
			t.Errorf("%s and %s share the slug %q", other, n.Path, n.Slug)
		}
		seen[n.Slug] = n.Path
	}
	for slug := range seen {
		bySlug, err := s.GetNoteBySlug(slug)
		if err != nil {
			t.Fatal(err)
		}
		byRef, err := s.ResolveNoteRef(slug)
		if err != nil {
			t.Fatal(err)
		}
		if bySlug.Path != byRef.Path {
			t.Errorf("slug %q: GetNoteBySlug %s, ResolveNoteRef %s", slug, bySlug.Path, byRef.Path)
		}
	}
}

// kb reads no hidden folder, so it refuses to create a note in one rather
// than write a file it then cannot find.
func TestANoteCannotBeCreatedInAHiddenFolder(t *testing.T) {
	s := testStore(t)
	n, err := s.CreateNoteAt(".drafts/x.md", "x", "body", "")
	if err == nil || n != nil {
		t.Errorf("note = %v, err = %v; want an error", n, err)
	}
	if _, statErr := os.Stat(filepath.Join(s.Vault().Root(), ".drafts", "x.md")); statErr == nil {
		t.Error("the file was written")
	}
}

// A note trashed by another process between kb's write and its reload is
// reported, not dereferenced.
func TestUpdatingANoteAnotherProcessTrashedIsAnError(t *testing.T) {
	s := testStore(t)
	n, err := s.CreateNote("gone", "", "body", "")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(s.Vault().Root(), n.Path))
	n.Rev = ""
	if err := s.UpdateNote(n); err == nil {
		t.Error("want an error")
	}
}

// Renaming a workspace rewrites every file that names it and reads the
// vault again once, not once per file.
func TestAWorkspaceRenameReadsTheVaultOnce(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 30; i++ {
		put(t, s, fmt.Sprintf("n%02d.md", i), "---\nworkspace: Work\n---\nbody")
	}
	put(t, s, "Boards/B.md", "---\nkanban-plugin: board\nworkspace: Work\n---\n\n## Todo\n")
	reload(t, s)
	ws, err := s.GetWorkspaceByName("Work")
	if err != nil {
		t.Fatal(err)
	}
	before := s.reloads
	renamed := *ws
	renamed.Name = "Job"
	if err := s.UpdateWorkspace(&renamed); err != nil {
		t.Fatal(err)
	}
	if n := s.reloads - before; n > 3 {
		t.Errorf("the rename read the vault %d times", n)
	}
	for i := 0; i < 30; i++ {
		if got := read(t, s, fmt.Sprintf("n%02d.md", i)); !strings.Contains(got, "workspace: Job") {
			t.Fatalf("n%02d.md:\n%s", i, got)
		}
	}
	if got := read(t, s, "Boards/B.md"); !strings.Contains(got, "workspace: Job") {
		t.Errorf("board:\n%s", got)
	}
	if _, err := s.GetWorkspaceByName("Work"); err == nil {
		t.Error("Work still exists")
	}
	if got := len(s.ListWorkspaces()); got != 2 {
		t.Errorf("workspaces = %d, want Default and Job", got)
	}
}

// An unreadable workspace-paths.yml stops only changes to a path: a
// workspace made or edited without one is saved whole, and the file is
// left alone.
func TestAnUnreadablePathsFileStopsOnlyPathChanges(t *testing.T) {
	s := testStore(t)
	paths := filepath.Join(s.opts.ConfigDir, "workspace-paths.yml")
	os.MkdirAll(s.opts.ConfigDir, 0o755)
	os.WriteFile(paths, []byte("{broken"), 0o644)
	reload(t, s)
	ws, err := s.CreateWorkspace("NoPath", model.KindArea, "", "")
	if err != nil {
		t.Fatalf("create without a path: %v", err)
	}
	edited := *ws
	edited.Description = "new description"
	if err := s.UpdateWorkspace(&edited); err != nil {
		t.Fatalf("edit without a path change: %v", err)
	}
	edited.Path = "/tmp/somewhere"
	if err := s.UpdateWorkspace(&edited); err == nil {
		t.Error("a path change: want an error")
	}
	if got, _ := s.GetWorkspaceByName("NoPath"); got.Path != "" {
		t.Errorf("path = %q", got.Path)
	}
	if data, _ := os.ReadFile(paths); string(data) != "{broken" {
		t.Errorf("workspace-paths.yml was rewritten: %s", data)
	}
}

// A rename with an unreadable workspaces.yml stops before it rewrites any
// note, instead of renaming the files and then failing.
func TestARenameWithAnUnreadableListChangesNoFile(t *testing.T) {
	s := testStore(t)
	put(t, s, "n.md", "---\nworkspace: Work\n---\nbody")
	put(t, s, workspacesFile, "workspaces: [broken\n")
	reload(t, s)
	ws, err := s.GetWorkspaceByName("Work")
	if err != nil {
		t.Fatal(err)
	}
	renamed := *ws
	renamed.Name = "Job"
	if err := s.UpdateWorkspace(&renamed); err == nil {
		t.Fatal("want an error")
	}
	if got := read(t, s, "n.md"); !strings.Contains(got, "workspace: Work") {
		t.Errorf("the note was rewritten:\n%s", got)
	}
}

// A write reads again only the files that changed, not the whole vault.
func TestAWriteReparsesOnlyChangedFiles(t *testing.T) {
	s := testStore(t)
	for i := 0; i < 200; i++ {
		put(t, s, fmt.Sprintf("n%03d.md", i), fmt.Sprintf("---\ntags: [t]\n---\nnote %d links [[n%03d]]\n", i, (i+1)%200))
	}
	put(t, s, "Boards/B.md", "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] a ^abcd1234\n")
	reload(t, s)
	n, err := s.GetNoteByPath("n007.md")
	if err != nil {
		t.Fatal(err)
	}
	before := s.parsed
	n.Body = "changed, links [[n100]]\n"
	if err := s.UpdateNote(n); err != nil {
		t.Fatal(err)
	}
	if got := s.parsed - before; got > 2 {
		t.Errorf("one note's save parsed %d files", got)
	}
	// What the store knows is still right after the cached reload.
	if bl := s.Backlinks(mustNote(t, s, "n100.md").ID); len(bl) != 2 {
		t.Errorf("n100 backlinks = %d, want n099 and n007", len(bl))
	}
	if _, err := s.AddCard("Boards/B.md", "Todo", "b", CardFields{}, false); err != nil {
		t.Fatal(err)
	}
	if cards, _ := s.Cards("Boards/B.md", "Todo"); len(cards) != 2 {
		t.Errorf("cards = %d", len(cards))
	}
	// A file changed behind kb's back, same size and all, is read again.
	put(t, s, "n050.md", "---\ntags: [x]\n---\nnote 50 links [[n051]]\n")
	reload(t, s)
	if got := mustNote(t, s, "n050.md").Tags; got != "x" {
		t.Errorf("an outside edit was missed: tags %q", got)
	}
}

func mustNote(t *testing.T, s *Store, rel string) *Note {
	t.Helper()
	n, err := s.GetNoteByPath(rel)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A vault that does not exist yet, reached through a symlinked folder,
// locks as the place it will be, as it will once created.
func TestAVaultNotCreatedYetLocksAsItsRealPlace(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got, want := ResolvePath(filepath.Join(link, "vault", "Note.md")), filepath.Join(ResolvePath(real), "vault", "Note.md"); got != want {
		t.Errorf("ResolvePath = %s, want %s", got, want)
	}
}
