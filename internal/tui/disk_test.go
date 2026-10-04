package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/fstore"
)

// rewriteBoard changes the board's file the way an editor or another kb
// would, behind the TUI's back.
func rewriteBoard(t *testing.T, db *fstore.Store, change func(string) string) {
	t.Helper()
	path := filepath.Join(db.Vault().Root(), "Boards", "work.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(change(string(data))), 0o644); err != nil {
		t.Fatal(err)
	}
}

func shown(app *App) []string {
	var out []string
	for _, c := range app.board.cards["Backlog"] {
		out = append(out, c.Title)
	}
	return out
}

// A card added to the board's file elsewhere shows on the next key, and r
// reads the board again on demand.
func TestTheBoardFollowsItsFile(t *testing.T) {
	app, db := boardApp(t, "one")
	rewriteBoard(t, db, func(s string) string {
		return strings.Replace(s, "- [ ] one", "- [ ] one\n- [ ] added elsewhere", 1)
	})
	app.Update(key("j"))
	if got := strings.Join(shown(app), ","); got != "one,added elsewhere" {
		t.Errorf("after a key: %s", got)
	}
	rewriteBoard(t, db, func(s string) string {
		return strings.Replace(s, "- [ ] added elsewhere", "- [ ] renamed elsewhere", 1)
	})
	app.Update(key("r"))
	if got := strings.Join(shown(app), ","); got != "one,renamed elsewhere" {
		t.Errorf("after r: %s", got)
	}
	if c := app.selectedCard(); c == nil || c.Title != "renamed elsewhere" {
		t.Errorf("the selected card should stay selected, got %v", c)
	}
}

// A save refused because the card changed on disk keeps what was typed,
// says so, and saves it on the next try.
func TestASaveThatConflictsKeepsTheTypedEdits(t *testing.T) {
	app, db := boardApp(t, "original")
	app.Update(key("e"))
	app.card.titleInput.SetValue("typed in kb")
	rewriteBoard(t, db, func(s string) string {
		return strings.Replace(s, "original", "changed in an editor", 1)
	})
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.mode != modeCardEdit || app.err == nil || !strings.Contains(app.err.Error(), "changed on disk") {
		t.Fatalf("mode %d, err %v: the form should stay open with the reason", app.mode, app.err)
	}
	if got := app.card.titleInput.Value(); got != "typed in kb" {
		t.Errorf("the typed title is gone: %q", got)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.err != nil {
		t.Fatal(app.err)
	}
	if got := laneTitles(t, db, "Backlog"); strings.Join(got, ",") != "typed in kb" {
		t.Errorf("Backlog = %v", got)
	}
}

// A card removed on disk while its form is open is reported, not saved
// back over another card.
func TestEditingACardRemovedOnDiskSaysSo(t *testing.T) {
	app, db := boardApp(t, "doomed")
	app.Update(key("e"))
	rewriteBoard(t, db, func(s string) string { return strings.Replace(s, "- [ ] doomed", "", 1) })
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.err == nil || !strings.Contains(app.err.Error(), "no longer on the board") {
		t.Errorf("err = %v", app.err)
	}
}

// Long fields survive a round trip through the form unchanged.
func TestLongFieldsSurviveAnEdit(t *testing.T) {
	long := strings.Repeat("long title words ", 20)
	app, db := boardApp(t, strings.TrimSpace(long))
	var desc []string
	for i := 0; i < 150; i++ {
		desc = append(desc, "line")
	}
	c := app.selectedCard()
	c.Description = strings.Join(desc, "\n")
	c.Labels = strings.Repeat("label", 50)
	c.ExternalID = "ext:" + strings.Repeat("x", 150)
	if err := db.UpdateCard(c); err != nil {
		t.Fatal(err)
	}
	app.loadBoard()
	before := *app.selectedCard()
	app.Update(key("e"))
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.err != nil {
		t.Fatal(app.err)
	}
	after := app.selectedCard()
	if after.Title != before.Title || after.Description != before.Description || after.Labels != before.Labels || after.ExternalID != before.ExternalID {
		t.Errorf("an unchanged save changed the card:\n before %+v\n after  %+v", before, *after)
	}
}

// After the terminal shrinks, the focused column is still on screen.
func TestAResizeKeepsTheFocusedColumnOnScreen(t *testing.T) {
	app, _ := boardApp(t, "one")
	for i := 0; i < 4; i++ {
		app.Update(key("l"))
	}
	app.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	view := app.View()
	focused := app.board.lanes[app.board.focusCol].Name
	if !strings.Contains(view, focused) {
		t.Errorf("column %q is off screen at 40 wide:\n%s", focused, view)
	}
}

// fits reports the lines of a view wider or taller than the terminal.
func fits(t *testing.T, name, view string, w, h int) {
	t.Helper()
	if got := lipgloss.Height(view); got > h {
		t.Errorf("%s: %d lines on a %d-line terminal", name, got, h)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > w {
			t.Errorf("%s: a line is %d cells on a %d-cell terminal: %q", name, got, w, line)
			return
		}
	}
}

// Every board screen fits small terminals: dialogs, the filter bar, the
// help, and an error in a wide script.
func TestBoardScreensFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {30, 10}} {
		w, h := size[0], size[1]
		app, _ := boardApp(t, "A card with a title long enough to need cutting short somewhere")
		app.Update(tea.WindowSizeMsg{Width: w, Height: h})
		fits(t, "board", app.View(), w, h)

		app.Update(key("d"))
		fits(t, "archive question", app.View(), w, h)
		app.Update(key("n"))

		app.Update(key("/"))
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat("filter ", 10))})
		fits(t, "filter bar", app.View(), w, h)
		app.Update(key("esc"))

		app.Update(key("?"))
		fits(t, "help", app.View(), w, h)
		app.Update(key("x"))

		app.Update(key("enter"))
		fits(t, "card view", app.View(), w, h)
		app.Update(key("d"))
		fits(t, "card view question", app.View(), w, h)
		app.Update(key("n"))
		app.Update(key("e"))
		fits(t, "card form", app.View(), w, h)
		app.Update(key("esc"))

		app.mode = modeBoard
		app.err = &wideError{}
		fits(t, "a wide error", app.View(), w, h)
		if first := strings.Split(app.View(), "\n")[0]; !strings.Contains(first, "kb") {
			t.Errorf("%dx%d: the title bar was pushed off: %q", w, h, first)
		}
	}
}

