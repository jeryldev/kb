package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	testDB, err := store.OpenWithPath(":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	db = testDB
	origPostRun := rootCmd.PersistentPostRunE
	rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error { return nil }
	t.Cleanup(func() {
		db.Close()
		db = nil
		rootCmd.PersistentPostRunE = origPostRun
	})
}

func resetFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
		f.Value.Set(f.DefValue)
	})
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

func executeCmd(t *testing.T, args ...string) string {
	t.Helper()
	resetFlags(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("command %v failed: %v\noutput: %s", args, err, buf.String())
	}
	return buf.String()
}

func createTestBoard(t *testing.T, name string) {
	t.Helper()
	_, err := db.CreateBoard(name, "test board")
	if err != nil {
		t.Fatalf("creating test board: %v", err)
	}
}

// --- Board tests ---

func TestBoardsListJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "alpha")
	createTestBoard(t, "beta")

	out := executeCmd(t, "boards", "--json")

	var boards []boardJSON
	if err := json.Unmarshal([]byte(out), &boards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(boards) != 2 {
		t.Fatalf("expected 2 boards, got %d", len(boards))
	}
	if boards[0].Name != "alpha" {
		t.Errorf("expected first board name 'alpha', got %q", boards[0].Name)
	}
	if boards[0].ID == "" {
		t.Error("expected board ID to be populated")
	}
}

func TestBoardsCreateJSON(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "boards", "create", "my-board", "-d", "A test board", "--json")

	var board boardJSON
	if err := json.Unmarshal([]byte(out), &board); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if board.Name != "my-board" {
		t.Errorf("expected name 'my-board', got %q", board.Name)
	}
	if board.Description != "A test board" {
		t.Errorf("expected description 'A test board', got %q", board.Description)
	}
	if board.ID == "" {
		t.Error("expected board ID to be populated")
	}
}

func TestBoardsDeleteJSON(t *testing.T) {
	setupTestDB(t)

	createTestBoard(t, "doomed")

	out := executeCmd(t, "boards", "delete", "doomed", "-f", "--json")

	var board boardJSON
	if err := json.Unmarshal([]byte(out), &board); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if board.Name != "doomed" {
		t.Errorf("expected name 'doomed', got %q", board.Name)
	}
}

// --- Card tests ---

func TestCardsListJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	db.CreateCard(columns[0].ID, "First card", "medium")
	db.CreateCard(columns[0].ID, "Second card", "high")

	out := executeCmd(t, "cards", "--json")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(cards))
	}
	if cards[0].Column != "Backlog" {
		t.Errorf("expected column 'Backlog', got %q", cards[0].Column)
	}
	if len(cards[0].ID) < 8 {
		t.Error("expected full card ID")
	}
}

func TestCardsAddJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards", "add", "My task", "-p", "high", "-c", "Todo", "--json")

	var card cardJSON
	if err := json.Unmarshal([]byte(out), &card); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if card.Title != "My task" {
		t.Errorf("expected title 'My task', got %q", card.Title)
	}
	if card.Priority != "high" {
		t.Errorf("expected priority 'high', got %q", card.Priority)
	}
	if card.Column != "Todo" {
		t.Errorf("expected column 'Todo', got %q", card.Column)
	}
}

func TestCardsAddWithExtraFields(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards", "add", "Full card",
		"-d", "A detailed description",
		"-l", "bug,frontend",
		"-e", "JIRA-123",
		"--json")

	var card cardJSON
	if err := json.Unmarshal([]byte(out), &card); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if card.Description != "A detailed description" {
		t.Errorf("expected description, got %q", card.Description)
	}
	if card.Labels != "bug,frontend" {
		t.Errorf("expected labels 'bug,frontend', got %q", card.Labels)
	}
	if card.ExternalID != "JIRA-123" {
		t.Errorf("expected external_id 'JIRA-123', got %q", card.ExternalID)
	}
}

func TestCardsShowJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[1].ID, "Show me", "urgent")

	out := executeCmd(t, "cards", "show", card.ID[:8], "--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Title != "Show me" {
		t.Errorf("expected title 'Show me', got %q", result.Title)
	}
	if result.Priority != "urgent" {
		t.Errorf("expected priority 'urgent', got %q", result.Priority)
	}
	if result.Column != "Todo" {
		t.Errorf("expected column 'Todo', got %q", result.Column)
	}
}

func TestCardsEditJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Original title", "medium")

	out := executeCmd(t, "cards", "edit", card.ID[:8],
		"-t", "Updated title",
		"-p", "urgent",
		"--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Title != "Updated title" {
		t.Errorf("expected title 'Updated title', got %q", result.Title)
	}
	if result.Priority != "urgent" {
		t.Errorf("expected priority 'urgent', got %q", result.Priority)
	}
}

func TestCardsEditPartial(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Keep this title", "high")
	card.Labels = "original"
	db.UpdateCard(card)

	out := executeCmd(t, "cards", "edit", card.ID[:8],
		"-l", "updated-label",
		"--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Title != "Keep this title" {
		t.Errorf("title should be unchanged, got %q", result.Title)
	}
	if result.Priority != "high" {
		t.Errorf("priority should be unchanged, got %q", result.Priority)
	}
	if result.Labels != "updated-label" {
		t.Errorf("expected labels 'updated-label', got %q", result.Labels)
	}
}

func TestCardsMoveJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Moving card", "medium")

	out := executeCmd(t, "cards", "move", card.ID[:8], "Done", "--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Column != "Done" {
		t.Errorf("expected column 'Done', got %q", result.Column)
	}
}

func TestCardsArchiveJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Archive me", "low")

	out := executeCmd(t, "cards", "archive", card.ID[:8], "--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Title != "Archive me" {
		t.Errorf("expected title 'Archive me', got %q", result.Title)
	}
}

func TestCardsDeleteJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Delete me", "medium")

	out := executeCmd(t, "cards", "delete", card.ID[:8], "--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if result.Title != "Delete me" {
		t.Errorf("expected title 'Delete me', got %q", result.Title)
	}
}

// --- Column tests ---

func TestColumnsListJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	db.CreateCard(columns[0].ID, "Card 1", "medium")
	db.CreateCard(columns[0].ID, "Card 2", "high")

	out := executeCmd(t, "columns", "--json")

	var cols []columnJSON
	if err := json.Unmarshal([]byte(out), &cols); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
	if cols[0].Name != "Backlog" {
		t.Errorf("expected first column 'Backlog', got %q", cols[0].Name)
	}
	if cols[0].Cards != 2 {
		t.Errorf("expected 2 cards in Backlog, got %d", cols[0].Cards)
	}
}

func TestColumnsAddJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "add", "Testing", "--json")

	var col columnJSON
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if col.Name != "Testing" {
		t.Errorf("expected name 'Testing', got %q", col.Name)
	}
	if col.Position != 5 {
		t.Errorf("expected position 5, got %d", col.Position)
	}
}

func TestColumnsDeleteJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "delete", "Review", "-f", "--json")

	var col columnJSON
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if col.Name != "Review" {
		t.Errorf("expected name 'Review', got %q", col.Name)
	}
}

func TestColumnsWIPLimitSetJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "wip-limit", "In Progress", "3", "--json")

	var col columnJSON
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if col.Name != "In Progress" {
		t.Errorf("expected name 'In Progress', got %q", col.Name)
	}
	if col.WIPLimit == nil || *col.WIPLimit != 3 {
		t.Errorf("expected WIP limit 3, got %v", col.WIPLimit)
	}
}

func TestColumnsWIPLimitClearJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	executeCmd(t, "columns", "wip-limit", "In Progress", "5", "--json")
	out := executeCmd(t, "columns", "wip-limit", "In Progress", "0", "--json")

	var col columnJSON
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if col.WIPLimit != nil {
		t.Errorf("expected WIP limit to be null, got %d", *col.WIPLimit)
	}
}

func TestColumnsReorderJSON(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	boardID := mustBoardID(t, "test-board")
	columns, _ := db.ListColumns(boardID)

	reversed := make([]string, len(columns))
	for i, col := range columns {
		reversed[len(columns)-1-i] = col.ID
	}
	idStr := ""
	for i, id := range reversed {
		if i > 0 {
			idStr += ","
		}
		idStr += id
	}

	out := executeCmd(t, "columns", "reorder", idStr, "--json")

	var cols []columnJSON
	if err := json.Unmarshal([]byte(out), &cols); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if cols[0].Name != "Done" {
		t.Errorf("expected first column 'Done' after reorder, got %q", cols[0].Name)
	}
}

// --- Helpers ---

func mustBoardID(t *testing.T, name string) string {
	t.Helper()
	board, err := db.GetBoardByName(name)
	if err != nil {
		t.Fatalf("getting board: %v", err)
	}
	if board == nil {
		t.Fatalf("board %q not found", name)
	}
	return board.ID
}

func executeCmdErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetFlags(rootCmd)
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// --- Output helpers tests ---

func TestFormatTime(t *testing.T) {
	ts := time.Date(2026, 2, 23, 14, 30, 0, 0, time.UTC)
	got := formatTime(ts)
	if got != "2026-02-23T14:30:00Z" {
		t.Errorf("formatTime() = %q, want RFC3339 format", got)
	}
}

func TestToBoardJSON(t *testing.T) {
	now := time.Now().UTC()
	b := &model.Board{
		ID: "board-123", Name: "test", Description: "desc",
		CreatedAt: now, UpdatedAt: now,
	}
	j := toBoardJSON(b)
	if j.ID != "board-123" || j.Name != "test" || j.Description != "desc" {
		t.Errorf("toBoardJSON() fields mismatch: %+v", j)
	}
}

func TestToCardJSON(t *testing.T) {
	now := time.Now().UTC()
	c := &model.Card{
		ID: "card-456", ColumnID: "col-1", Title: "Fix bug",
		Description: "desc", Priority: model.PriorityHigh,
		Labels: "bug", ExternalID: "EXT-1",
		CreatedAt: now, UpdatedAt: now,
	}
	j := toCardJSON(c, "Todo")
	if j.Column != "Todo" || j.Title != "Fix bug" || j.Priority != "high" {
		t.Errorf("toCardJSON() fields mismatch: %+v", j)
	}
	if j.Labels != "bug" || j.ExternalID != "EXT-1" {
		t.Errorf("toCardJSON() extra fields mismatch: %+v", j)
	}
}

func TestToColumnJSON(t *testing.T) {
	limit := 5
	col := &model.Column{ID: "col-1", Name: "Todo", Position: 1, WIPLimit: &limit}
	j := toColumnJSON(col, 3)
	if j.Name != "Todo" || j.Position != 1 || j.Cards != 3 {
		t.Errorf("toColumnJSON() fields mismatch: %+v", j)
	}
	if j.WIPLimit == nil || *j.WIPLimit != 5 {
		t.Errorf("toColumnJSON() WIPLimit mismatch: %v", j.WIPLimit)
	}
}

func TestToColumnJSONNilWIPLimit(t *testing.T) {
	col := &model.Column{ID: "col-1", Name: "Todo", Position: 0}
	j := toColumnJSON(col, 0)
	if j.WIPLimit != nil {
		t.Errorf("expected nil WIPLimit, got %v", j.WIPLimit)
	}
}

func TestResolveColumnByName(t *testing.T) {
	setupTestDB(t)
	createTestBoard(t, "test-board")
	boardID := mustBoardID(t, "test-board")

	col, err := resolveColumnByName(boardID, "todo")
	if err != nil {
		t.Fatalf("resolveColumnByName() failed: %v", err)
	}
	if col.Name != "Todo" {
		t.Errorf("expected 'Todo', got %q", col.Name)
	}
}

