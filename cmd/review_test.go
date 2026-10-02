package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	os.WriteFile(filepath.Join(dir, "data", "kb", "kb.db"), nil, 0o644)
	_, err := executeCmdErr(t, "notes")
	if err == nil || !strings.Contains(err.Error(), "kb import") {
		t.Errorf("err = %v, want it to say to run kb import", err)
	}
}
