package cmd

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/vault"
	"github.com/spf13/cobra"
)

// Tests for the findings of the 0.3 CLI review, by finding.

func TestColumnNamesAreUnique(t *testing.T) { // C1
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	if _, err := executeCmdErr(t, "columns", "add", "todo"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v, want already exists", err)
	}
	if _, err := executeCmdErr(t, "columns", "rename", "Todo", "done"); err == nil {
		t.Error("renaming onto another column's name should fail")
	}
}

func TestTheDefaultWorkspaceStays(t *testing.T) { // C2
	setupTestDB(t)
	for _, args := range [][]string{
		{"workspace", "delete", "Default"},
		{"workspace", "archive", "Default"},
		{"workspace", "edit", "Default", "--name", "Other"},
	} {
		if _, err := executeCmdErr(t, args...); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
	executeCmd(t, "notes", "create", "Still works")
}

func TestAWorkspaceInUseCannotBeDeleted(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "workspace", "create", "Busy")
	executeCmd(t, "notes", "create", "Mine", "-w", "Busy")
	_, err := executeCmdErr(t, "workspace", "delete", "busy")
	if err == nil || !strings.Contains(err.Error(), "Mine.md") {
		t.Errorf("err = %v, want it to name the note", err)
	}
}

func TestPublishPathsMustBeAbsolute(t *testing.T) { // C4
	setupTestDB(t)
	if _, err := executeCmdErr(t, "publish", "setup", "blog", "--path", "./site"); err == nil {
		t.Error("a relative site path should be refused")
	}
	home, _ := os.UserHomeDir()
	out := executeCmd(t, "publish", "setup", "blog", "--path", "~/site", "--json")
	var pt publishTargetJSON
	json.Unmarshal([]byte(out), &pt)
	if pt.BasePath != filepath.Join(home, "site") {
		t.Errorf("base path = %q", pt.BasePath)
	}
}

func TestNoteDeleteAsksFirstAndUsesTheTrash(t *testing.T) { // C5
	setupTestDB(t)
	executeCmd(t, "notes", "create", "Keep")
	if _, err := executeCmdErr(t, "notes", "delete", "keep"); !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v, want cancelled", err)
	}
	out := executeCmd(t, "notes", "delete", "keep", "-f")
	if !strings.Contains(out, ".trash/Keep.md") {
		t.Errorf("output: %s", out)
	}
	if _, err := os.Stat(filepath.Join(db.Vault().Root(), ".trash", "Keep.md")); err != nil {
		t.Error(err)
	}
}

func TestReorderNeedsEveryColumnOnce(t *testing.T) { // C7
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	for _, order := range []string{
		"Done,Review",
		"Done,Review,In Progress,Todo,Nope",
		"Done,Review,In Progress,Todo,Todo",
	} {
		if _, err := executeCmdErr(t, "columns", "reorder", order); err == nil {
			t.Errorf("reorder %q should fail", order)
		}
	}
	executeCmd(t, "columns", "reorder", "done, review, in progress, todo, backlog")
	var cols []columnJSON
	json.Unmarshal([]byte(executeCmd(t, "columns", "--json")), &cols)
	if len(cols) != 5 || cols[0].Name != "Done" || cols[4].Name != "Backlog" {
		t.Errorf("columns = %+v", cols)
	}
}

