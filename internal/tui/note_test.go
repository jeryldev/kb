package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jeryldev/kb/internal/model"
)

func testNotes() []*model.Note {
	return []*model.Note{
		{ID: "n1", Title: "Alpha Note", Slug: "alpha-note", Body: "Content of alpha", Tags: "go,test", UpdatedAt: time.Now()},
		{ID: "n2", Title: "Beta Note", Slug: "beta-note", Body: "Content of beta", Tags: "rust", UpdatedAt: time.Now()},
		{ID: "n3", Title: "Gamma Note", Slug: "gamma-note", Body: "", Tags: "", UpdatedAt: time.Now()},
	}
}

func testNoteApp(notes []*model.Note) *App {
	return &App{
		mode: modeNotes,
		noteList: noteListModel{
			notes: notes,
		},
		width:  80,
		height: 24,
	}
}

func TestNoteListNavigation(t *testing.T) {
	app := testNoteApp(testNotes())

	if app.noteList.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", app.noteList.cursor)
	}

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.noteList.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", app.noteList.cursor)
	}

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.noteList.cursor != 2 {
		t.Errorf("after j: cursor = %d, want 2", app.noteList.cursor)
	}

	// Should not go past end
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.noteList.cursor != 2 {
		t.Errorf("after j at end: cursor = %d, want 2", app.noteList.cursor)
	}

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.noteList.cursor != 1 {
		t.Errorf("after k: cursor = %d, want 1", app.noteList.cursor)
	}
}

func TestNoteListNavigationBoundary(t *testing.T) {
	app := testNoteApp(testNotes())

	// k at top should stay at 0
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.noteList.cursor != 0 {
		t.Errorf("k at top: cursor = %d, want 0", app.noteList.cursor)
	}
}

func TestNoteListFilter(t *testing.T) {
	app := testNoteApp(testNotes())

	// Enter filter mode
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !app.noteList.filtering {
		t.Fatal("expected filtering to be true")
	}

	// Type filter text
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	// Confirm filter
	app.updateNoteList(tea.KeyMsg{Type: tea.KeyEnter})

	if app.noteList.filtering {
		t.Fatal("expected filtering to be false after enter")
	}
	if app.noteList.filter != "alph" {
		t.Errorf("filter = %q, want 'alph'", app.noteList.filter)
	}

	filtered := app.filteredNotes()
	if len(filtered) != 1 {
		t.Errorf("filtered = %d, want 1", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].Slug != "alpha-note" {
		t.Errorf("filtered[0].Slug = %q, want 'alpha-note'", filtered[0].Slug)
	}
}

func TestNoteListFilterByTag(t *testing.T) {
	app := testNoteApp(testNotes())

	app.noteList.filter = "rust"
	filtered := app.filteredNotes()

	if len(filtered) != 1 {
		t.Errorf("filtered = %d, want 1", len(filtered))
	}
	if len(filtered) > 0 && filtered[0].Slug != "beta-note" {
		t.Errorf("filtered[0].Slug = %q, want 'beta-note'", filtered[0].Slug)
	}
}

func TestNoteListFilterEscape(t *testing.T) {
	app := testNoteApp(testNotes())

	app.noteList.filter = "alpha"
	app.noteList.filtering = true
	app.noteList.filterInput = "alpha"

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyEscape})

	if app.noteList.filtering {
		t.Fatal("expected filtering to be false")
	}
	if app.noteList.filter != "" {
		t.Errorf("filter = %q, want empty (cleared on esc)", app.noteList.filter)
	}
}

func TestNoteListFilterBackspace(t *testing.T) {
	app := testNoteApp(testNotes())

	app.noteList.filtering = true
	app.noteList.filterInput = "abc"

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyBackspace})
	if app.noteList.filterInput != "ab" {
		t.Errorf("filterInput = %q, want 'ab'", app.noteList.filterInput)
	}
}

func TestNoteListEmptyView(t *testing.T) {
	app := testNoteApp(nil)

	view := app.viewNoteList()
	if !strings.Contains(view, "No notes found") {
		t.Error("expected 'No notes found' in empty view")
	}
}

func TestNoteListEmptyFilterView(t *testing.T) {
	app := testNoteApp(testNotes())
	app.noteList.filter = "zzzznotexist"

	view := app.viewNoteList()
	if !strings.Contains(view, "No notes matching") {
		t.Error("expected 'No notes matching' in filtered empty view")
	}
}

func TestNoteListViewContent(t *testing.T) {
	app := testNoteApp(testNotes())

	view := app.viewNoteList()
	if !strings.Contains(view, "Notes") {
		t.Error("missing title bar")
	}
	if !strings.Contains(view, "Alpha Note") {
		t.Error("missing note title")
	}
	if !strings.Contains(view, "alpha-note") {
		t.Error("missing note slug")
	}
}