func TestResolveColumnByNameCaseInsensitive(t *testing.T) {
	setupTestDB(t)
	createTestBoard(t, "test-board")
	boardID := mustBoardID(t, "test-board")

	for _, name := range []string{"IN PROGRESS", "in progress", "In Progress"} {
		col, err := resolveColumnByName(boardID, name)
		if err != nil {
			t.Fatalf("resolveColumnByName(%q) failed: %v", name, err)
		}
		if col.Name != "In Progress" {
			t.Errorf("resolveColumnByName(%q) = %q, want 'In Progress'", name, col.Name)
		}
	}
}

func TestResolveColumnByNameNotFound(t *testing.T) {
	setupTestDB(t)
	createTestBoard(t, "test-board")
	boardID := mustBoardID(t, "test-board")

	_, err := resolveColumnByName(boardID, "Nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent column")
	}
}

// --- Human-readable output tests ---

func TestBoardsListHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "my-project")

	out := executeCmd(t, "boards")

	if !strings.Contains(out, "my-project") {
		t.Errorf("expected board name in output: %s", out)
	}
	if !strings.Contains(out, "ID") {
		t.Errorf("expected ID column header in output: %s", out)
	}
}

func TestBoardsListEmpty(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "boards")

	if !strings.Contains(out, "No boards found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestBoardCreateHuman(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "boards", "create", "new-board")

	if !strings.Contains(out, "Created board") || !strings.Contains(out, "new-board") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestBoardDeleteHumanForce(t *testing.T) {
	setupTestDB(t)
	createTestBoard(t, "to-delete")

	out := executeCmd(t, "boards", "delete", "to-delete", "-f")

	if !strings.Contains(out, "Deleted board") {
		t.Errorf("expected deletion confirmation, got: %s", out)
	}
}

func TestBoardDeleteNotFound(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "boards", "delete", "nonexistent", "-f")
	if err == nil {
		t.Error("expected error for nonexistent board")
	}
}

func TestCardsListHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	db.CreateCard(columns[0].ID, "Test card", "high")

	out := executeCmd(t, "cards")

	if !strings.Contains(out, "Test card") {
		t.Errorf("expected card title in output: %s", out)
	}
	if !strings.Contains(out, "Backlog") {
		t.Errorf("expected column name in output: %s", out)
	}
}

func TestCardsListEmpty(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards")

	if !strings.Contains(out, "No cards") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestCardsAddHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards", "add", "New task")

	if !strings.Contains(out, "Created card") || !strings.Contains(out, "New task") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestCardsAddCaseInsensitiveColumn(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards", "add", "Test", "-c", "todo", "--json")

	var card cardJSON
	json.Unmarshal([]byte(out), &card)
	if card.Column != "Todo" {
		t.Errorf("expected column 'Todo' (case-insensitive match), got %q", card.Column)
	}
}

func TestCardsShowHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Detailed card", "high")
	card.Description = "A long description"
	card.ExternalID = "JIRA-100"
	db.UpdateCard(card)

	out := executeCmd(t, "cards", "show", card.ID[:8])

	if !strings.Contains(out, "Detailed card") {
		t.Errorf("expected title in output: %s", out)
	}
	if !strings.Contains(out, "A long description") {
		t.Errorf("expected description in output: %s", out)
	}
	if !strings.Contains(out, "JIRA-100") {
		t.Errorf("expected external ID in output: %s", out)
	}
}

func TestCardsEditHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Old title", "medium")

	out := executeCmd(t, "cards", "edit", card.ID[:8], "-t", "New title")

	if !strings.Contains(out, "Updated card") || !strings.Contains(out, "New title") {
		t.Errorf("expected update confirmation, got: %s", out)
	}
}

func TestCardsMoveHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Move me", "medium")

	out := executeCmd(t, "cards", "move", card.ID[:8], "Done")

	if !strings.Contains(out, "Moved card to Done") {
		t.Errorf("expected move confirmation, got: %s", out)
	}
}

func TestCardsMoveCaseInsensitiveColumn(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Move me", "medium")

	out := executeCmd(t, "cards", "move", card.ID[:8], "done", "--json")

	var result cardJSON
	json.Unmarshal([]byte(out), &result)
	if result.Column != "Done" {
		t.Errorf("expected column 'Done' (case-insensitive match), got %q", result.Column)
	}
}

func TestCardsArchiveHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Archive me", "medium")

	out := executeCmd(t, "cards", "archive", card.ID[:8])

	if !strings.Contains(out, "Card archived") {
		t.Errorf("expected archive confirmation, got: %s", out)
	}
}

func TestCardsDeleteHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Delete me", "medium")

	out := executeCmd(t, "cards", "delete", card.ID[:8])

	if !strings.Contains(out, "Deleted card") {
		t.Errorf("expected delete confirmation, got: %s", out)
	}
}

func TestColumnsListHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns")

	for _, name := range model.DefaultColumns {
		if !strings.Contains(out, name) {
			t.Errorf("expected column %q in output: %s", name, out)
		}
	}
}

func TestColumnsAddHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "add", "QA")

	if !strings.Contains(out, "Added column") || !strings.Contains(out, "QA") {
		t.Errorf("expected add confirmation, got: %s", out)
	}
}

func TestColumnsReorderHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	ids := make([]string, len(columns))
	for i, col := range columns {
		ids[i] = col.ID
	}

	out := executeCmd(t, "columns", "reorder", strings.Join(ids, ","))

	if !strings.Contains(out, "Columns reordered") {
		t.Errorf("expected reorder confirmation, got: %s", out)
	}
}

func TestColumnsDeleteHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "delete", "Review", "-f")

	if !strings.Contains(out, "Deleted column") {
		t.Errorf("expected delete confirmation, got: %s", out)
	}
}

func TestColumnsWIPLimitSetHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "wip-limit", "In Progress", "3")

	if !strings.Contains(out, "Set WIP limit") {
		t.Errorf("expected set confirmation, got: %s", out)
	}
}

func TestColumnsWIPLimitClearHuman(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "wip-limit", "In Progress", "0")

	if !strings.Contains(out, "Cleared WIP limit") {
		t.Errorf("expected clear confirmation, got: %s", out)
	}
}

// --- Error cases ---

func TestCardNotFoundError(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "cards", "show", "nonexistent-id-1234")
	if err == nil {
		t.Error("expected error for nonexistent card")
	}
}

func TestCardInvalidPriority(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "cards", "add", "Bad priority", "-p", "critical")
	if err == nil {
		t.Error("expected error for invalid priority")
	}
}

func TestColumnNotFoundOnAdd(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "cards", "add", "Test", "-c", "Nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent column")
	}
}

func TestColumnDeleteNotFound(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "columns", "delete", "Nonexistent", "-f")
	if err == nil {
		t.Error("expected error for nonexistent column")
	}
}

func TestWIPLimitInvalidNumber(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "columns", "wip-limit", "Todo", "abc")
	if err == nil {
		t.Error("expected error for non-numeric limit")
	}
}

func TestWIPLimitNegativeNumber(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	_, err := executeCmdErr(t, "columns", "wip-limit", "Todo", "-1")
	if err == nil {
		t.Error("expected error for negative limit")
	}
}

func TestNoBoardDetected(t *testing.T) {
	setupTestDB(t)
	os.Unsetenv("KB_BOARD")
	os.Unsetenv("TMUX_SESSION_NAME")

	_, err := executeCmdErr(t, "cards")
	if err == nil {
		t.Error("expected error when no board can be detected")
	}
}

// --- Edit with all flags ---

func TestCardsEditAllFields(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Original", "low")

	out := executeCmd(t, "cards", "edit", card.ID[:8],
		"-t", "New title",
		"-d", "New desc",
		"-l", "label1,label2",
		"-p", "urgent",
		"-e", "GH-42",
		"--json")

	var result cardJSON
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Title != "New title" {
		t.Errorf("title = %q, want 'New title'", result.Title)
	}
	if result.Description != "New desc" {
		t.Errorf("description = %q, want 'New desc'", result.Description)
	}
	if result.Labels != "label1,label2" {
		t.Errorf("labels = %q, want 'label1,label2'", result.Labels)
	}
	if result.Priority != "urgent" {
		t.Errorf("priority = %q, want 'urgent'", result.Priority)
	}
	if result.ExternalID != "GH-42" {
		t.Errorf("external_id = %q, want 'GH-42'", result.ExternalID)
	}
}

// --- Cards add defaults to first column ---

func TestCardsAddDefaultsToFirstColumn(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "cards", "add", "Default column card", "--json")

	var card cardJSON
	json.Unmarshal([]byte(out), &card)
	if card.Column != "Backlog" {
		t.Errorf("expected default column 'Backlog', got %q", card.Column)
	}
}

// --- Card ID prefix resolution ---

func TestCardIDFullMatch(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")
	columns, _ := db.ListColumns(mustBoardID(t, "test-board"))
	card, _ := db.CreateCard(columns[0].ID, "Full ID test", "medium")

	out := executeCmd(t, "cards", "show", card.ID, "--json")

	var result cardJSON
	json.Unmarshal([]byte(out), &result)
	if result.ID != card.ID {
		t.Errorf("expected full ID match, got %q", result.ID)
	}
}

// --- Board delete skips confirmation in JSON mode ---

func TestBoardDeleteJSONSkipsConfirmation(t *testing.T) {
	setupTestDB(t)

	createTestBoard(t, "auto-delete")

	out := executeCmd(t, "boards", "delete", "auto-delete", "--json")

	var board boardJSON
	if err := json.Unmarshal([]byte(out), &board); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if board.Name != "auto-delete" {
		t.Errorf("expected name 'auto-delete', got %q", board.Name)
	}
}

// --- Column delete skips confirmation in JSON mode ---

func TestColumnDeleteJSONSkipsConfirmation(t *testing.T) {
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	defer os.Unsetenv("KB_BOARD")

	createTestBoard(t, "test-board")

	out := executeCmd(t, "columns", "delete", "Review", "--json")

	var col columnJSON
	if err := json.Unmarshal([]byte(out), &col); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if col.Name != "Review" {
		t.Errorf("expected name 'Review', got %q", col.Name)
	}
}

// --- Card filter tests ---

func setupFilteredCards(t *testing.T) {
	t.Helper()
	setupTestDB(t)
	os.Setenv("KB_BOARD", "test-board")
	t.Cleanup(func() { os.Unsetenv("KB_BOARD") })

	createTestBoard(t, "test-board")
	boardID := mustBoardID(t, "test-board")
	columns, _ := db.ListColumns(boardID)

	c1, _ := db.CreateCard(columns[0].ID, "Auth login fix", model.PriorityHigh)
	c1.Labels = "bug,backend"
	c1.Description = "Fix the OAuth flow"
	db.UpdateCard(c1)

	c2, _ := db.CreateCard(columns[1].ID, "Dashboard redesign", model.PriorityMedium)
	c2.Labels = "frontend"
	c2.Description = "Update layout with auth token display"
	db.UpdateCard(c2)

	c3, _ := db.CreateCard(columns[1].ID, "Urgent hotfix", model.PriorityUrgent)
	c3.Labels = "bug"
	db.UpdateCard(c3)

	db.CreateCard(columns[2].ID, "Write tests", model.PriorityLow)
}

func TestCardsFilterByPriority(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-p", "high")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Priority != "high" {
		t.Errorf("expected priority 'high', got %q", cards[0].Priority)
	}
}

func TestCardsFilterByLabel(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-l", "bug")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards with 'bug' label, got %d", len(cards))
	}
}