func TestEditsWithNothingToChangeAreErrors(t *testing.T) { // C9
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	executeCmd(t, "notes", "create", "Plain")
	before, _ := os.ReadFile(filepath.Join(db.Vault().Root(), "Plain.md"))
	card, _ := createTestCard(t, testLanes(t, "test-board")[0], "Card", "")
	for _, args := range [][]string{
		{"notes", "edit", "plain"},
		{"cards", "edit", card.ID, "--json"},
		{"workspace", "edit", "Default"},
	} {
		if _, err := executeCmdErr(t, args...); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
	after, _ := os.ReadFile(filepath.Join(db.Vault().Root(), "Plain.md"))
	if string(before) != string(after) {
		t.Errorf("the note's file changed:\n%s\n---\n%s", before, after)
	}
}

func TestMovingANoteToItsOwnWorkspaceLeavesTheFile(t *testing.T) { // C9
	setupTestDB(t)
	path := filepath.Join(db.Vault().Root(), "Plain.md")
	os.WriteFile(path, []byte("just text\n"), 0o644)
	db.Reload()
	executeCmd(t, "notes", "move", "plain")
	if data, _ := os.ReadFile(path); string(data) != "just text\n" {
		t.Errorf("file = %q", data)
	}
}

func TestTheGraphHasBoardsCardsAndOutsideLinks(t *testing.T) { // C10
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	executeCmd(t, "workspace", "create", "Side")
	createTestBoard(t, "test-board")
	executeCmd(t, "notes", "create", "Spec")
	executeCmd(t, "notes", "create", "Aside", "-w", "Side", "--body", "see [[Spec]]")
	executeCmd(t, "cards", "add", "Implement [[Spec]]")

	var g struct {
		Nodes []struct {
			Label   string `json:"label"`
			Type    string `json:"type"`
			Outside bool   `json:"outside"`
		} `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "graph", "--json", "-w", "Default")), &g); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]string{}
	for _, n := range g.Nodes {
		kinds[n.Label] = n.Type
		if n.Label == "Aside" && !n.Outside {
			t.Error("Aside is in another workspace and should be marked outside")
		}
	}
	if kinds["Spec"] != "note" || kinds["test-board"] != "board" || kinds["Implement [[Spec]]"] != "card" || kinds["Aside"] != "note" {
		t.Errorf("nodes = %v", kinds)
	}
	if len(g.Edges) < 2 {
		t.Errorf("edges = %d", len(g.Edges))
	}
	if _, err := executeCmdErr(t, "graph", "extra"); err == nil {
		t.Error("graph takes no arguments")
	}
}

func TestEmptyListsPrintAnEmptyJSONList(t *testing.T) { // C12
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	for _, args := range [][]string{{"boards"}, {"notes"}, {"tags"}, {"publish", "list"}, {"workspace", "-k", "archive"}} {
		out := executeCmd(t, append(args, "--json")...)
		if strings.TrimSpace(out) != "[]" {
			t.Errorf("%v --json = %q, want []", args, out)
		}
	}
	createTestBoard(t, "test-board")
	for _, args := range [][]string{{"cards"}, {"cards", "-l", "nothing"}} {
		if out := executeCmd(t, append(args, "--json")...); strings.TrimSpace(out) != "[]" {
			t.Errorf("%v --json = %q, want []", args, out)
		}
	}
}

func TestRuntimeErrorsDoNotPrintUsage(t *testing.T) { // C13
	setupTestDB(t)
	out, err := executeCmdErr(t, "notes", "show", "missing")
	if err == nil || strings.Contains(out, "Usage:") {
		t.Errorf("err %v, output %q", err, out)
	}
}

func TestDuplicateNamesGetAPlainError(t *testing.T) { // C14
	setupTestDB(t)
	createTestBoard(t, "dup")
	executeCmd(t, "workspace", "create", "One")
	executeCmd(t, "workspace", "create", "Two")
	for _, args := range [][]string{
		{"boards", "create", "DUP"},
		{"workspace", "edit", "One", "--name", "two"},
		{"workspace", "create", "one"},
	} {
		_, err := executeCmdErr(t, args...)
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestGroupCommandsRefuseStrayWords(t *testing.T) { // C15
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	for _, args := range [][]string{{"boards", "remove", "x"}, {"cards", "rm"}, {"columns", "x"}, {"workspace", "x"}, {"notes", "x"}, {"index", "x"}} {
		if _, err := executeCmdErr(t, args...); err == nil {
			t.Errorf("%v should fail", args)
		}
	}
}

func TestNotesTagAndSearchCombine(t *testing.T) { // C22
	setupTestDB(t)
	executeCmd(t, "notes", "create", "Go tips", "--tags", "go", "--body", "channels")
	executeCmd(t, "notes", "create", "Rust tips", "--tags", "rust", "--body", "channels")
	var notes []noteJSON
	json.Unmarshal([]byte(executeCmd(t, "notes", "--tag", "go", "--search", "channels", "--json")), &notes)
	if len(notes) != 1 || notes[0].Title != "Go tips" {
		t.Errorf("notes = %+v", notes)
	}
}

func TestBoardsByPathOrAnyCaseAndArchivedCardsShow(t *testing.T) { // C26
	setupTestDB(t)
	createTestBoard(t, "Work")
	card, _ := createTestCard(t, testLanes(t, "Work")[0], "Old", "")
	executeCmd(t, "cards", "archive", card.ID, "-B", "work")
	out := executeCmd(t, "cards", "show", card.ID, "--board", "Boards/Work.md", "--json")
	var c cardJSON
	json.Unmarshal([]byte(out), &c)
	if !c.Archived || c.Board != "Work" {
		t.Errorf("card = %+v", c)
	}
}

func TestCardsAreFoundOnAnyBoardWithoutABoard(t *testing.T) {
	setupTestDB(t)
	t.Setenv("KB_BOARD", "")
	createTestBoard(t, "Elsewhere")
	card, _ := createTestCard(t, testLanes(t, "Elsewhere")[1], "Far away", "")
	t.Chdir(t.TempDir()) // a folder named after no board
	out := executeCmd(t, "cards", "move", card.ID[:5], "done", "--json")
	var c cardJSON
	json.Unmarshal([]byte(out), &c)
	if c.Column != "Done" || c.Board != "Elsewhere" {
		t.Errorf("card = %+v", c)
	}
	if _, err := executeCmdErr(t, "cards"); err == nil || !strings.Contains(err.Error(), "--board") {
		t.Errorf("listing with no board: err = %v, want a hint about --board", err)
	}
}

func TestLabelsBecomeTagsAndPriorityATag(t *testing.T) {
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	out := executeCmd(t, "cards", "add", "Tagged", "-l", "needs review, ui", "-p", "high", "--json")
	var c cardJSON
	json.Unmarshal([]byte(out), &c)
	if strings.Join(c.Labels, ",") != "needs-review,ui" {
		t.Errorf("labels = %v", c.Labels)
	}
	raw, _ := os.ReadFile(filepath.Join(db.Vault().Root(), "Boards", "test-board.md"))
	if !strings.Contains(string(raw), "- [ ] Tagged #needs-review #ui #priority/high") {
		t.Errorf("board file:\n%s", raw)
	}
	// Medium is the default, so it is not written.
	executeCmd(t, "cards", "edit", c.ID, "-p", "medium")
	raw, _ = os.ReadFile(filepath.Join(db.Vault().Root(), "Boards", "test-board.md"))
	if strings.Contains(string(raw), "#priority/") {
		t.Errorf("board file:\n%s", raw)
	}
}

func TestWIPLimitsNeedForce(t *testing.T) {
	setupTestDB(t)
	t.Setenv("KB_BOARD", "test-board")
	createTestBoard(t, "test-board")
	executeCmd(t, "columns", "wip-limit", "todo", "1")
	executeCmd(t, "cards", "add", "First", "-c", "Todo")
	if _, err := executeCmdErr(t, "cards", "add", "Second", "-c", "Todo"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("err = %v", err)
	}
	executeCmd(t, "cards", "add", "Second", "-c", "Todo", "-f")
	raw, _ := os.ReadFile(filepath.Join(db.Vault().Root(), "Boards", "test-board.md"))
	if !strings.Contains(string(raw), "## Todo (1)") {
		t.Errorf("the limit belongs in the lane title:\n%s", raw)
	}
}

// The real start-up path: kb opens the vault named by $KB_VAULT, and
// refuses to run while a 0.3 database waits to be imported (C28).
func TestStartupOpensTheVaultAndWaitsForAnImport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KB_VAULT", filepath.Join(dir, "vault"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	db = nil
	t.Cleanup(func() { db = nil })

	executeCmd(t, "notes", "create", "Real")
	if _, err := os.Stat(filepath.Join(dir, "vault", "Real.md")); err != nil {
		t.Fatal(err)
	}

	db = nil
	os.MkdirAll(filepath.Join(dir, "data", "kb"), 0o755)
	fakeLegacyDB(t, filepath.Join(dir, "data", "kb", "kb.db"))
	_, err := executeCmdErr(t, "notes")
	if err == nil || !strings.Contains(err.Error(), "kb import") {
		t.Errorf("err = %v, want it to say to run kb import", err)
	}
}

func TestImportCommandDryRunThenImport(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		missing(t, "sqlite3 is not installed")
	}
	dir := t.TempDir()
	t.Setenv("KB_VAULT", filepath.Join(dir, "vault"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("KB_BOARD", "")
	db = nil
	t.Cleanup(func() { db = nil })

	dbPath := filepath.Join(dir, "data", "kb", "kb.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	schema, err := os.ReadFile("../internal/legacy/testdata/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("sqlite3", dbPath)
	build.Stdin = strings.NewReader(string(schema) + `
INSERT INTO workspaces (id, name, kind) VALUES ('w0', 'Default', 'area');
INSERT INTO boards (id, name, workspace_id) VALUES ('b1', 'Side Project', 'w0');
INSERT INTO columns (id, board_id, name, position) VALUES ('c1', 'b1', 'Todo', 0), ('c2', 'b1', 'Done', 1);
INSERT INTO cards (id, column_id, title, labels, priority, position) VALUES ('0123abcd-0000-4000-8000-000000000000', 'c1', 'First task', 'label 1', 'urgent', 0);
`)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	out := executeCmd(t, "import", "--dry-run")
	if !strings.Contains(out, "board Side Project (Boards/Side Project.md): 1 card") || !strings.Contains(out, `"label 1" becomes #label-1`) || !strings.Contains(out, "Nothing was written") {
		t.Errorf("dry run:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault", "Boards")); !os.IsNotExist(err) {
		t.Error("a dry run wrote to the vault")
	}
	db = nil
	if _, err := executeCmdErr(t, "boards"); err == nil {
		t.Error("kb should still wait for the import")
	}

	db = nil
	out = executeCmd(t, "import")
	if !strings.Contains(out, "Done.") {
		t.Errorf("import:\n%s", out)
	}
	if _, err := os.Stat(dbPath + ".imported-0.4"); err != nil {
		t.Error(err)
	}
	db = nil
	out = executeCmd(t, "cards", "show", "0123abcd", "--json")
	var c cardJSON
	json.Unmarshal([]byte(out), &c)
	if c.Title != "First task" || c.Priority != "urgent" || strings.Join(c.Labels, ",") != "label-1" || c.Board != "Side Project" {
		t.Errorf("imported card = %+v", c)
	}
	db = nil
	if out := executeCmd(t, "import"); !strings.Contains(out, "Already imported") {
		t.Errorf("second import:\n%s", out)
	}
}

// kb opened from dev's popup runs in the workspace's folder: a worktree
// such as ~/.worktrees/allocator-one/sample, or a folder inside a repo.
// The board is the repository's, not the folder's.
func TestTheBoardComesFromTheGitRepoOfTheFolder(t *testing.T) {
	setupTestDB(t)
	t.Setenv("KB_BOARD", "")
	t.Setenv("TMUX_SESSION_NAME", "")
	createTestBoard(t, "allocator-one")
	createTestTitle := func(title string) {
		createTestCard(t, testLanes(t, "allocator-one")[0], title, "")
	}
	createTestTitle("On the repo's board")

	root := t.TempDir()
	repo := filepath.Join(root, "code", "allocator-one")
	os.MkdirAll(filepath.Join(repo, ".git", "worktrees", "sample"), 0o755)
	os.MkdirAll(filepath.Join(repo, "lib", "deep"), 0o755)
	tree := filepath.Join(root, ".worktrees", "allocator-one", "sample")
	os.MkdirAll(filepath.Join(tree, "assets"), 0o755)
	os.WriteFile(filepath.Join(tree, ".git"), []byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "sample")+"\n"), 0o644)

	for _, dir := range []string{filepath.Join(repo, "lib", "deep"), tree, filepath.Join(tree, "assets")} {
		t.Chdir(dir)
		out, err := executeCmdErr(t, "cards")
		if err != nil || !strings.Contains(out, "On the repo's board") {
			t.Errorf("in %s: %v\n%s", dir, err, out)
		}
	}

	// A board named after the folder itself still comes first.
	createTestBoard(t, "sample")
	t.Chdir(tree)
	if out := executeCmd(t, "cards"); !strings.Contains(out, `No cards on board "sample"`) {
		t.Errorf("the folder's own board should win:\n%s", out)
	}

	// $KB_BOARD names one board, and a missing one is an error.
	t.Setenv("KB_BOARD", "nope")
	if _, err := executeCmdErr(t, "cards"); err == nil {
		t.Error("a missing $KB_BOARD board should be an error")
	}
}

