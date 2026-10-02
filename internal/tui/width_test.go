package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/model"
)

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
}