func TestCardsFilterByColumn(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-c", "Todo")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards in Todo, got %d", len(cards))
	}
	for _, c := range cards {
		if c.Column != "Todo" {
			t.Errorf("expected column 'Todo', got %q", c.Column)
		}
	}
}

func TestCardsFilterBySearch(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-s", "auth")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards matching 'auth' (title + description), got %d", len(cards))
	}
}

func TestCardsFilterCombined(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-p", "urgent", "-c", "Todo")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 card (urgent + Todo), got %d", len(cards))
	}
	if cards[0].Title != "Urgent hotfix" {
		t.Errorf("expected 'Urgent hotfix', got %q", cards[0].Title)
	}
}

func TestCardsFilterNoResults(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-p", "urgent", "-c", "Backlog")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 0 {
		t.Fatalf("expected 0 cards, got %d", len(cards))
	}
}

func TestCardsFilterInvalidPriority(t *testing.T) {
	setupFilteredCards(t)

	_, err := executeCmdErr(t, "cards", "-p", "bogus")
	if err == nil {
		t.Error("expected error for invalid priority filter")
	}
}

func TestCardsFilterHumanOutput(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "-p", "high")

	if !strings.Contains(out, "Auth login fix") {
		t.Errorf("expected 'Auth login fix' in output: %s", out)
	}
	if strings.Contains(out, "Dashboard redesign") {
		t.Errorf("should not contain 'Dashboard redesign' in filtered output: %s", out)
	}
}

func TestCardsFilterSearchDescription(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-s", "OAuth")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 1 {
		t.Fatalf("expected 1 card matching description 'OAuth', got %d", len(cards))
	}
	if cards[0].Title != "Auth login fix" {
		t.Errorf("expected 'Auth login fix', got %q", cards[0].Title)
	}
}

func TestCardsFilterLabelExact(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-l", "bu")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 0 {
		t.Fatalf("'bu' should not match 'bug' (exact label match), got %d cards", len(cards))
	}
}

func TestCardsFilterCaseInsensitive(t *testing.T) {
	setupFilteredCards(t)

	out := executeCmd(t, "cards", "--json", "-c", "todo")

	var cards []cardJSON
	if err := json.Unmarshal([]byte(out), &cards); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(cards) != 2 {
		t.Fatalf("expected 2 cards in 'todo' (case-insensitive), got %d", len(cards))
	}

	out2 := executeCmd(t, "cards", "--json", "-s", "AUTH")

	var cards2 []cardJSON
	if err := json.Unmarshal([]byte(out2), &cards2); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out2)
	}
	if len(cards2) != 2 {
		t.Fatalf("expected 2 cards matching 'AUTH' (case-insensitive search), got %d", len(cards2))
	}
}

// --- Note tests ---

func TestNoteCreateJSON(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "notes", "create", "My First Note", "--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Title != "My First Note" {
		t.Errorf("title = %q, want 'My First Note'", note.Title)
	}
	if note.Slug != "my-first-note" {
		t.Errorf("slug = %q, want 'my-first-note'", note.Slug)
	}
}

func TestNoteCreateWithFlags(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "notes", "create", "Tagged Note",
		"--tags", "go,pkm",
		"--body", "Some content with [[wikilinks]]",
		"--slug", "custom-slug",
		"--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Tags != "go,pkm" {
		t.Errorf("tags = %q, want 'go,pkm'", note.Tags)
	}
	if note.Slug != "custom-slug" {
		t.Errorf("slug = %q, want 'custom-slug'", note.Slug)
	}
	if note.Body != "Some content with [[wikilinks]]" {
		t.Errorf("body = %q", note.Body)
	}
}

func TestNoteListJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Note A", "--json")
	executeCmd(t, "notes", "create", "Note B", "--json")

	out := executeCmd(t, "notes", "--json")

	var notes []noteJSON
	if err := json.Unmarshal([]byte(out), &notes); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(notes))
	}
}

func TestNoteListByTag(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Go Note", "--tags", "go,tools", "--json")
	executeCmd(t, "notes", "create", "Rust Note", "--tags", "rust", "--json")

	out := executeCmd(t, "notes", "--tag", "go", "--json")

	var notes []noteJSON
	if err := json.Unmarshal([]byte(out), &notes); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note with tag 'go', got %d", len(notes))
	}
}

func TestNoteListBySearch(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Go Patterns", "--body", "Concurrency patterns", "--json")
	executeCmd(t, "notes", "create", "Rust Guide", "--body", "Memory safety", "--json")

	out := executeCmd(t, "notes", "--search", "concurrency", "--json")

	var notes []noteJSON
	if err := json.Unmarshal([]byte(out), &notes); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note matching 'concurrency', got %d", len(notes))
	}
}

func TestNoteShowJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Show Me", "--body", "details here", "--json")

	out := executeCmd(t, "notes", "show", "show-me", "--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Title != "Show Me" {
		t.Errorf("title = %q, want 'Show Me'", note.Title)
	}
	if note.Body != "details here" {
		t.Errorf("body = %q, want 'details here'", note.Body)
	}
}

func TestNoteEditJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Original Title", "--json")

	out := executeCmd(t, "notes", "edit", "original-title",
		"--title", "Updated Title",
		"--tags", "updated",
		"--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Title != "Updated Title" {
		t.Errorf("title = %q, want 'Updated Title'", note.Title)
	}
	if note.Tags != "updated" {
		t.Errorf("tags = %q, want 'updated'", note.Tags)
	}
}

func TestNoteEditPartial(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Keep Title", "--tags", "original", "--json")

	out := executeCmd(t, "notes", "edit", "keep-title",
		"--tags", "changed",
		"--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Title != "Keep Title" {
		t.Errorf("title should be unchanged, got %q", note.Title)
	}
	if note.Tags != "changed" {
		t.Errorf("tags = %q, want 'changed'", note.Tags)
	}
}

func TestNoteDeleteJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Delete Me", "--json")

	out := executeCmd(t, "notes", "delete", "delete-me", "--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.Title != "Delete Me" {
		t.Errorf("title = %q, want 'Delete Me'", note.Title)
	}
}

func TestNoteBacklinksJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Target Note", "--json")
	executeCmd(t, "notes", "create", "Source Note",
		"--body", "Links to [[target-note]] for info", "--json")

	out := executeCmd(t, "notes", "backlinks", "target-note", "--json")

	var links []backlinkJSON
	if err := json.Unmarshal([]byte(out), &links); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 backlink, got %d", len(links))
	}
}

func TestNoteCreateHuman(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "notes", "create", "My Note")

	if !strings.Contains(out, "Created note") || !strings.Contains(out, "my-note") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestNoteListEmpty(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "notes")

	if !strings.Contains(out, "No notes") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestNoteShowByIDPrefix(t *testing.T) {
	setupTestDB(t)

	createOut := executeCmd(t, "notes", "create", "ID Prefix Test", "--json")
	var created noteJSON
	json.Unmarshal([]byte(createOut), &created)

	out := executeCmd(t, "notes", "show", created.ID[:8], "--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.ID != created.ID {
		t.Errorf("expected full ID match via prefix")
	}
}

func TestNoteShowNotFound(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "notes", "show", "nonexistent-slug")
	if err == nil {
		t.Error("expected error for nonexistent note")
	}
}

// --- Workspace tests ---

func TestWorkspaceCreateJSON(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "workspace", "create", "My Project", "--kind", "project", "--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Name != "My Project" {
		t.Errorf("name = %q, want 'My Project'", ws.Name)
	}
	if ws.Kind != "project" {
		t.Errorf("kind = %q, want 'project'", ws.Kind)
	}
}

func TestWorkspaceCreateHuman(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "workspace", "create", "Test Area", "--kind", "area")

	if !strings.Contains(out, "Created workspace") || !strings.Contains(out, "Test Area") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
}

func TestWorkspaceCreateWithFlags(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "workspace", "create", "Dev Project",
		"--kind", "project",
		"--description", "A dev project",
		"--path", "/tmp/dev",
		"--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Description != "A dev project" {
		t.Errorf("description = %q, want 'A dev project'", ws.Description)
	}
	if ws.Path != "/tmp/dev" {
		t.Errorf("path = %q, want '/tmp/dev'", ws.Path)
	}
}

func TestWorkspaceListJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Alpha", "--kind", "project", "--json")
	executeCmd(t, "workspace", "create", "Beta", "--kind", "area", "--json")

	out := executeCmd(t, "workspace", "--json")

	var workspaces []workspaceJSON
	if err := json.Unmarshal([]byte(out), &workspaces); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}
}

func TestWorkspaceListEmpty(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "workspace")

	if !strings.Contains(out, "No workspaces") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestWorkspaceListByKind(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "P1", "--kind", "project", "--json")
	executeCmd(t, "workspace", "create", "A1", "--kind", "area", "--json")
	executeCmd(t, "workspace", "create", "P2", "--kind", "project", "--json")

	out := executeCmd(t, "workspace", "--kind", "project", "--json")

	var workspaces []workspaceJSON
	if err := json.Unmarshal([]byte(out), &workspaces); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 project workspaces, got %d", len(workspaces))
	}
}

func TestWorkspaceShowJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Show Me", "--kind", "resource", "--description", "A resource", "--json")

	out := executeCmd(t, "workspace", "show", "Show Me", "--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Name != "Show Me" {
		t.Errorf("name = %q, want 'Show Me'", ws.Name)
	}
	if ws.Kind != "resource" {
		t.Errorf("kind = %q, want 'resource'", ws.Kind)
	}
}

func TestWorkspaceShowHuman(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "My Workspace", "--kind", "project", "--json")

	out := executeCmd(t, "workspace", "show", "My Workspace")

	if !strings.Contains(out, "My Workspace") {
		t.Errorf("expected name in output: %s", out)
	}
	if !strings.Contains(out, "[P]") {
		t.Errorf("expected kind label in output: %s", out)
	}
}

func TestWorkspaceEditJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Original", "--kind", "project", "--json")

	out := executeCmd(t, "workspace", "edit", "Original",
		"--name", "Updated",
		"--description", "new desc",
		"--kind", "area",
		"--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Name != "Updated" {
		t.Errorf("name = %q, want 'Updated'", ws.Name)
	}
	if ws.Kind != "area" {
		t.Errorf("kind = %q, want 'area'", ws.Kind)
	}
	if ws.Description != "new desc" {
		t.Errorf("description = %q, want 'new desc'", ws.Description)
	}
}

func TestWorkspaceArchiveJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "To Archive", "--kind", "project", "--json")

	out := executeCmd(t, "workspace", "archive", "To Archive", "--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Kind != "archive" {
		t.Errorf("kind = %q, want 'archive'", ws.Kind)
	}
}

func TestWorkspaceDeleteJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "To Delete", "--kind", "project", "--json")

	out := executeCmd(t, "workspace", "delete", "To Delete", "--json")

	var ws workspaceJSON
	if err := json.Unmarshal([]byte(out), &ws); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if ws.Name != "To Delete" {
		t.Errorf("name = %q, want 'To Delete'", ws.Name)
	}
}

func TestWorkspaceNotFound(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "workspace", "show", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent workspace")
	}
}

func TestWorkspaceInvalidKind(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "workspace", "create", "Bad Kind", "--kind", "invalid")
	if err == nil {
		t.Error("expected error for invalid kind")
	}
}

func TestBoardMoveToWorkspaceJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Dev WS", "--kind", "project", "--json")
	createTestBoard(t, "my-board")

	out := executeCmd(t, "boards", "move", "my-board", "--workspace", "Dev WS", "--json")

	var board boardJSON
	if err := json.Unmarshal([]byte(out), &board); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if board.WorkspaceID == nil {
		t.Fatal("expected WorkspaceID to be set")
	}
}

