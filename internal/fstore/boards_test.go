package fstore

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func testBoard(t *testing.T, s *Store) *model.Board {
	t.Helper()
	b, err := s.CreateBoard("Sprint", "the sprint board", "")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func cardTitles(t *testing.T, s *Store, boardID, lane string) string {
	t.Helper()
	cards, err := s.Cards(boardID, lane)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range cards {
		out = append(out, c.Title)
	}
	return strings.Join(out, ",")
}

func TestCreateBoardWritesAKanbanFile(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	if b.ID != "Boards/Sprint.md" || b.Name != "Sprint" || b.Description != "the sprint board" {
		t.Errorf("board = %+v", b)
	}
	content := read(t, s, "Boards/Sprint.md")
	for _, want := range []string{"kanban-plugin: board", "description: the sprint board", "## Backlog", "## Done", "%% kanban:settings"} {
		if !strings.Contains(content, want) {
			t.Errorf("file lacks %q:\n%s", want, content)
		}
	}
	lanes, _ := s.Lanes(b.ID)
	if len(lanes) != 5 || lanes[0].Name != "Backlog" {
		t.Errorf("lanes = %+v", lanes)
	}
	if len(s.ListNotes()) != 0 {
		t.Error("a board is not a note")
	}
	if _, err := s.CreateBoard("sprint", "", ""); err == nil {
		t.Error("a second board with the same name should be refused")
	}
	if got, err := s.GetBoard("sprint"); err != nil || got.ID != b.ID {
		t.Errorf("GetBoard by name: %+v, %v", got, err)
	}
}

func TestCardsAddEditMoveArchiveDelete(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	c, err := s.AddCard(b.ID, "Todo", "Fix login", CardFields{Priority: "high", Labels: []string{"some label", "ui"}, Description: "steps\nmore"}, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCard(b.ID, c.ID)
	if err != nil || got.Title != "Fix login" || got.Priority != model.PriorityHigh || got.Labels != "some-label,ui" || got.Description != "steps\nmore" || got.ColumnID != "Todo" {
		t.Fatalf("card = %+v, %v", got, err)
	}
	if !strings.Contains(read(t, s, "Boards/Sprint.md"), "- [ ] Fix login #some-label #ui #priority/high ^"+c.ID) {
		t.Errorf("card line:\n%s", read(t, s, "Boards/Sprint.md"))
	}

	got.Title = "Fix login on mobile"
	got.Priority = model.PriorityUrgent
	if err := s.UpdateCard(got); err != nil {
		t.Fatal(err)
	}
	s.AddCard(b.ID, "Doing", "Already here", CardFields{}, false)
	if err := s.MoveCard(b.ID, c.ID, "In Progress", "", false); err != nil {
		t.Fatal(err)
	}
	other, _ := s.AddCard(b.ID, "In Progress", "Second", CardFields{}, false)
	if err := s.MoveCard(b.ID, other.ID, "In Progress", c.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := cardTitles(t, s, b.ID, "In Progress"); got != "Second,Fix login on mobile" {
		t.Errorf("In Progress = %s", got)
	}
	if err := s.ArchiveCard(b.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if got := cardTitles(t, s, b.ID, "In Progress"); got != "Fix login on mobile" {
		t.Errorf("after archive = %s", got)
	}
	if arch, _ := s.ArchivedCards(b.ID); len(arch) != 1 || arch[0].Title != "Second" {
		t.Errorf("archived = %+v", arch)
	}
	if err := s.DeleteCard(b.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetCard(b.ID, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted card still found: %v", err)
	}
}

func TestMovingIntoAFullLaneIsRefusedUnlessForced(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	if err := s.SetWIPLimit(b.ID, "In Progress", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, s, "Boards/Sprint.md"), "## In Progress (1)") {
		t.Error("the limit is written in the plugin's lane-title form")
	}
	a, _ := s.AddCard(b.ID, "Todo", "A", CardFields{}, false)
	c, _ := s.AddCard(b.ID, "Todo", "B", CardFields{}, false)
	s.MoveCard(b.ID, a.ID, "In Progress", "", false)
	var wip *WIPLimitError
	if err := s.MoveCard(b.ID, c.ID, "In Progress", "", false); !errors.As(err, &wip) || wip.Limit != 1 {
		t.Errorf("err = %v, want a WIP limit error", err)
	}
	if _, err := s.AddCard(b.ID, "In Progress", "C", CardFields{}, false); !errors.As(err, &wip) {
		t.Errorf("adding into a full lane: %v", err)
	}
	if err := s.MoveCard(b.ID, c.ID, "In Progress", "", true); err != nil {
		t.Errorf("forced move: %v", err)
	}
	// Reordering within the full lane is not adding to it.
	if err := s.MoveCard(b.ID, c.ID, "In Progress", a.ID, false); err != nil {
		t.Errorf("reordering inside the lane: %v", err)
	}
}

func TestAStaleCardEditIsRefused(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	c, _ := s.AddCard(b.ID, "Todo", "Original", CardFields{}, false)
	other := sibling(t, s)
	edit, _ := other.GetCard(b.ID, c.ID)
	edit.Title = "Changed elsewhere"
	if err := other.UpdateCard(edit); err != nil {
		t.Fatal(err)
	}
	c.Title = "Stale"
	if err := s.UpdateCard(c); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	// Moving the card is not an edit of its text and applies to the file as
	// it is now.
	if err := s.MoveCard(b.ID, c.ID, "Done", "", false); err != nil {
		t.Errorf("move after an outside edit: %v", err)
	}
	if got, _ := s.GetCard(b.ID, c.ID); got.Title != "Changed elsewhere" || got.ColumnID != "Done" {
		t.Errorf("card = %+v", got)
	}
}

func TestManyWritersLoseNoCards(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	var stores []*Store
	for i := 0; i < 4; i++ {
		stores = append(stores, sibling(t, s))
	}
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := stores[i%4].AddCard(b.ID, "Backlog", fmt.Sprintf("card %02d", i), CardFields{}, false)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	reload(t, s)
	cards, _ := s.Cards(b.ID, "Backlog")
	if len(cards) != 40 {
		t.Errorf("%d cards survived 40 concurrent adds", len(cards))
	}
}

func TestLanes(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	if err := s.AddLane(b.ID, "QA"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLane(b.ID, "qa"); err == nil {
		t.Error("a duplicate lane name should be refused")
	}
	if err := s.AddLane(b.ID, "Q3 (2024)"); err == nil {
		t.Error(`a lane name ending in "(N)" should be refused`)
	}
	c, _ := s.AddCard(b.ID, "QA", "Under test", CardFields{}, false)
	if err := s.DeleteLane(b.ID, "QA", false); err == nil {
		t.Error("deleting a lane with cards needs force")
	}
	if err := s.DeleteLane(b.ID, "QA", true); err != nil {
		t.Fatal(err)
	}
	if arch, _ := s.ArchivedCards(b.ID); len(arch) != 1 || arch[0].ID != c.ID {
		t.Errorf("a forced lane delete archives its cards: %+v", arch)
	}
	if err := s.ReorderLanes(b.ID, []string{"Done", "Review", "In Progress", "Todo", "Backlog"}); err != nil {
		t.Fatal(err)
	}
	lanes, _ := s.Lanes(b.ID)
	if lanes[0].Name != "Done" || lanes[4].Name != "Backlog" {
		t.Errorf("order = %+v", lanes)
	}
	if err := s.ReorderLanes(b.ID, []string{"Done", "Todo"}); err == nil {
		t.Error("a partial reorder should be refused")
	}
	if err := s.RenameLane(b.ID, "Todo", "To do"); err != nil {
		t.Fatal(err)
	}
	if got := cardTitles(t, s, b.ID, "To do"); got != "" {
		t.Errorf("renamed lane = %q", got)
	}
}

func TestCardDescriptionsLinkToNotes(t *testing.T) {
	s := testStore(t)
	b := testBoard(t, s)
	s.CreateNote("API Spec", "", "", "")
	c, _ := s.AddCard(b.ID, "Todo", "Build endpoint", CardFields{Description: "per [[api spec#auth]]"}, false)
	if got := backlinkNames(t, s, "api-spec"); got != "card:Build endpoint" {
		t.Errorf("backlinks = %q", got)
	}
	s.ArchiveCard(b.ID, c.ID)
	if got := backlinkNames(t, s, "api-spec"); got != "" {
		t.Errorf("an archived card is not a backlink: %q", got)
	}
}

func TestWorkspacesLiveInTheVault(t *testing.T) {
	s := testStore(t)
	ws, err := s.CreateWorkspace("School", model.KindArea, "DVS", "/Users/me/school")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, s, ".kb/workspaces.yml"), "name: School") {
		t.Error("workspace not written to the vault")
	}
	if strings.Contains(read(t, s, ".kb/workspaces.yml"), "/Users/me") {
		t.Error("a machine path must not go into the synced vault")
	}
	if got, _ := openAt(t, s.Vault().Root()).GetWorkspaceByName("school"); got != nil && got.ID != ws.ID {
		t.Error("workspace ids must be stable across loads")
	}
	n, _ := s.CreateNote("Lecture", "", "", ws.ID)
	b, _ := s.CreateBoard("Course", "", ws.ID)
	renamed := *ws
	renamed.Name = "University"
	if err := s.UpdateWorkspace(&renamed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, s, n.Path), "workspace: University") || !strings.Contains(read(t, s, b.ID), "workspace: University") {
		t.Error("a rename must reach the notes and boards that name the workspace")
	}
	if err := s.DeleteWorkspace(ws.ID); err == nil || !strings.Contains(err.Error(), "Lecture.md") {
		t.Errorf("deleting a workspace in use should name its users: %v", err)
	}
	def := s.DefaultWorkspace()
	gone := *def
	gone.Name = "Personal"
	if err := s.UpdateWorkspace(&gone); err == nil {
		t.Error("Default cannot be renamed")
	}
	if err := s.DeleteWorkspace(def.ID); err == nil {
		t.Error("Default cannot be deleted")
	}
}

// Card ids are unique within a board, not across boards: an id (or
// prefix) on two boards is an error naming both, and a board narrows it.
func TestFindCardAcrossBoards(t *testing.T) {
	s := testStore(t)
	board := "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] %s ^abcd1234\n"
	put(t, s, "Boards/One.md", fmt.Sprintf(board, "first"))
	put(t, s, "Boards/Two.md", fmt.Sprintf(board, "second"))
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := s.FindCard("", "abcd")
	if err == nil || !strings.Contains(err.Error(), "One") || !strings.Contains(err.Error(), "Two") {
		t.Errorf("err = %v, want it to name both boards", err)
	}
	c, err := s.FindCard("two", "abcd")
	if err != nil || c.Title != "second" {
		t.Errorf("on board two: %v, %v", c, err)
	}
	if _, err := s.FindCard("", "abc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a 3-character prefix: err = %v, want not found", err)
	}
}
