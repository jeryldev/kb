package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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