func TestOnlyTheTUIInATmuxTerminalWaitsAfterAnError(t *testing.T) {
	if !holdOnError(rootCmd, true, true) {
		t.Error("the TUI in a tmux popup should wait")
	}
	if holdOnError(noteCmd, true, true) || holdOnError(rootCmd, false, true) || holdOnError(rootCmd, true, false) {
		t.Error("only the TUI, in a terminal, in tmux")
	}
}

func TestASitesPermalinkPatternShapesLinksBetweenPosts(t *testing.T) { // C11
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site, "--permalink", "/posts/:title/")
	executeCmd(t, "notes", "create", "First", "--body", "one")
	executeCmd(t, "notes", "create", "Second", "--body", "see [[First]]")
	executeCmd(t, "publish", "first")
	executeCmd(t, "publish", "second")
	posts, _ := filepath.Glob(filepath.Join(site, "_posts", "*second.md"))
	if len(posts) != 1 {
		t.Fatalf("posts = %v", posts)
	}
	data, _ := os.ReadFile(posts[0])
	if !strings.Contains(string(data), "[First](/posts/first/)") {
		t.Errorf("post:\n%s", data)
	}
}

func TestANoteNamedLikeASubcommandCanBePublished(t *testing.T) { // C25
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site)
	executeCmd(t, "notes", "create", "List", "--body", "a list")
	out := executeCmd(t, "publish", "--", "list")
	if !strings.Contains(out, `Published "List"`) {
		t.Errorf("output: %s", out)
	}
}