func TestBoardMoveHuman(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "WS", "--kind", "project", "--json")
	createTestBoard(t, "my-board")

	out := executeCmd(t, "boards", "move", "my-board", "--workspace", "WS")

	if !strings.Contains(out, "Moved board") {
		t.Errorf("expected move confirmation, got: %s", out)
	}
}

func TestBoardMoveUnassign(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "WS", "--kind", "project", "--json")
	createTestBoard(t, "my-board")
	executeCmd(t, "boards", "move", "my-board", "--workspace", "WS", "--json")

	out := executeCmd(t, "boards", "move", "my-board")

	if !strings.Contains(out, "Removed board") {
		t.Errorf("expected unassign confirmation, got: %s", out)
	}
}

func TestNoteMoveToWorkspaceJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Notes WS", "--kind", "area", "--json")
	executeCmd(t, "notes", "create", "My Note", "--json")

	out := executeCmd(t, "notes", "move", "my-note", "--workspace", "Notes WS", "--json")

	var note noteJSON
	if err := json.Unmarshal([]byte(out), &note); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if note.WorkspaceID == nil {
		t.Fatal("expected WorkspaceID to be set")
	}
}

func TestNoteMoveHuman(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "WS", "--kind", "project", "--json")
	executeCmd(t, "notes", "create", "My Note", "--json")

	out := executeCmd(t, "notes", "move", "my-note", "--workspace", "WS")

	if !strings.Contains(out, "Moved note") {
		t.Errorf("expected move confirmation, got: %s", out)
	}
}

func TestNoteMoveUnassign(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "WS", "--kind", "project", "--json")
	executeCmd(t, "notes", "create", "My Note", "--json")
	executeCmd(t, "notes", "move", "my-note", "--workspace", "WS", "--json")

	out := executeCmd(t, "notes", "move", "my-note")

	if !strings.Contains(out, "Removed note") {
		t.Errorf("expected unassign confirmation, got: %s", out)
	}
}

func TestWorkspaceShowWithContents(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "Full WS", "--kind", "project", "--json")
	createTestBoard(t, "ws-board")
	executeCmd(t, "boards", "move", "ws-board", "--workspace", "Full WS", "--json")
	executeCmd(t, "notes", "create", "WS Note", "--json")
	executeCmd(t, "notes", "move", "ws-note", "--workspace", "Full WS", "--json")

	out := executeCmd(t, "workspace", "show", "Full WS")

	if !strings.Contains(out, "ws-board") {
		t.Errorf("expected board name in show output: %s", out)
	}
	if !strings.Contains(out, "ws-note") {
		t.Errorf("expected note slug in show output: %s", out)
	}
}

func TestWorkspaceAlias(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "ws", "create", "Alias Test", "--kind", "project", "--json")

	out := executeCmd(t, "ws", "--json")

	var workspaces []workspaceJSON
	if err := json.Unmarshal([]byte(out), &workspaces); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace via alias, got %d", len(workspaces))
	}
}

// --- Publish tests ---

func TestPublishSetupJSON(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()

	out := executeCmd(t, "publish", "setup", "my-site",
		"--engine", "jekyll",
		"--path", tmpDir,
		"--json")

	var pt publishTargetJSON
	if err := json.Unmarshal([]byte(out), &pt); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if pt.Name != "my-site" {
		t.Errorf("name = %q, want 'my-site'", pt.Name)
	}
	if pt.Engine != "jekyll" {
		t.Errorf("engine = %q, want 'jekyll'", pt.Engine)
	}
	if pt.BasePath != tmpDir {
		t.Errorf("base_path = %q, want %q", pt.BasePath, tmpDir)
	}
	if pt.PostsDir != "_posts" {
		t.Errorf("posts_dir = %q, want '_posts'", pt.PostsDir)
	}
}

func TestPublishSetupHuman(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "publish", "setup", "blog",
		"--engine", "jekyll",
		"--path", t.TempDir())

	if !strings.Contains(out, "Created publish target") {
		t.Errorf("expected confirmation, got: %s", out)
	}
}

func TestPublishSetupWithWorkspace(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "workspace", "create", "WS", "--kind", "project", "--json")

	out := executeCmd(t, "publish", "setup", "blog",
		"--engine", "jekyll",
		"--path", t.TempDir(),
		"--workspace", "WS",
		"--json")

	var pt publishTargetJSON
	if err := json.Unmarshal([]byte(out), &pt); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if pt.WorkspaceID == nil {
		t.Fatal("expected WorkspaceID to be set")
	}
}

func TestPublishSetupMissingPath(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "publish", "setup", "blog", "--engine", "jekyll")
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestPublishSetupInvalidEngine(t *testing.T) {
	setupTestDB(t)

	_, err := executeCmdErr(t, "publish", "setup", "blog", "--engine", "hugo", "--path", "/tmp")
	if err == nil {
		t.Error("expected error for invalid engine")
	}
}

func TestPublishListTargetsJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "publish", "setup", "site-a", "--engine", "jekyll", "--path", t.TempDir(), "--json")
	executeCmd(t, "publish", "setup", "site-b", "--engine", "jekyll", "--path", t.TempDir(), "--json")

	out := executeCmd(t, "publish", "list", "--json")

	var targets []publishTargetJSON
	if err := json.Unmarshal([]byte(out), &targets); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
}

func TestPublishListEmpty(t *testing.T) {
	setupTestDB(t)

	out := executeCmd(t, "publish", "list")

	if !strings.Contains(out, "No publish targets") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestPublishDeleteJSON(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "publish", "setup", "to-delete", "--engine", "jekyll", "--path", t.TempDir(), "--json")

	out := executeCmd(t, "publish", "delete", "to-delete", "--json")

	var pt publishTargetJSON
	if err := json.Unmarshal([]byte(out), &pt); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if pt.Name != "to-delete" {
		t.Errorf("name = %q, want 'to-delete'", pt.Name)
	}
}

