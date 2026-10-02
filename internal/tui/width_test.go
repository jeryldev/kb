package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/model"
)

func TestNoteListRowsFitTheWidthWithoutBreakingCharacters(t *testing.T) {
	notes := []*model.Note{{
		ID: "n1", Title: "日本語のとても長いタイトル — strategic foresight and scenario analysis notes",
		Slug: "very-long-slug-for-a-very-long-title", Tags: "strategy,foresight,innovation", UpdatedAt: time.Now(),
	}}
	app := testNoteApp(notes)
	app.width = 40
	for _, line := range strings.Split(app.viewNoteList(), "\n") {
		if !utf8.ValidString(line) {
			t.Errorf("broken UTF-8: %q", line)
		}
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("line is %d cells wide: %q", w, line)
		}
	}
}

func TestWorkspaceRowsFitTheWidth(t *testing.T) {
	app := &App{mode: modeWSContent, width: 50, height: 20}
	app.wsContent = wsContentModel{
		workspace: &model.Workspace{ID: "w", Name: "Default"},
		notes: []*model.Note{{
			ID: "n1", Title: "obsidian-cli.nvim - Wrapping obsidian-cli for Neovim",
			Slug: "obsidian-cli-nvim-linkedin-article", Tags: "neovim,obsidian,open-source,developer-tools",
		}},
	}
	for _, line := range strings.Split(app.viewWSContent(), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line is %d cells wide: %q", w, line)
		}
	}
}

// Text typed faster than kb reads it, or pasted, arrives as one key event
// holding several characters. Every text field must keep all of them.
func TestTextFieldsKeepFastTypingAndPastes(t *testing.T) {
	burst := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Q4 Of")}
	paste := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fsite"), Paste: true}

	ws := &App{mode: modeWSContent}
	ws.wsContent.creating = "note"
	ws.updateWSContentCreating(burst)
	ws.updateWSContentCreating(paste)
	if ws.wsContent.input != "Q4 Offsite" {
		t.Errorf("new note title = %q", ws.wsContent.input)
	}

	board := testApp(testColumns(), testCards())
	board.board.filtering = true
	board.updateBoardFiltering(burst)
	board.updateBoardFiltering(paste)
	if board.board.filterInput != "Q4 Offsite" {
		t.Errorf("board filter = %q", board.board.filterInput)
	}

	notes := testNoteApp(testNotes())
	notes.noteList.filtering = true
	notes.updateNoteList(burst)
	notes.updateNoteList(paste)
	if notes.noteList.filterInput != "Q4 Offsite" {
		t.Errorf("note filter = %q", notes.noteList.filterInput)
	}
}