// A note whose frontmatter is broken can still be found and deleted, and
// an edit says what to fix rather than overwrite it (A11).
func TestANoteWithBrokenFrontmatter(t *testing.T) {
	setupTestDB(t)
	path := filepath.Join(db.Vault().Root(), "Broken.md")
	os.WriteFile(path, []byte("---\ntitle: [unclosed\n---\nbody\n"), 0o644)
	db.Reload()
	if _, err := executeCmdErr(t, "notes", "edit", "broken", "--body", "x"); err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Errorf("edit: err = %v", err)
	}
	executeCmd(t, "notes", "delete", "broken", "-f")
	if _, err := os.Stat(filepath.Join(db.Vault().Root(), ".trash", "Broken.md")); err != nil {
		t.Error(err)
	}
}

func TestVersionWorksWhileAnImportWaits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	os.MkdirAll(filepath.Join(dir, "kb"), 0o755)
	fakeLegacyDB(t, filepath.Join(dir, "kb", "kb.db"))
	db = nil
	t.Cleanup(func() { db = nil })
	if out := executeCmd(t, "--version"); !strings.HasPrefix(out, "kb version ") {
		t.Errorf("output: %q", out)
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

func TestTheGuidesOpenWithKbHelp(t *testing.T) {
	for topic, want := range map[string]string{
		"start":     "Connect two notes with a link",
		"links":     "[[Cash and Cash Equivalents|cash]]",
		"kanban":    "kb card move <id> Done -B study",
		"tui":       "1 2 3 4",
		"files":     "aliases: [PCF]",
		"scripting": "kb cards -B study --json",
	} {
		out := executeCmd(t, "help", topic)
		if !strings.Contains(out, want) {
			t.Errorf("kb help %s lacks %q", topic, want)
		}
	}
	if out := executeCmd(t, "--help"); !strings.Contains(out, "kb help start") || !strings.Contains(out, "Additional help topics") {
		t.Errorf("kb --help should point to the guides:\n%s", out)
	}
}

// Every command that does something shows how to use it.
func TestEveryCommandHasExamples(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Runnable() && sub.Example == "" && sub.Name() != "help" && sub.Parent().Name() != "completion" && sub.Name() != "completion" {
				t.Errorf("%s has no examples", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(rootCmd)
}

func TestTheFullScreenViewExplainsAMissingTerminal(t *testing.T) {
	setupTestDB(t)
	// Tests run with no terminal, as an AI assistant's shell does.
	_, err := executeCmdErr(t)
	if !errors.Is(err, errNoTerminal) || !strings.Contains(err.Error(), "kb notes") {
		t.Errorf("err = %v", err)
	}
}

// fakeLegacyDB writes a file that starts as an SQLite database does, which
// is all kb looks at before it stops for an import.
func fakeLegacyDB(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, append([]byte("SQLite format 3\x00"), make([]byte, 84)...), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A kb.db that is empty or not a database (a failed copy, a stray file)
// holds nothing to import, so it never stops kb.
func TestAKbDBThatIsNoDatabaseDoesNotStopKb(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "damaged": "not a database at all"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("KB_VAULT", filepath.Join(dir, "vault"))
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
			t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
			t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
			db = nil
			t.Cleanup(func() { db = nil })
			os.MkdirAll(filepath.Join(dir, "data", "kb"), 0o755)
			os.WriteFile(filepath.Join(dir, "data", "kb", "kb.db"), []byte(content), 0o644)

			executeCmd(t, "notes", "create", "Still works")
			db = nil
			_, err := executeCmdErr(t, "import")
			if err == nil || !strings.Contains(err.Error(), "not an SQLite database") {
				t.Errorf("import: err = %v, want it to say the file is not a database", err)
			}
		})
	}
}

// An SQLite file that is not kb 0.3's says so, and how to get past it.
func TestImportingADatabaseThatIsNotKbsSaysSo(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		missing(t, "sqlite3 is not installed")
	}
	dir := t.TempDir()
	t.Setenv("KB_VAULT", filepath.Join(dir, "vault"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	db = nil
	t.Cleanup(func() { db = nil })
	dbPath := filepath.Join(dir, "data", "kb", "kb.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	build := exec.Command("sqlite3", dbPath, "CREATE TABLE other (x);")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	_, err := executeCmdErr(t, "import")
	if err == nil || !strings.Contains(err.Error(), "not a kb 0.3 database") || !strings.Contains(err.Error(), "mv ") {
		t.Errorf("err = %v, want it to say this is not kb 0.3's database and how to move it aside", err)
	}
}

// A permalink kb cannot fill in is refused when the site is set up, not
// discovered later as broken links.
func TestAPermalinkKbCannotFillIsRefusedAtSetup(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	if _, err := executeCmdErr(t, "publish", "setup", "site", "--path", site, "--permalink", "pretty"); err == nil {
		t.Error("want an error for a Jekyll style name")
	}
	if _, err := executeCmdErr(t, "publish", "setup", "site", "--path", site, "--permalink", "/:categories/:title/"); err == nil {
		t.Error("want an error for :categories")
	}
	if len(db.ListPublishTargets()) != 0 {
		t.Error("a target was saved")
	}
}

// A dry run reports the post's path as a publish does: relative to the
// site, with the full path beside it.
func TestADryRunReportsThePathAsAPublishDoes(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site)
	executeCmd(t, "notes", "create", "Post", "--body", "text")
	var dry, real struct {
		FilePath string `json:"file_path"`
		FullPath string `json:"full_path"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "publish", "post", "--dry-run", "--json")), &dry); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "publish", "post", "--json")), &real); err != nil {
		t.Fatal(err)
	}
	if dry != real || filepath.IsAbs(dry.FilePath) || dry.FullPath != filepath.Join(site, dry.FilePath) {
		t.Errorf("dry run %+v, publish %+v", dry, real)
	}
}

// A card found on a board other than the current one is deleted only once
// the question has named that board.
func TestDeletingACardOnAnotherBoardNamesTheBoard(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "boards", "create", "Home")
	executeCmd(t, "boards", "create", "Work")
	var card struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "card", "add", "Fix sink", "-B", "Home", "--json")), &card); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KB_BOARD", "Work")
	rootCmd.SetIn(strings.NewReader("n\n"))
	defer rootCmd.SetIn(nil)
	out, err := executeCmdErr(t, "card", "delete", card.ID)
	if !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, `on board "Home"`) {
		t.Errorf("the question does not name the board:\n%s", out)
	}
}

// workspace show --json holds what the text shows: the boards and notes.
func TestWorkspaceShowJSONListsItsBoardsAndNotes(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "workspace", "create", "Work")
	executeCmd(t, "boards", "create", "Sprint", "-w", "Work")
	executeCmd(t, "notes", "create", "Plan", "-w", "Work")
	var got struct {
		Name   string   `json:"name"`
		Boards []string `json:"boards"`
		Notes  []string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "workspace", "show", "Work", "--json")), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Work" || strings.Join(got.Boards, ",") != "Sprint" || strings.Join(got.Notes, ",") != "plan" {
		t.Errorf("got %+v", got)
	}
}

// --open and --json ask for two different things; kb says so rather than
// silently doing one.
func TestGraphOpenAndJSONTogetherIsAnError(t *testing.T) {
	setupTestDB(t)
	if _, err := executeCmdErr(t, "graph", "--open", "--json"); err == nil {
		t.Error("want an error")
	}
}

// The board pick: --board, else $KB_BOARD, else the first board that
// exists named after the dev tmux session, the folder, or the repository.
func TestTheBoardPickOrder(t *testing.T) {
	setupTestDB(t)
	for _, name := range []string{"flagged", "env", "session", "folder"} {
		createTestBoard(t, name)
		createTestCard(t, testLanes(t, name)[0], "card on "+name, "")
	}
	dir := filepath.Join(t.TempDir(), "folder")
	os.MkdirAll(dir, 0o755)
	t.Chdir(dir)
	pick := func() string {
		t.Helper()
		out, err := executeCmdErr(t, "cards")
		if err != nil {
			return err.Error()
		}
		for _, name := range []string{"flagged", "env", "session", "folder"} {
			if strings.Contains(out, "card on "+name) {
				return name
			}
		}
		return out
	}
	t.Setenv("KB_BOARD", "")
	t.Setenv("TMUX_SESSION_NAME", "")
	if got := pick(); got != "folder" {
		t.Errorf("folder only: %s", got)
	}
	t.Setenv("TMUX_SESSION_NAME", "dev-session")
	if got := pick(); got != "session" {
		t.Errorf("with a tmux session: %s", got)
	}
	t.Setenv("TMUX_SESSION_NAME", "dev-no-such-board")
	if got := pick(); got != "folder" {
		t.Errorf("a session with no board falls through to the folder: %s", got)
	}
	t.Setenv("KB_BOARD", "env")
	if got := pick(); got != "env" {
		t.Errorf("with $KB_BOARD: %s", got)
	}
	out, err := executeCmdErr(t, "cards", "-B", "flagged")
	if err != nil || !strings.Contains(out, "card on flagged") {
		t.Errorf("with --board: %v\n%s", err, out)
	}
}

// card move --before puts the card just above the other one, and refuses
// a card that is not in the target column.
func TestCardMoveBefore(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "boards", "create", "B")
	add := func(title, col string) string {
		var c struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(executeCmd(t, "card", "add", title, "-B", "B", "-c", col, "--json")), &c); err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	first := add("first", "Done")
	second := add("second", "Done")
	mover := add("mover", "Todo")
	executeCmd(t, "card", "move", mover, "Done", "--before", second, "-B", "B")
	var cards []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "cards", "-B", "B", "-c", "Done", "--json")), &cards); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, c := range cards {
		order = append(order, c.Title)
	}
	if strings.Join(order, ",") != "first,mover,second" {
		t.Errorf("order = %v", order)
	}
	if _, err := executeCmdErr(t, "card", "move", first, "Todo", "--before", second, "-B", "B"); err == nil {
		t.Error("--before a card in another column: want an error")
	}
}

// kb 0.3 killed before its first checkpoint leaves kb.db empty and every
// board in kb.db-wal: that still waits for an import, and kb never says to
// remove it.
func TestAnEmptyKbDBWithAWALStillWaitsForTheImport(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KB_VAULT", filepath.Join(dir, "vault"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	db = nil
	t.Cleanup(func() { db = nil })
	dbPath := filepath.Join(dir, "data", "kb", "kb.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	os.WriteFile(dbPath, nil, 0o644)
	os.WriteFile(dbPath+"-wal", []byte("wal frames"), 0o644)
	if _, err := executeCmdErr(t, "notes"); err == nil || !strings.Contains(err.Error(), "kb import") {
		t.Errorf("notes: err = %v, want it to wait for the import", err)
	}
	db = nil
	_, err := executeCmdErr(t, "import")
	if err == nil || strings.Contains(err.Error(), "rm ") || !strings.Contains(err.Error(), "-wal") {
		t.Errorf("import: err = %v, want the WAL explained and no rm", err)
	}
}

// A posts folder kb cannot use is an error, not an endless search for a
// free post name.
func TestPublishingIntoAnUnusablePostsFolderFails(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site)
	os.WriteFile(filepath.Join(site, "_posts"), []byte("a file, not a folder"), 0o644)
	executeCmd(t, "notes", "create", "Post", "--body", "text")
	done := make(chan error, 1)
	go func() { _, err := executeCmdErr(t, "publish", "post", "--dry-run"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publish did not return")
	}
}

// A note still in iCloud is not published: kb has only its name, and its
// empty body would replace the post.
func TestANoteInICloudIsNotPublished(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site)
	os.WriteFile(filepath.Join(db.Vault().Root(), "Remote.md"), []byte("the real text\n"), 0o644)
	vault.IsDataless = func(fi fs.FileInfo) bool { return fi.Name() == "Remote.md" }
	t.Cleanup(func() { vault.IsDataless = func(fs.FileInfo) bool { return false } })
	db.Reload()
	if _, err := executeCmdErr(t, "publish", "remote"); err == nil || !strings.Contains(err.Error(), "iCloud") {
		t.Errorf("err = %v", err)
	}
	if posts, _ := filepath.Glob(filepath.Join(site, "_posts", "*")); len(posts) != 0 {
		t.Errorf("posts = %v", posts)
	}
}

// When the note cannot record where it was published, no post is left
// behind for the next publish to duplicate.
func TestAPublishThatCannotBeRecordedWritesNoPost(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--path", site)
	dir := filepath.Join(db.Vault().Root(), "locked")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "Locked.md"), []byte("---\ncreated: 2026-09-01\n---\ntext\n"), 0o644)
	db.Reload()
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if _, err := executeCmdErr(t, "publish", "locked"); err == nil {
		t.Fatal("want an error")
	}
	if posts, _ := filepath.Glob(filepath.Join(site, "_posts", "*")); len(posts) != 0 {
		t.Errorf("a post was left: %v", posts)
	}
	os.Chmod(dir, 0o755)
	db.Reload()
	executeCmd(t, "publish", "locked")
	if posts, _ := filepath.Glob(filepath.Join(site, "_posts", "*")); len(posts) != 1 {
		t.Errorf("posts after a retry = %v", posts)
	}
}

// Links between posts carry the site's baseurl, read from its _config.yml.
func TestLinksBetweenPostsCarryTheSitesBaseurl(t *testing.T) {
	setupTestDB(t)
	site := t.TempDir()
	os.WriteFile(filepath.Join(site, "_config.yml"), []byte("title: my site\nbaseurl: /repo\n"), 0o644)
	executeCmd(t, "publish", "setup", "site", "--path", site, "--permalink", "/posts/:title/")
	executeCmd(t, "notes", "create", "First", "--body", "one")
	executeCmd(t, "notes", "create", "Second", "--body", "see [[First]]")
	executeCmd(t, "publish", "first")
	executeCmd(t, "publish", "second")
	posts, _ := filepath.Glob(filepath.Join(site, "_posts", "*second.md"))
	data, _ := os.ReadFile(posts[0])
	if !strings.Contains(string(data), "[First](/repo/posts/first/)") {
		t.Errorf("post:\n%s", data)
	}
}

// With several sites and no --target, a note goes to the site set up for
// its workspace, when exactly one is.
func TestANoteGoesToItsWorkspacesSite(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "workspace", "create", "Writing")
	work, blog := t.TempDir(), t.TempDir()
	executeCmd(t, "publish", "setup", "work", "--path", work)
	executeCmd(t, "publish", "setup", "blog", "--path", blog, "-w", "Writing")
	executeCmd(t, "notes", "create", "Essay", "--body", "words", "-w", "Writing")
	executeCmd(t, "publish", "essay")
	if posts, _ := filepath.Glob(filepath.Join(blog, "_posts", "*essay.md")); len(posts) != 1 {
		t.Errorf("blog posts = %v", posts)
	}
	executeCmd(t, "notes", "create", "Memo", "--body", "words")
	if _, err := executeCmdErr(t, "publish", "memo"); err == nil {
		t.Error("a note no site is set up for, with two sites: want an error")
	}
}

// JSON times are all in one zone, local time with its offset.
func TestJSONTimesShareAZone(t *testing.T) {
	setupTestDB(t)
	os.WriteFile(filepath.Join(db.Vault().Root(), "Z.md"), []byte("---\ncreated: 2026-10-04T04:53:56Z\n---\nbody\n"), 0o644)
	db.Reload()
	var n struct {
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "note", "show", "z", "--json")), &n); err != nil {
		t.Fatal(err)
	}
	if n.CreatedAt[19:] != n.UpdatedAt[19:] {
		t.Errorf("zones differ: %s and %s", n.CreatedAt, n.UpdatedAt)
	}
}

// Column names can hold commas; reorder takes them as separate arguments.
func TestReorderTakesColumnsAsArguments(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "boards", "create", "B")
	executeCmd(t, "column", "add", "Q&A, later", "-B", "B")
	lanes := testLanes(t, "B")
	var names []string
	for i := len(lanes) - 1; i >= 0; i-- {
		names = append(names, lanes[i].Name)
	}
	executeCmd(t, append([]string{"column", "reorder", "-B", "B"}, names...)...)
	if got := testLanes(t, "B")[0].Name; got != "Q&A, later" {
		t.Errorf("first column = %q", got)
	}
}

// KB_DAILY_DIR may be a path in the vault, absolute or not; outside it is
// an error that says so.
func TestTheDailyFolderSetting(t *testing.T) {
	setupTestDB(t)
	root := db.Vault().Root()
	t.Setenv("KB_DAILY_DIR", filepath.Join(root, "journal"))
	var n struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "daily", "--date", "2026-10-04", "--json")), &n); err != nil {
		t.Fatal(err)
	}
	if n.Path != "journal/2026-10-04.md" {
		t.Errorf("path = %q", n.Path)
	}
	t.Setenv("KB_DAILY_DIR", "../elsewhere")
	if _, err := executeCmdErr(t, "daily", "--date", "2026-10-04", "--json"); err == nil || !strings.Contains(err.Error(), "outside the vault") {
		t.Errorf("err = %v", err)
	}
}

// note delete --json says where the file went.
func TestNoteDeleteJSONSaysWhereTheFileWent(t *testing.T) {
	setupTestDB(t)
	executeCmd(t, "notes", "create", "Gone")
	var out struct {
		Trashed string `json:"trashed"`
	}
	if err := json.Unmarshal([]byte(executeCmd(t, "notes", "delete", "gone", "-f", "--json")), &out); err != nil {
		t.Fatal(err)
	}
	if out.Trashed != ".trash/Gone.md" {
		t.Errorf("trashed = %q", out.Trashed)
	}
}

// note show on a note still in iCloud says its text is not here yet.
func TestShowingANoteInICloudSaysSo(t *testing.T) {
	setupTestDB(t)
	os.WriteFile(filepath.Join(db.Vault().Root(), "Remote.md"), []byte("text\n"), 0o644)
	vault.IsDataless = func(fi fs.FileInfo) bool { return fi.Name() == "Remote.md" }
	t.Cleanup(func() { vault.IsDataless = func(fs.FileInfo) bool { return false } })
	db.Reload()
	out := executeCmd(t, "note", "show", "remote")
	if !strings.Contains(out, "iCloud") {
		t.Errorf("output:\n%s", out)
	}
}

// graph --open writes one page in kb's cache folder, replaced each time,
// not a new temporary file per run.
func TestGraphOpenReusesOnePage(t *testing.T) {
	setupTestDB(t)
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	bin := t.TempDir()
	for _, name := range []string{"open", "xdg-open"} {
		os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	executeCmd(t, "notes", "create", "A", "--body", "[[B]]")
	executeCmd(t, "graph", "--open")
	executeCmd(t, "graph", "--open")
	pages, _ := filepath.Glob(filepath.Join(cache, "kb", "*.html"))
	if len(pages) != 1 {
		t.Errorf("pages = %v", pages)
	}
}
