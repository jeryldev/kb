package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
)

// boardApp opens a board with the given cards in its first lane
// ("Backlog"), at a usual terminal size.
func boardApp(t *testing.T, titles ...string) (*App, *fstore.Store) {
	t.Helper()
	db := testStore(t)
	board, err := db.CreateBoard("work", "", db.DefaultWorkspace().ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range titles {
		if _, err := db.AddCard(board.ID, "Backlog", title, fstore.CardFields{}, false); err != nil {
			t.Fatal(err)
		}
	}
	app := NewApp(db, "work")
	app.width, app.height = 120, 30
	app.Init()
	if app.mode != modeBoard {
		t.Fatalf("mode %d, err %v", app.mode, app.err)
	}
	return app, db
}

func laneTitles(t *testing.T, db *fstore.Store, lane string) []string {
	t.Helper()
	cards, err := db.Cards("Boards/work.md", lane)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, c := range cards {
		out = append(out, c.Title)
	}
	return out
}

// Moving a card among filtered cards puts it among the cards shown, and
// the hidden cards keep their places (T3).
func TestAFilteredMoveKeepsTheHiddenCardsInPlace(t *testing.T) {
	app, db := boardApp(t, "a1 match", "b hidden", "c2 match", "d hidden", "e3 match")
	app.setFilter("match", "")
	// Shown: a1, c2, e3. Move e3 above c2.
	app.Update(key("j"))
	app.Update(key("j"))
	app.Update(key("K"))
	app.Update(key("enter"))
	app.Update(key("y"))
	if app.err != nil {
		t.Fatal(app.err)
	}
	got := strings.Join(laneTitles(t, db, "Backlog"), ", ")
	if want := "a1 match, b hidden, e3 match, c2 match, d hidden"; got != want {
		t.Errorf("lane = %s\nwant   %s", got, want)
	}
	if c := app.selectedCard(); c == nil || c.Title != "e3 match" {
		t.Errorf("the moved card should stay selected, got %v", c)
	}
}

// Moving a filtered card last puts it right below the last card shown,
// not at the very end past hidden cards.
func TestAFilteredMoveToTheEndGoesBelowTheLastShownCard(t *testing.T) {
	app, db := boardApp(t, "a1 match", "c2 match", "d hidden")
	app.setFilter("match", "")
	app.Update(key("J"))
	app.Update(key("enter"))
	app.Update(key("y"))
	got := strings.Join(laneTitles(t, db, "Backlog"), ", ")
	if want := "c2 match, a1 match, d hidden"; got != want {
		t.Errorf("lane = %s\nwant   %s", got, want)
	}
}

func TestMovingIntoAFullColumnShowsTheLimit(t *testing.T) {
	app, db := boardApp(t, "one", "two")
	if err := db.SetWIPLimit("Boards/work.md", "Todo", 1); err != nil {
		t.Fatal(err)
	}
	db.AddCard("Boards/work.md", "Todo", "already here", fstore.CardFields{}, false)
	app.loadBoard()
	app.Update(key("L"))
	app.Update(key("enter"))
	app.Update(key("y"))
	var wip *fstore.WIPLimitError
	if !errors.As(app.err, &wip) {
		t.Fatalf("err = %v, want the WIP limit", app.err)
	}
	if app.board.moving || app.board.focusCol != 0 {
		t.Errorf("the move should be undone: moving %v, column %d", app.board.moving, app.board.focusCol)
	}
	if got := laneTitles(t, db, "Backlog"); len(got) != 2 {
		t.Errorf("Backlog = %v", got)
	}
}

// A long column shows the selected card, with the title bar and key hints
// still on screen (T4).
func TestALongColumnKeepsTheSelectedCardAndTheBarsOnScreen(t *testing.T) {
	var titles []string
	for i := 1; i <= 30; i++ {
		titles = append(titles, fmt.Sprintf("Card %02d", i))
	}
	app, _ := boardApp(t, titles...)
	for i := 0; i < 29; i++ {
		app.Update(key("j"))
	}
	view := app.View()
	if h := lipgloss.Height(view); h > app.height {
		t.Errorf("view is %d lines on a %d-line terminal", h, app.height)
	}
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[0], "kb: Default > work") {
		t.Errorf("first line should be the title bar: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "hjkl") {
		t.Errorf("last line should be the key hints: %q", lines[len(lines)-1])
	}
	if !strings.Contains(view, "Card 30") || !strings.Contains(view, "↑") || strings.Contains(view, "Card 01") {
		t.Errorf("the window should show the last card and say there are more above:\n%s", view)
	}
}

func TestALongWorkspaceListKeepsTheCursorOnScreen(t *testing.T) {
	db := testStore(t)
	for i := 1; i <= 40; i++ {
		db.CreateNote(fmt.Sprintf("Note %02d", i), "", "", db.DefaultWorkspace().ID)
	}
	app := NewApp(db, "")
	app.width, app.height = 80, 20
	app.Init()
	app.Update(key("enter"))
	for i := 0; i < 39; i++ {
		app.Update(key("j"))
	}
	view := app.View()
	if h := lipgloss.Height(view); h > app.height {
		t.Errorf("view is %d lines on a %d-line terminal", h, app.height)
	}
	if !strings.Contains(view, "▸ ") || !strings.Contains(strings.Split(view, "\n")[0], "Default") {
		t.Errorf("cursor or title missing:\n%s", view)
	}
}

// An error never replaces the list it is about, and it goes on the next
// key (T2, T13).
func TestAnErrorShowsAboveTheKeysAndClearsOnTheNextKey(t *testing.T) {
	db := testStore(t)
	db.CreateBoard("taken", "", db.DefaultWorkspace().ID)
	app := NewApp(db, "")
	app.Init()
	app.Update(key("enter"))
	app.Update(key("n"))
	app.Update(key("taken"))
	app.Update(key("enter"))
	if app.err == nil || app.mode != modeWSContent {
		t.Fatalf("creating a duplicate board: err %v, mode %d", app.err, app.mode)
	}
	if view := app.View(); !strings.Contains(view, "already exists") || !strings.Contains(view, "taken") {
		t.Errorf("both the error and the list should show:\n%s", view)
	}
	app.Update(key("j"))
	if app.err != nil || strings.Contains(app.View(), "already exists") {
		t.Error("the error should clear on the next key")
	}
}

// Each screen reads the vault when it opens, so changes made outside kb
// show up (T5).
func TestScreensShowChangesMadeOutsideKB(t *testing.T) {
	db := testStore(t)
	note, _ := db.CreateNote("Shared", "", "first", db.DefaultWorkspace().ID)
	app := NewApp(db, "")
	app.Init()
	app.Update(key("enter")) // the workspace
	app.Update(key("enter")) // the note

	os.WriteFile(filepath.Join(db.Vault().Root(), "Elsewhere.md"), []byte("written by another app"), 0o644)
	path := filepath.Join(db.Vault().Root(), note.Path)
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), "first", "second", 1)), 0o644)

	app.Update(key("b"))
	if app.mode != modeWSContent || len(app.wsContent.notes) != 2 {
		t.Fatalf("mode %d, notes %d, want the new file too", app.mode, len(app.wsContent.notes))
	}
	for i, n := range app.wsContent.notes {
		if n.Title == "Shared" {
			app.wsContent.cursor = i
		}
	}
	data, _ = os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), "second", "third", 1)), 0o644)
	app.Update(key("enter"))
	if app.mode != modeNoteView || app.noteView.note.Body != "third" {
		t.Errorf("note view shows %q, want the file as it is now", app.noteView.note.Body)
	}
}

