// Package editor runs the user's text editor on a file, the way git and
// other terminal tools do: $VISUAL, then $EDITOR, then the first of nvim,
// vim, vi or nano found on PATH.
package editor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var fallbacks = []string{"nvim", "vim", "vi", "nano"}

// Command returns the editor command for path, wired to the terminal.
func Command(path string) (*exec.Cmd, error) {
	argv, err := argv()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], append(argv[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd, nil
}

// Name is the editor's program name, for hints like "e: edit (nvim)", or ""
// when there is no editor.
func Name() string {
	argv, err := argv()
	if err != nil {
		return ""
	}
	return filepath.Base(argv[0])
}

func argv() ([]string, error) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			words, err := Split(value)
			if err != nil {
				return nil, fmt.Errorf("$%s: %w", env, err)
			}
			if len(words) == 0 || words[0] == "" {
				return nil, fmt.Errorf("$%s names no program", env)
			}
			return words, nil
		}
	}
	for _, name := range fallbacks {
		if path, err := exec.LookPath(name); err == nil {
			return []string{path}, nil
		}
	}
	return nil, errors.New("no editor found: set $EDITOR (for example, export EDITOR=nvim)")
}

// Split breaks an $EDITOR value into words as a POSIX shell would for the
// common cases: whitespace separates words, single quotes are literal,
// double quotes group, and a backslash escapes the next character, except
// inside double quotes where it escapes only $, `, " and \ (so a Windows
// style "C:\Tools" keeps its backslashes). Variables and globs are not
// expanded.
func Split(s string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			if quote == '"' && !strings.ContainsRune("$`\"\\\n", r) {
				word.WriteRune('\\')
			}
			word.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unterminated quote or escape in %q", s)
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}