func TestNoteListBackToPicker(t *testing.T) {
	app := testNoteApp(testNotes())

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if app.mode != modePicker {
		t.Errorf("mode = %d, want modePicker (%d)", app.mode, modePicker)
	}
}

func TestNoteListEnterViewMode(t *testing.T) {
	app := testNoteApp(testNotes())

	app.updateNoteList(tea.KeyMsg{Type: tea.KeyEnter})
	if app.mode != modeNoteView {
		t.Errorf("mode = %d, want modeNoteView (%d)", app.mode, modeNoteView)
	}
	if app.noteView.note.ID != "n1" {
		t.Errorf("noteView.note.ID = %q, want 'n1'", app.noteView.note.ID)
	}
}

func TestNoteViewContent(t *testing.T) {
	app := &App{
		mode: modeNoteView,
		noteView: noteViewModel{
			note: &model.Note{
				ID:        "n1",
				Title:     "Test Note",
				Slug:      "test-note",
				Body:      "This is the body content",
				Tags:      "go,testing",
				UpdatedAt: time.Now(),
			},
		},
		width:  80,
		height: 30,
	}

	view := app.viewNoteDetail()
	if !strings.Contains(view, "Test Note") {
		t.Error("missing note title in view")
	}
	if !strings.Contains(view, "test-note") {
		t.Error("missing slug in view")
	}
	if !strings.Contains(view, "This is the body content") {
		t.Error("missing body in view")
	}
	if !strings.Contains(view, "go,testing") {
		t.Error("missing tags in view")
	}
}

func TestNoteViewEmptyBody(t *testing.T) {
	app := &App{
		mode: modeNoteView,
		noteView: noteViewModel{
			note: &model.Note{
				ID:        "n1",
				Title:     "Empty",
				Slug:      "empty",
				UpdatedAt: time.Now(),
			},
		},
		width:  80,
		height: 30,
	}

	view := app.viewNoteDetail()
	if !strings.Contains(view, "(empty note)") {
		t.Error("expected empty note indicator")
	}
}

func TestNoteViewWithBacklinks(t *testing.T) {
	app := &App{
		mode: modeNoteView,
		noteView: noteViewModel{
			note: &model.Note{
				ID:        "n1",
				Title:     "Target",
				Slug:      "target",
				Body:      "Some content",
				UpdatedAt: time.Now(),
			},
			backlinks: []backlinkDisplay{
				{label: "[[source-note]] Source Note", context: "mentions target"},
			},
		},
		width:  80,
		height: 30,
	}

	view := app.viewNoteDetail()
	if !strings.Contains(view, "Backlinks (1)") {
		t.Error("missing backlinks header")
	}
	if !strings.Contains(view, "mentions target") {
		t.Error("missing backlink context")
	}
}

func TestNoteViewScroll(t *testing.T) {
	app := &App{
		mode: modeNoteView,
		noteView: noteViewModel{
			note: &model.Note{
				ID:        "n1",
				Title:     "Scrollable",
				Slug:      "scrollable",
				Body:      strings.Repeat("Line content\n", 50),
				UpdatedAt: time.Now(),
			},
		},
		width:  80,
		height: 20,
	}

	if app.noteView.scroll != 0 {
		t.Fatalf("initial scroll = %d, want 0", app.noteView.scroll)
	}

	app.updateNoteView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.noteView.scroll != 1 {
		t.Errorf("after j: scroll = %d, want 1", app.noteView.scroll)
	}

	app.updateNoteView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.noteView.scroll != 0 {
		t.Errorf("after k: scroll = %d, want 0", app.noteView.scroll)
	}

	// k at 0 should stay at 0
	app.updateNoteView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if app.noteView.scroll != 0 {
		t.Errorf("k at 0: scroll = %d, want 0", app.noteView.scroll)
	}
}

func TestBoardSwitchToNotes(t *testing.T) {
	app := testApp(testColumns(), testCards())
	app.mode = modeBoard

	app.updateBoard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if app.mode != modeNotes {
		t.Errorf("mode = %d, want modeNotes (%d)", app.mode, modeNotes)
	}
}

func TestPickerSwitchToNotes(t *testing.T) {
	app := &App{
		mode: modePicker,
		picker: pickerModel{
			boards: []*model.Board{
				{ID: "b1", Name: "Board 1"},
			},
		},
		width:  80,
		height: 24,
	}

	app.updatePicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	if app.mode != modeNotes {
		t.Errorf("mode = %d, want modeNotes (%d)", app.mode, modeNotes)
	}
}

func TestNoteViewBackToList(t *testing.T) {
	app := &App{
		mode: modeNoteView,
		noteView: noteViewModel{
			note: &model.Note{
				ID:        "n1",
				Title:     "Test",
				Slug:      "test",
				UpdatedAt: time.Now(),
			},
		},
		width:  80,
		height: 24,
	}

	app.updateNoteView(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if app.mode != modeNotes {
		t.Errorf("mode = %d, want modeNotes (%d)", app.mode, modeNotes)
	}
}