// Going back to a workspace keeps the cursor where it was (T10).
func TestBackToTheWorkspaceKeepsTheCursor(t *testing.T) {
	db := testStore(t)
	for _, title := range []string{"A", "B", "C"} {
		db.CreateNote(title, "", "", db.DefaultWorkspace().ID)
	}
	app := NewApp(db, "")
	app.Init()
	app.Update(key("enter"))
	app.Update(key("j"))
	app.Update(key("j"))
	want := app.wsContent.selectedNote().ID
	app.Update(key("enter"))
	app.Update(key("b"))
	if got := app.wsContent.selectedNote(); got == nil || got.ID != want {
		t.Errorf("cursor moved to %v", got)
	}
}

// A new card is written once, fields and all (T12).
func TestANewCardIsWrittenWithAllItsFields(t *testing.T) {
	app, db := boardApp(t)
	app.Update(key("n"))
	app.Update(key("Write docs"))
	app.Update(key("tab"))
	app.Update(key("h")) // medium -> high
	app.Update(key("h")) // high -> urgent
	app.Update(key("tab"))
	app.Update(key("docs, big task"))
	app.Update(key("enter"))
	if app.err != nil || app.mode != modeBoard {
		t.Fatalf("err %v, mode %d", app.err, app.mode)
	}
	cards, _ := db.Cards("Boards/work.md", "Backlog")
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	c := cards[0]
	if c.Title != "Write docs" || c.Priority != model.PriorityUrgent || c.Labels != "docs,big-task" {
		t.Errorf("card = %+v", c)
	}
}

