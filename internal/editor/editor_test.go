package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := map[string][]string{
		"nvim":                                {"nvim"},
		"code -w":                             {"code", "-w"},
		`"/Applications/My Ed/bin/ed" --wait`: {"/Applications/My Ed/bin/ed", "--wait"},
		`nvim -c 'set ft=markdown'`:           {"nvim", "-c", "set ft=markdown"},
		`emacs\ client -nw`:                   {"emacs client", "-nw"},
		"  spaced   out  ":                    {"spaced", "out"},
		`ed "C:\Tools\x"`:                     {"ed", `C:\Tools\x`},
		`ed "say \"hi\""`:                     {"ed", `say "hi"`},
	}
	for in, want := range cases {
		got, err := Split(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "''")
	if _, err := Command("x"); err == nil || !strings.Contains(err.Error(), "EDITOR") {
		t.Errorf("an empty quoted EDITOR should be a clear error, got %v", err)
	}
	if _, err := Split(`nvim "unclosed`); err == nil {
		t.Error("unterminated quote should be an error")
	}
}

func TestCommandUsesVisualThenEditorWithArgs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code -w")
	cmd, err := Command("/vault/note.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"code", "-w", "/vault/note.md"}; !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("args = %q", cmd.Args)
	}
	t.Setenv("VISUAL", "hx")
	cmd, _ = Command("/vault/note.md")
	if cmd.Args[0] != "hx" {
		t.Errorf("VISUAL ignored: %q", cmd.Args)
	}
}

func TestCommandFallsBackToAnInstalledEditor(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "vi")
	os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", dir)
	cmd, err := Command("n.md")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != fake {
		t.Errorf("path = %q", cmd.Path)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := Command("n.md"); err == nil {
		t.Error("no editor anywhere should be an error")
	}
}

func TestName(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "/opt/homebrew/bin/nvim --clean")
	if got := Name(); got != "nvim" {
		t.Errorf("Name() = %q", got)
	}
}