type wideError struct{}

func (*wideError) Error() string {
	return strings.Repeat("変更が保存されませんでした ", 6)
}

// The picker, a workspace and a note fit small terminals too.
func TestOtherScreensFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {30, 10}} {
		w, h := size[0], size[1]
		db := testStore(t)
		if _, err := db.CreateNote("A note with quite a long title for a small screen", "", strings.Repeat("body text ", 80), db.DefaultWorkspace().ID); err != nil {
			t.Fatal(err)
		}
		app := NewApp(db, "")
		app.Init()
		app.Update(tea.WindowSizeMsg{Width: w, Height: h})
		fits(t, "picker", app.View(), w, h)
		app.Update(key("enter"))
		if app.mode != modeWSContent {
			t.Fatalf("mode %d", app.mode)
		}
		fits(t, "workspace", app.View(), w, h)
		app.Update(key("enter"))
		fits(t, "note", app.View(), w, h)
	}
}

// After a conflict, only the fields typed in the form win: a field left
// alone takes what the other change wrote, so saving again loses nothing.
func TestAConflictKeepsOnlyTheFieldsTyped(t *testing.T) {
	app, db := boardApp(t, "original")
	app.Update(key("e"))
	app.card.descInput.SetValue("described in kb")
	rewriteBoard(t, db, func(s string) string {
		return strings.Replace(s, "original", "retitled elsewhere #urgent", 1)
	})
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.err == nil {
		t.Fatal("want the conflict reported")
	}
	if got := app.card.titleInput.Value(); got != "retitled elsewhere" {
		t.Errorf("the title nobody typed should follow the file: %q", got)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if app.err != nil {
		t.Fatal(app.err)
	}
	c := app.selectedCard()
	if c.Title != "retitled elsewhere" || c.Description != "described in kb" || !strings.Contains(c.Labels, "urgent") {
		t.Errorf("card = %+v", *c)
	}
}
