package legacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeryldev/kb/internal/fstore"
)

// fixture builds a kb 0.3 database from the real schema plus rows.
func fixture(t *testing.T, rows string) string {
	t.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		missing(t, "sqlite3 is not installed")
	}
	schema, err := os.ReadFile("testdata/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "kb.db")
	cmd := exec.Command("sqlite3", path)
	cmd.Stdin = strings.NewReader(string(schema) + "\n" + rows)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fixture: %v\n%s", err, out)
	}
	return path
}

func testStore(t *testing.T) *fstore.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := fstore.Open(filepath.Join(dir, "vault"), fstore.Options{
		ConfigDir: filepath.Join(dir, "config"),
		LockDir:   filepath.Join(dir, "locks"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func read(t *testing.T, s *fstore.Store, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.Vault().Root(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The shape of a real 0.3 database: the default columns, labels with
// spaces, a description with a blank line, urgent, archived and deleted
// cards, a WIP limit, and a NULL external id from an old schema.
const realShape = `
INSERT INTO workspaces (id, name, kind, position) VALUES
  ('ws-default', 'Default', 'area', 0),
  ('ws-work', 'Work', 'project', 1);
UPDATE workspaces SET description = 'day job', path = '/tmp/work' WHERE id = 'ws-work';
INSERT INTO boards (id, name, description, workspace_id) VALUES
  ('b1', 'kb', 'the kb board', 'ws-work');
INSERT INTO columns (id, board_id, name, position, wip_limit) VALUES
  ('c1', 'b1', 'Backlog', 0, NULL),
  ('c2', 'b1', 'Todo', 1, NULL),
  ('c3', 'b1', 'In Progress', 2, 2),
  ('c4', 'b1', 'Review', 3, NULL),
  ('c5', 'b1', 'Done', 4, NULL);
INSERT INTO cards (id, column_id, title, description, priority, position, labels, external_id, archived_at, deleted_at) VALUES
  ('889962bb-0000-4000-8000-000000000001', 'c1', 'Fix login bug', 'First paragraph.

Second paragraph.
', 'high', 0, 'some label', '', NULL, NULL),
  ('375c351d-0000-4000-8000-000000000002', 'c1', 'Add auth', '', 'medium', 1, 'backend,  api ', 'GH-12', NULL, NULL),
  ('5b20d812-0000-4000-8000-000000000003', 'c3', 'Write tests', '', 'low', 0, '', NULL, NULL, NULL),
  ('7b6712d7-0000-4000-8000-000000000004', 'c4', 'sample ticket', 'old', 'urgent', 0, 'label1', '', '2026-03-01 10:00:00', NULL),
  ('da6d1af4-0000-4000-8000-000000000005', 'c5', 'gone card', '', 'medium', 0, '', '', NULL, '2026-03-02 10:00:00');
`

func TestImportWritesBoardsInThePluginFormat(t *testing.T) {
	s := testStore(t)
	plan, err := Read(fixture(t, realShape), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	got := read(t, s, "Boards/kb.md")
	for _, want := range []string{
		"kanban-plugin: board",
		"description: the kb board",
		"workspace: Work",
		"kb-import: b1",
		"## In Progress (2)",
		"- [ ] Fix login bug #some-label #priority/high ^889962bb\n    First paragraph.\n    \n    Second paragraph.\n- [ ] Add auth",
		"- [ ] Add auth #backend #api [ext:: GH-12] ^375c351d",
		"- [ ] Write tests #priority/low ^5b20d812",
		"## Archive",
		"sample ticket #label1 #priority/urgent ^7b6712d7",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("board file lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "gone card") {
		t.Error("deleted cards are dropped")
	}

	// A board in Default names no workspace, as kb writes boards.
	s2 := testStore(t)
	plan, err = Read(fixture(t, strings.Replace(realShape, "'the kb board', 'ws-work'", "'the kb board', 'ws-default'", 1)), s2)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(s2); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s2, "Boards/kb.md"); strings.Contains(got, "workspace:") {
		t.Errorf("a Default board names its workspace:\n%s", got)
	}

	// kb reads it back the same.
	cards, err := s.BoardCards("Boards/kb.md")
	if err != nil || len(cards) != 3 {
		t.Fatalf("cards = %v, %v", cards, err)
	}
	if c := cards[0]; c.Description != "First paragraph.\n\nSecond paragraph." || c.Labels != "some-label" {
		t.Errorf("first card = %+v", c)
	}
	ws, err := s.GetWorkspaceByName("Work")
	if err != nil || ws.Description != "day job" || ws.Path != "/tmp/work" || string(ws.Kind) != "project" {
		t.Errorf("workspace = %+v, %v", ws, err)
	}
}

func TestTheReportNamesEveryChangedLabel(t *testing.T) {
	s := testStore(t)
	plan, err := Read(fixture(t, realShape), s)
	if err != nil {
		t.Fatal(err)
	}
	var changes []string
	for _, c := range plan.Labels {
		changes = append(changes, c.From+" -> "+c.To)
	}
	if strings.Join(changes, "; ") != "some label -> some-label" {
		t.Errorf("label changes = %v", changes)
	}
	report := plan.Report()
	for _, want := range []string{"kb (Boards/kb.md): 3 cards, 1 archived, 1 deleted card dropped", "workspace Work", `"some label" becomes #some-label`} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

func TestADryRunWritesNothing(t *testing.T) {
	s := testStore(t)
	plan, err := Read(fixture(t, realShape), s)
	if err != nil {
		t.Fatal(err)
	}
	_ = plan.Report()
	entries, _ := os.ReadDir(s.Vault().Root())
	if len(entries) != 0 {
		t.Errorf("the vault has %d entries after only reading", len(entries))
	}
}

// Running the import again, say after it stopped part way, changes
// nothing that it already did.
func TestImportingTwiceChangesNothing(t *testing.T) {
	s := testStore(t)
	db := fixture(t, realShape)
	for i := 0; i < 2; i++ {
		plan, err := Read(db, s)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && len(plan.Boards) != 0 {
			t.Errorf("second plan still has boards: %v", plan.Boards)
		}
		if err := plan.Apply(s); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(s.ListBoards()); n != 1 {
		t.Errorf("boards = %d", n)
	}
	if n := len(s.ListWorkspaces()); n != 2 {
		t.Errorf("workspaces = %d", n)
	}
}

// Problems are found before anything is written.
func TestProblemsAreFoundBeforeAnythingIsWritten(t *testing.T) {
	s := testStore(t)
	// A board already in the vault under a name the import needs.
	if _, err := s.CreateBoard("KB", "", ""); err != nil {
		t.Fatal(err)
	}
	rows := realShape + `
INSERT INTO boards (id, name) VALUES ('b2', 'Second');
INSERT INTO columns (id, board_id, name, position) VALUES ('d1', 'b2', 'Doing (3)', 0);
INSERT INTO boards (id, name) VALUES ('b3', 'a/b'), ('b4', 'a:b');
`
	_, err := Read(fixture(t, rows), s)
	if err == nil {
		t.Fatal("expected problems")
	}
	for _, want := range []string{`"kb"`, "Doing (3)", "a/b", "a:b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	if n := len(s.ListBoards()); n != 1 {
		t.Errorf("boards = %d, nothing should be written", n)
	}
	if _, err := s.GetWorkspaceByName("Work"); err == nil {
		t.Error("the workspace should not have been written")
	}
}

func TestMultiLineTitlesMoveTheRestToTheDescription(t *testing.T) {
	s := testStore(t)
	rows := realShape + `
INSERT INTO cards (id, column_id, title, description, position) VALUES
  ('aaaaaaaa-0000-4000-8000-000000000009', 'c2', 'Line one
line two', 'desc', 0);
`
	plan, err := Read(fixture(t, rows), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	c, err := s.FindCard("Boards/kb.md", "aaaaaaaa")
	if err != nil || c.Title != "Line one" || c.Description != "line two\n\ndesc" {
		t.Errorf("card = %+v, %v", c, err)
	}
	if !strings.Contains(plan.Report(), "Line one") {
		t.Errorf("the report should name the changed title:\n%s", plan.Report())
	}
}

func TestPublishTargetsAndHistory(t *testing.T) {
	s := testStore(t)
	site := t.TempDir()
	note, err := s.CreateNoteAt("Posted.md", "Posted", "text", s.DefaultWorkspace().ID)
	if err != nil {
		t.Fatal(err)
	}
	rows := realShape + `
INSERT INTO notes (id, title, slug, path, workspace_id) VALUES ('old-note-id', 'Posted', 'posted', 'Posted.md', 'ws-default');
INSERT INTO publish_targets (id, name, engine, base_path, posts_dir, workspace_id) VALUES ('t1', 'blog', 'jekyll', '` + site + `', '_posts', 'ws-work');
INSERT INTO publish_log (id, note_id, target_id, file_path, front_matter, published_at) VALUES
  ('l1', 'old-note-id', 't1', '_posts/2026-05-01-posted.md', 'published: false', '2026-05-01 10:00:00'),
  ('l2', 'old-note-id', 't1', '_posts/2026-05-01-posted.md', 'title: x', '2026-05-02 10:00:00');
`
	plan, err := Read(fixture(t, rows), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	pt, err := s.GetPublishTarget("blog")
	if err != nil || pt.BasePath != site || pt.WorkspaceID == nil {
		t.Fatalf("target = %+v, %v", pt, err)
	}
	post, ok := s.LatestPost(note.ID, "blog")
	if !ok || post.Path != "_posts/2026-05-01-posted.md" || post.Draft {
		t.Errorf("post = %+v, %v (the latest publish was not a draft)", post, ok)
	}
}

// A note whose workspace only the database knew gets it in its file.
func TestNoteWorkspacesKnownOnlyToTheDatabaseAreWritten(t *testing.T) {
	s := testStore(t)
	note, _ := s.CreateNoteAt("Plan.md", "Plan", "x", s.DefaultWorkspace().ID)
	rows := realShape + `
INSERT INTO notes (id, title, slug, path, workspace_id) VALUES ('n1', 'Plan', 'plan', 'Plan.md', 'ws-work');
INSERT INTO notes (id, title, slug, path, workspace_id) VALUES ('n2', 'Missing', 'missing', 'Missing.md', 'ws-work');
`
	plan, err := Read(fixture(t, rows), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetNote(note.ID)
	ws, _ := s.GetWorkspaceByName("Work")
	if got.WorkspaceID != ws.ID {
		t.Errorf("note workspace = %q", got.WorkspaceID)
	}
}

func TestAWALFileMeansKB03MayBeRunning(t *testing.T) {
	s := testStore(t)
	db := fixture(t, realShape)
	os.WriteFile(db+"-wal", []byte("pending"), 0o644)
	if _, err := Read(db, s); err == nil || !strings.Contains(err.Error(), "running") {
		t.Errorf("err = %v", err)
	}
}

func TestFinishRenamesTheDatabase(t *testing.T) {
	db := fixture(t, realShape)
	os.WriteFile(db+"-shm", []byte("x"), 0o644)
	if err := Finish(db); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Error("kb.db should be gone")
	}
	for _, f := range []string{db + ".imported-0.4", db + "-shm.imported-0.4"} {
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}
}

// An import that fails part way can be run again once the cause is gone,
// and finishes without doing anything twice.
func TestAnImportThatStoppedPartWayCanBeRunAgain(t *testing.T) {
	s := testStore(t)
	db := fixture(t, realShape)
	plan, err := Read(db, s)
	if err != nil {
		t.Fatal(err)
	}
	// Something takes the board's file name between the check and the write.
	blocker := filepath.Join(s.Vault().Root(), "Boards", "kb.md")
	os.MkdirAll(filepath.Dir(blocker), 0o755)
	os.WriteFile(blocker, []byte("a note, not a board"), 0o644)
	if err := plan.Apply(s); err == nil {
		t.Fatal("expected the board write to fail")
	}
	if _, err := s.GetWorkspaceByName("Work"); err != nil {
		t.Fatal("the workspace was written before the failure")
	}

	os.Remove(blocker)
	s.Reload()
	plan, err = Read(db, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Workspaces) != 0 || len(plan.Boards) != 1 {
		t.Errorf("second plan: %d workspaces, %d boards", len(plan.Workspaces), len(plan.Boards))
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	if n := len(s.ListWorkspaces()); n != 2 {
		t.Errorf("workspaces = %d", n)
	}
	if _, err := s.FindCard("kb", "889962bb"); err != nil {
		t.Error(err)
	}
}

// missing skips a test that needs a tool this machine lacks, except in CI
// (KB_CI set), where every test must run.
func missing(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("KB_CI") != "" {
		t.Fatalf("%s, and KB_CI is set", what)
	}
	t.Skip(what)
}

// Two columns whose names differ only in case would be one lane: kb finds
// lanes ignoring case, so their cards would be merged.
func TestColumnsDifferingOnlyInCaseAreAProblem(t *testing.T) {
	s := testStore(t)
	rows := realShape + `
INSERT INTO columns (id, board_id, name, position) VALUES ('c6', 'b1', 'done', 5);
`
	_, err := Read(fixture(t, rows), s)
	if err == nil || !strings.Contains(err.Error(), `"done"`) || !strings.Contains(err.Error(), `"Done"`) {
		t.Fatalf("err = %v, want a problem naming both columns", err)
	}
}

// A label holding characters a tag cannot is written as a tag the plugin
// reads whole, and the report says so.
func TestLabelsWithPunctuationBecomeWholeTags(t *testing.T) {
	s := testStore(t)
	rows := realShape + `
UPDATE cards SET labels = 'c#,q&a,[wip]' WHERE id LIKE '5b20d812%';
`
	plan, err := Read(fixture(t, rows), s)
	if err != nil {
		t.Fatal(err)
	}
	var changes []string
	for _, c := range plan.Labels {
		changes = append(changes, c.From+" -> "+c.To)
	}
	got := strings.Join(changes, "; ")
	for _, want := range []string{"c# -> c", "q&a -> q-a", "[wip] -> wip"} {
		if !strings.Contains(got, want) {
			t.Errorf("label changes %q lack %q", got, want)
		}
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	if b := read(t, s, "Boards/kb.md"); !strings.Contains(b, "Write tests #c #q-a #wip") {
		t.Errorf("board:\n%s", b)
	}
}

// A label with nothing a tag can hold is reported as dropped, and an old
// Mac line break (a lone CR) in a title splits it like any other.
func TestOddLabelsAndTitlesImportCleanly(t *testing.T) {
	s := testStore(t)
	rows := realShape + `
UPDATE cards SET labels = '!!!,ok', title = 'old' || char(13) || 'mac' WHERE id LIKE '5b20d812%';
`
	plan, err := Read(fixture(t, rows), s)
	if err != nil {
		t.Fatal(err)
	}
	if report := plan.Report(); !strings.Contains(report, `"!!!" is dropped`) {
		t.Errorf("report:\n%s", report)
	}
	if err := plan.Apply(s); err != nil {
		t.Fatal(err)
	}
	b := read(t, s, "Boards/kb.md")
	if !strings.Contains(b, "- [ ] old #ok") || !strings.Contains(b, "    mac") {
		t.Errorf("board:\n%s", b)
	}
}