func TestPublishNoteJSON(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "My Blog Post", "--body", "Hello world", "--tags", "go,blog", "--json")

	out := executeCmd(t, "publish", "my-blog-post", "--json")

	var pl publishLogJSON
	if err := json.Unmarshal([]byte(out), &pl); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if pl.NoteSlug != "my-blog-post" {
		t.Errorf("note_slug = %q, want 'my-blog-post'", pl.NoteSlug)
	}
	if !strings.Contains(pl.FilePath, "my-blog-post.md") {
		t.Errorf("file_path = %q, want to contain 'my-blog-post.md'", pl.FilePath)
	}

	writtenPath := filepath.Join(tmpDir, pl.FilePath)
	content, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("reading published file: %v", err)
	}
	if !strings.Contains(string(content), "Hello world") {
		t.Errorf("published content missing body: %s", content)
	}
	if !strings.Contains(string(content), `title: "My Blog Post"`) {
		t.Errorf("published content missing title front matter: %s", content)
	}
	if !strings.Contains(string(content), "tags: [go, blog]") {
		t.Errorf("published content missing tags: %s", content)
	}
}

func TestPublishNoteHuman(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "Human Post", "--body", "Content", "--json")

	out := executeCmd(t, "publish", "human-post")

	if !strings.Contains(out, "Published") || !strings.Contains(out, "Human Post") {
		t.Errorf("expected publish confirmation, got: %s", out)
	}
}

func TestPublishNoteDraft(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "Draft Post", "--body", "Draft content", "--json")

	out := executeCmd(t, "publish", "draft-post", "--draft")

	if !strings.Contains(out, "draft") {
		t.Errorf("expected draft label, got: %s", out)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir, "_posts", "*.md"))
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	if !strings.Contains(string(content), "published: false") {
		t.Errorf("expected 'published: false' in draft: %s", content)
	}
}

func TestPublishNoteDryRun(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "Dry Run Post", "--body", "Preview", "--json")

	out := executeCmd(t, "publish", "dry-run-post", "--dry-run")

	if !strings.Contains(out, "Would write to:") {
		t.Errorf("expected dry run message, got: %s", out)
	}
	if !strings.Contains(out, "Preview") {
		t.Errorf("expected body content in dry run, got: %s", out)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir, "_posts", "*.md"))
	if len(files) != 0 {
		t.Errorf("dry run should not write files, found %d", len(files))
	}
}

func TestPublishNoteDryRunJSON(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "Dry JSON", "--body", "Preview content", "--json")

	out := executeCmd(t, "publish", "dry-json", "--dry-run", "--json")

	var result struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
		Draft    bool   `json:"draft"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if !strings.Contains(result.Content, "Preview content") {
		t.Errorf("expected body in content: %s", result.Content)
	}
	if result.Draft {
		t.Error("expected draft=false for non-draft dry run")
	}
}

func TestPublishListLogsJSON(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")
	executeCmd(t, "notes", "create", "Log Test", "--body", "content", "--json")
	executeCmd(t, "publish", "log-test", "--json")

	out := executeCmd(t, "publish", "list", "--target", "site", "--json")

	var logs []publishLogJSON
	if err := json.Unmarshal([]byte(out), &logs); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].NoteSlug != "log-test" {
		t.Errorf("note_slug = %q, want 'log-test'", logs[0].NoteSlug)
	}
}

func TestPublishNoteResolvesWikilinks(t *testing.T) {
	setupTestDB(t)

	tmpDir := t.TempDir()
	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", tmpDir, "--json")

	executeCmd(t, "notes", "create", "Referenced", "--body", "I am referenced", "--json")
	executeCmd(t, "notes", "create", "Linker", "--body", "See [[referenced]] for info", "--json")

	executeCmd(t, "publish", "referenced", "--json")
	executeCmd(t, "publish", "linker", "--json")

	files, _ := filepath.Glob(filepath.Join(tmpDir, "_posts", "*linker*"))
	if len(files) != 1 {
		t.Fatalf("expected 1 linker file, got %d", len(files))
	}
	content, _ := os.ReadFile(files[0])
	if !strings.Contains(string(content), "[Referenced]") {
		t.Errorf("expected resolved wikilink in content: %s", content)
	}
}

func TestPublishNoteNotFound(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "publish", "setup", "site", "--engine", "jekyll", "--path", t.TempDir(), "--json")

	_, err := executeCmdErr(t, "publish", "nonexistent-note")
	if err == nil {
		t.Error("expected error for nonexistent note")
	}
}

func TestPublishNoTargets(t *testing.T) {
	setupTestDB(t)

	executeCmd(t, "notes", "create", "Orphan", "--json")

	_, err := executeCmdErr(t, "publish", "orphan")
	if err == nil {
		t.Error("expected error when no publish targets configured")
	}
}

func TestPublishWithTargetFlag(t *testing.T) {
	setupTestDB(t)

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	executeCmd(t, "publish", "setup", "site-1", "--engine", "jekyll", "--path", tmpDir1, "--json")
	executeCmd(t, "publish", "setup", "site-2", "--engine", "jekyll", "--path", tmpDir2, "--json")
	executeCmd(t, "notes", "create", "Multi Target", "--body", "content", "--json")

	out := executeCmd(t, "publish", "multi-target", "--target", "site-2", "--json")

	var pl publishLogJSON
	if err := json.Unmarshal([]byte(out), &pl); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}

	files, _ := filepath.Glob(filepath.Join(tmpDir2, "_posts", "*multi-target*"))
	if len(files) != 1 {
		t.Fatalf("expected file in site-2, got %d", len(files))
	}

	files1, _ := filepath.Glob(filepath.Join(tmpDir1, "_posts", "*multi-target*"))
	if len(files1) != 0 {
		t.Errorf("should not have file in site-1")
	}
}