// From the card viewer, e then Esc goes back to the viewer, and a save
// shows the saved card there (T15).
func TestEditingFromTheViewerReturnsToTheViewer(t *testing.T) {
	app, _ := boardApp(t, "Original")
	app.Update(key("enter"))
	app.Update(key("e"))
	app.Update(key("esc"))
	if app.mode != modeCardView {
		t.Fatalf("Esc from the form went to mode %d, want the viewer", app.mode)
	}
	app.Update(key("e"))
	app.Update(key(" and more"))
	app.Update(key("enter"))
	if app.mode != modeCardView || app.cardView.card.Title != "Original and more" {
		t.Errorf("mode %d, card %q", app.mode, app.cardView.card.Title)
	}
}

func TestArchiveAndDeleteFromTheBoard(t *testing.T) {
	app, db := boardApp(t, "keep", "archive me", "delete me")
	app.Update(key("j"))
	app.Update(key("d"))
	app.Update(key("y"))
	app.Update(key("j"))
	app.Update(key("D"))
	app.Update(key("n")) // cancelled
	app.Update(key("D"))
	app.Update(key("y"))
	if got := laneTitles(t, db, "Backlog"); strings.Join(got, ",") != "keep" {
		t.Errorf("Backlog = %v", got)
	}
	archived, _ := db.ArchivedCards("Boards/work.md")
	if len(archived) != 1 || archived[0].Title != "archive me" {
		t.Errorf("archived = %v", archived)
	}
	if !strings.Contains(app.feedback, "delete me") {
		t.Errorf("feedback = %q", app.feedback)
	}
}

// Deleting a note moves its file to the vault's trash and goes back.
func TestDeletingANoteMovesItToTheTrash(t *testing.T) {
	app, db, note := testEditApp(t)
	app.Update(key("d"))
	app.Update(key("y"))
	if app.mode != modeWSContent || !strings.Contains(app.feedback, ".trash/") {
		t.Errorf("mode %d, feedback %q", app.mode, app.feedback)
	}
	if _, err := os.Stat(filepath.Join(db.Vault().Root(), ".trash", note.Path)); err != nil {
		t.Errorf("not in the trash: %v", err)
	}
	if len(app.wsContent.notes) != 0 {
		t.Errorf("notes = %v", app.wsContent.notes)
	}
}

func TestTheNoteViewListsCardBacklinks(t *testing.T) {
	app, db := boardApp(t, "Read [[Spec]] first")
	note, _ := db.CreateNote("Spec", "", "the spec", db.DefaultWorkspace().ID)
	app.switchToNoteView(note.ID)
	if view := app.View(); !strings.Contains(view, `card "Read [[Spec]] first" on work`) {
		t.Errorf("view:\n%s", view)
	}
}

// The window fills the column: a "more" line takes one line, not two.
func TestALongColumnUsesTheSpaceItHas(t *testing.T) {
	var cards []*model.Card
	for i := 0; i < 14; i++ {
		cards = append(cards, &model.Card{ID: fmt.Sprint(i), ColumnID: "Backlog", Title: fmt.Sprint("Card ", i), Priority: model.PriorityMedium})
	}
	app := testApp(testColumns(), map[string][]*model.Card{"Backlog": cards})
	// Header 2 lines, then 21: four 4-line cards would leave 5, so five
	// cards (20) and the "more" line (1) fit exactly.
	out := app.renderSingleColumn(app.board.lanes[0], 0, 26, 23)
	if n := strings.Count(out, "Card "); n != 5 || !strings.Contains(out, "↓ 9 more") {
		t.Errorf("shows %d cards:\n%s", n, out)
	}
}
