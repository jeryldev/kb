package tui

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
)

var titleLine = regexp.MustCompile(`(?m)^title: .*\n`)

// testStore opens a store on an empty vault in the test's temp dir.
func testStore(t *testing.T) *fstore.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := fstore.Open(filepath.Join(dir, "vault"), fstore.Options{
		ConfigDir: filepath.Join(dir, "config"),
		LockDir:   filepath.Join(dir, "locks"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func testEditApp(t *testing.T) (*App, *fstore.Store, *model.Note) {
	t.Helper()
	db := testStore(t)
	note, err := db.CreateNote("Edit Me", "", "before", db.DefaultWorkspace().ID)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{db: db, width: 80, height: 24}
	app.switchToWSContent(db.DefaultWorkspace())
	app.switchToNoteView(note.ID)
	return app, db, note
}

func TestEditOpensTheVaultFileItself(t *testing.T) {
	app, db, note := testEditApp(t)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nvim -u NONE")

	cmd, err := app.noteEditorCommand(note)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(db.Vault().Root(), note.Path)
	if got := cmd.Args; len(got) != 4 || got[0] != "nvim" || got[3] != want {
		t.Errorf("args = %q, want nvim -u NONE %s", got, want)
	}
}

func TestAfterEditTheNoteIsReadBackFromItsFile(t *testing.T) {
	app, db, note := testEditApp(t)
	db.CreateNote("Link", "", "", db.DefaultWorkspace().ID)

	// What the editor does: rewrite the file, frontmatter and all.
	path := filepath.Join(db.Vault().Root(), note.Path)
	data, _ := os.ReadFile(path)
	edited := strings.Replace(string(data), "before", "after, with a [[Link]]", 1)
	edited = titleLine.ReplaceAllString(edited, "")
	edited = strings.Replace(edited, "---\n", "---\ntitle: Edited Title\n", 1)
	os.WriteFile(path, []byte(edited), 0o644)

	app.Update(noteEditedMsg{noteID: note.ID})
	got := app.noteView.note
	if app.mode != modeNoteView || got.Body != "after, with a [[Link]]" || got.Title != "Edited Title" {
		t.Errorf("mode %d, note %+v", app.mode, got)
	}
	link, _ := db.ResolveNoteRef("Link")
	if bl := db.Backlinks(link.ID); len(bl) != 1 {
		t.Errorf("backlinks to Link = %v", bl)
	}
}

func TestAfterEditOfADeletedFileGoesBack(t *testing.T) {
	app, db, note := testEditApp(t)
	os.Remove(filepath.Join(db.Vault().Root(), note.Path))

	app.Update(noteEditedMsg{noteID: note.ID})
	if app.mode != modeWSContent || !strings.Contains(app.feedback, "gone") {
		t.Errorf("mode %d, feedback %q", app.mode, app.feedback)
	}
}

// An editor that fails leaves the note on screen with the error shown,
// until the next key (T1).
func TestEditorFailureStaysOnTheNoteWithTheError(t *testing.T) {
	app, _, note := testEditApp(t)
	app.Update(noteEditedMsg{noteID: note.ID, err: errors.New("exit status 1")})
	if app.mode != modeNoteView || app.err == nil {
		t.Fatalf("mode %d, err %v", app.mode, app.err)
	}
	if view := app.View(); !strings.Contains(view, "exit status 1") || !strings.Contains(view, "before") {
		t.Errorf("the error and the note should both show:\n%s", view)
	}
	app.Update(key("j"))
	if app.err != nil {
		t.Error("the error should clear on the next key")
	}
}

func TestEditingLeavesNoTempFiles(t *testing.T) {
	app, _, note := testEditApp(t)
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "kb-note-*"))
	t.Setenv("EDITOR", "true")
	if _, err := app.noteEditorCommand(note); err != nil {
		t.Fatal(err)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "kb-note-*"))
	if len(after) > len(before) {
		t.Errorf("temp files created: %v", after)
	}
}

// A board opened automatically at start-up (KB_BOARD, the tmux session or
// the folder name) must not reopen every time the picker loads, or "b"
// can never leave it. "b" goes to the board's workspace.
func TestStartupBoardOpensOnlyOnce(t *testing.T) {
	db := testStore(t)
	ws, err := db.CreateWorkspace("Work", model.KindProject, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateBoard("auto", "", ws.ID); err != nil {
		t.Fatal(err)
	}
	app := NewApp(db, "auto")
	app.Init()
	if app.mode != modeBoard || app.board.board.Name != "auto" {
		t.Fatalf("the start-up board should open first; mode %d", app.mode)
	}
	app.Update(key("b"))
	if app.mode != modeWSContent || app.wsContent.workspace.Name != "Work" {
		t.Fatalf("b should go to the board's workspace; mode %d", app.mode)
	}
	app.Update(key("b"))
	if app.mode != modePicker {
		t.Errorf("going back reopened something other than the picker: mode %d", app.mode)
	}
}

func TestAMissingStartupBoardShowsThePicker(t *testing.T) {
	db := testStore(t)
	app := NewApp(db, "no-such-board")
	app.Init()
	if app.mode != modePicker || app.err != nil {
		t.Errorf("mode %d, err %v", app.mode, app.err)
	}
}
