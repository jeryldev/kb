package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jeryldev/kb/internal/fstore"
)

var firstCardID = regexp.MustCompile(`(- \[ \] one \^[0-9a-f]+)`)

func boardText(t *testing.T, db *fstore.Store) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(db.Vault().Root(), "Boards", "work.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func ctrlS() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlS} }

// The form's fields clean text as it is loaded (a tab becomes spaces, a
// stray CR a line break). A field left alone is saved as the card had it.
func TestFieldsLeftAloneKeepTheirText(t *testing.T) {
	for name, change := range map[string]string{
		"tab in description": "$1\n    a\tb",
		"CR in description":  "$1\n    line one\r\n    line two",
		"tab in title":       "- [ ] one\tx ^aaaabbbb",
	} {
		t.Run(name, func(t *testing.T) {
			app, db := boardApp(t, "one")
			rewriteBoard(t, db, func(s string) string { return firstCardID.ReplaceAllString(s, change) })
			app.Update(key("r"))
			before := *app.selectedCard()
			app.Update(key("e"))
			app.Update(tea.KeyMsg{Type: tea.KeyTab})
			app.Update(key("h")) // only the priority changes
			app.Update(ctrlS())
			if app.err != nil {
				t.Fatal(app.err)
			}
			after := app.selectedCard()
			if after.Title != before.Title || after.Description != before.Description {
				t.Errorf("title %q -> %q, description %q -> %q", before.Title, after.Title, before.Description, after.Description)
			}
		})
	}
}

// After a conflict, a field left alone takes the other change, however
// the field cleaned its text.
func TestAConflictTakesTheOtherChangeToAFieldLeftAlone(t *testing.T) {
	app, db := boardApp(t, "one")
	rewriteBoard(t, db, func(s string) string { return firstCardID.ReplaceAllString(s, "$1\n    a\tb") })
	app.Update(key("r"))
	app.Update(key("e"))
	app.card.titleInput.SetValue("typed")
	rewriteBoard(t, db, func(s string) string { return strings.Replace(s, "a\tb", "changed elsewhere", 1) })
	app.Update(ctrlS())
	if got := app.card.descInput.Value(); got != "changed elsewhere" {
		t.Errorf("description in the form = %q", got)
	}
	app.Update(ctrlS())
	if c := app.selectedCard(); c.Title != "typed" || c.Description != "changed elsewhere" {
		t.Errorf("card = %+v", *c)
	}
}

// After a conflict, the card viewer shows the card as it now is.
func TestTheViewerAfterAConflictShowsTheCardAsItIs(t *testing.T) {
	app, db := boardApp(t, "original")
	app.Update(key("enter"))
	app.Update(key("e"))
	app.card.descInput.SetValue("typed")
	rewriteBoard(t, db, func(s string) string { return strings.Replace(s, "original", "changed elsewhere", 1) })
	app.Update(ctrlS())
	app.mode = modeCardEdit
	app.Update(key("esc"))
	if app.mode == modeCardEdit {
		app.Update(key("y")) // discard the typed text
	}
	if app.mode != modeCardView || app.cardView.card.Title != "changed elsewhere" {
		t.Errorf("mode %d, viewer shows %q", app.mode, app.cardView.card.Title)
	}
}

// A card that is gone from the file while its form is open (a plugin card
// renamed in Obsidian gets a new id) keeps the typed text, which Enter
// saves as a new card.
func TestTypedTextOutlivesItsCard(t *testing.T) {
	app, db := boardApp(t, "one")
	app.Update(key("e"))
	app.card.labelsInput.SetValue("typed-label")
	rewriteBoard(t, db, func(s string) string { return firstCardID.ReplaceAllString(s, "- [ ] someone else's card") })
	app.Update(ctrlS())
	if app.mode != modeCardEdit || app.err == nil || strings.Contains(app.err.Error(), "\"") && regexp.MustCompile(`[0-9a-f]{8}`).MatchString(app.err.Error()) {
		t.Fatalf("mode %d, err %v: want the form open and a plain message", app.mode, app.err)
	}
	app.Update(ctrlS())
	if app.err != nil {
		t.Fatal(app.err)
	}
	if got := boardText(t, db); !strings.Contains(got, "one #typed-label") || !strings.Contains(got, "someone else's card") {
		t.Errorf("board:\n%s", got)
	}
}

// A board whose file is removed while it is open says so and goes back.
func TestABoardRemovedOnDiskSaysSo(t *testing.T) {
	app, db := boardApp(t, "one")
	os.Remove(filepath.Join(db.Vault().Root(), "Boards", "work.md"))
	app.Update(key("j"))
	if app.err == nil || app.mode == modeBoard {
		t.Errorf("mode %d, err %v", app.mode, app.err)
	}
	app2, db2 := boardApp(t, "one")
	app2.Update(key("b"))
	os.Remove(filepath.Join(db2.Vault().Root(), "Boards", "work.md"))
	app2.Update(key("enter"))
	if app2.mode == modeBoard {
		t.Errorf("opened a board that is gone:\n%s", app2.View())
	}
}

// A full column offers to go over its limit, in the TUI's own terms.
func TestAFullColumnAsksToGoOverItsLimit(t *testing.T) {
	app, db := boardApp(t, "one")
	if err := db.SetWIPLimit("Boards/work.md", "Todo", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddCard("Boards/work.md", "Todo", "full", fstore.CardFields{}, false); err != nil {
		t.Fatal(err)
	}
	app.Update(key("r"))
	app.Update(key("L"))
	app.Update(key("enter"))
	app.Update(key("y"))
	if app.err == nil || strings.Contains(app.err.Error(), "--force") {
		t.Fatalf("move: err = %v", app.err)
	}
	if !strings.Contains(app.View(), "Todo is full") {
		t.Errorf("the question should say why it asks:\n%s", app.View())
	}
	if app.board.confirming != "" {
		app.Update(key("y"))
	}
	if got := laneTitles(t, db, "Todo"); len(got) != 2 {
		t.Errorf("Todo = %v after confirming the move over the limit", got)
	}
	app.Update(key("n"))
	app.card.titleInput.SetValue("long typed card")
	app.Update(key("enter"))
	if app.err == nil || strings.Contains(app.err.Error(), "--force") || app.mode != modeCardEdit {
		t.Fatalf("new card: mode %d err %v", app.mode, app.err)
	}
	app.Update(key("enter"))
	if got := laneTitles(t, db, "Todo"); len(got) != 3 {
		t.Errorf("Todo = %v after adding over the limit", got)
	}
}

// Keys that arrive together (typed fast, or a repeat) each count.
func TestAKeyBurstCountsEveryKey(t *testing.T) {
	app, db := boardApp(t, "a", "b", "c", "d")
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	if app.board.focusCard != 2 {
		t.Errorf("focusCard %d after jj", app.board.focusCard)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dy")})
	if got := laneTitles(t, db, "Backlog"); len(got) != 3 {
		t.Errorf("Backlog = %v after d then y", got)
	}
}

// Esc on a form with typed changes asks before dropping them.
func TestEscAsksBeforeDroppingTypedChanges(t *testing.T) {
	app, _ := boardApp(t, "one")
	app.Update(key("e"))
	app.Update(key("esc"))
	if app.mode != modeBoard {
		t.Fatalf("an untouched form should close on Esc: mode %d", app.mode)
	}
	app.Update(key("e"))
	app.card.titleInput.SetValue("typed")
	app.Update(key("esc"))
	if app.mode != modeCardEdit {
		t.Fatalf("a typed form closed on Esc without asking")
	}
	app.Update(key("n"))
	if app.mode != modeCardEdit || app.card.titleInput.Value() != "typed" {
		t.Fatalf("n should keep the form: mode %d, title %q", app.mode, app.card.titleInput.Value())
	}
	app.Update(key("esc"))
	app.Update(key("y"))
	if app.mode != modeBoard {
		t.Errorf("y should drop the form: mode %d", app.mode)
	}
}

// A start-up board that does not exist says so on the picker.
func TestAMissingStartupBoardSaysSo(t *testing.T) {
	db := testStore(t)
	app := NewApp(db, "nosuch")
	app.Init()
	if app.mode != modePicker || app.feedback == "" && app.err == nil {
		t.Errorf("mode %d, nothing said about the missing board", app.mode)
	}
}

// The card form keeps its description field on a short terminal.
func TestTheCardFormFitsAShortTerminal(t *testing.T) {
	app, _ := boardApp(t, "one")
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	app.Update(key("e"))
	out := app.View()
	if !strings.Contains(out, "Description") || lipgloss.Height(out) > 12 {
		t.Errorf("at 80x12:\n%s", out)
	}
}
