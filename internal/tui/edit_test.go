package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/store"
)

var titleLine = regexp.MustCompile(`(?m)^title: .*\n`)

func testEditApp(t *testing.T) (*App, *store.DB) {
	t.Helper()
	db, err := store.OpenWithPath(":memory:", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &App{db: db, mode: modeNoteView, width: 80, height: 24}, db
}

func TestEditOpensTheVaultFileItself(t *testing.T) {
	app, db := testEditApp(t)
	ws, _ := db.GetDefaultWorkspace()
	note, _ := db.CreateNote("Edit Me", "edit-me", "body", ws.ID)
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
	app, db := testEditApp(t)
	ws, _ := db.GetDefaultWorkspace()
	note, _ := db.CreateNote("Edit Me", "edit-me", "before", ws.ID)

	// What the editor does: rewrite the file, frontmatter and all.
	path := filepath.Join(db.Vault().Root(), note.Path)
	data, _ := os.ReadFile(path)
	edited := strings.Replace(string(data), "before", "after, with a [[Link]]", 1)
	edited = titleLine.ReplaceAllString(edited, "")
	edited = strings.Replace(edited, "---\n", "---\ntitle: Edited Title\n", 1)
	os.WriteFile(path, []byte(edited), 0o644)
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(path, later, later)

	msg := app.afterNoteEdit(note.ID)(nil)
	got, ok := msg.(noteEditedMsg)
	if !ok {
		t.Fatalf("msg = %#v", msg)
	}
	if got.note.Body != "after, with a [[Link]]" || got.note.Title != "Edited Title" {
		t.Errorf("note = %+v", got.note)
	}
	if links, _ := db.GetForwardLinks("note", note.ID); len(links) != 1 {
		t.Errorf("links = %d", len(links))
	}
}

func TestAfterEditOfADeletedFileLeavesTheNote(t *testing.T) {
	app, db := testEditApp(t)
	ws, _ := db.GetDefaultWorkspace()
	note, _ := db.CreateNote("Doomed", "doomed", "x", ws.ID)
	os.Remove(filepath.Join(db.Vault().Root(), note.Path))

	if _, ok := app.afterNoteEdit(note.ID)(nil).(noteDeletedMsg); !ok {
		t.Error("expected noteDeletedMsg")
	}
}

func TestEditingLeavesNoTempFiles(t *testing.T) {
	app, db := testEditApp(t)
	ws, _ := db.GetDefaultWorkspace()
	note, _ := db.CreateNote("Clean", "clean", "x", ws.ID)
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
// can never leave it.
func TestStartupBoardOpensOnlyOnce(t *testing.T) {
	db, err := store.OpenWithPath(":memory:", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ws, _ := db.GetDefaultWorkspace()
	if _, err := db.CreateBoard("auto", "", ws.ID); err != nil {
		t.Fatal(err)
	}
	app := NewApp(db, "auto")

	if _, ok := app.initPicker()().(boardCreatedMsg); !ok {
		t.Fatal("the start-up board should open first")
	}
	if msg, ok := app.initPicker()().(workspacesLoadedMsg); !ok {
		t.Errorf("going back reopened the board instead of the picker: %#v", msg)
	}
}
