package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/jeryldev/kb/internal/editor"
	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

type noteViewModel struct {
	note       *model.Note
	backlinks  []fstore.Backlink
	scroll     int
	confirming bool
}

// noteEditedMsg arrives when the editor opened on a note exits.
type noteEditedMsg struct {
	noteID string
	err    error
}

// switchToNoteView shows a note as its file is now.
func (a *App) switchToNoteView(noteID string) {
	if !a.reload() {
		return
	}
	note, err := a.db.GetNote(noteID)
	if a.fail(err) {
		return
	}
	a.mode = modeNoteView
	scroll := 0
	if a.noteView.note != nil && a.noteView.note.ID == noteID {
		scroll = a.noteView.scroll
	}
	a.noteView = noteViewModel{note: note, backlinks: a.db.Backlinks(noteID), scroll: scroll}
}

func (a *App) updateNoteView(msg tea.Msg) (tea.Model, tea.Cmd) {
	v := &a.noteView
	switch msg := msg.(type) {
	case noteEditedMsg:
		if !a.reload() {
			return a, nil
		}
		if _, err := a.db.GetNote(msg.noteID); errors.Is(err, fstore.ErrNotFound) {
			a.backToWorkspace()
			a.feedback = "The note's file is gone"
			return a, nil
		}
		a.switchToNoteView(msg.noteID)
		if msg.err != nil {
			a.err = fmt.Errorf("editor: %w", msg.err)
		}

	case tea.KeyMsg:
		if v.confirming {
			v.confirming = false
			if isYes(msg) {
				dest, err := a.db.TrashNote(v.note.ID)
				if a.fail(err) {
					return a, nil
				}
				a.backToWorkspace()
				a.feedback = "Moved to " + dest
			}
			return a, nil
		}
		switch msg.String() {
		case "q":
			return a, tea.Quit
		case "b", "esc":
			a.backToWorkspace()
		case "e":
			return a, a.editNoteExternal()
		case "d":
			v.confirming = true
		case "j", "down":
			v.scroll++
		case "k", "up":
			v.scroll = max(0, v.scroll-1)
		}
	}
	return a, nil
}

// editorName is looked up once: with no $EDITOR it searches PATH, which is
// too slow to repeat on every render.
var editorName = sync.OnceValue(editor.Name)

// editNoteExternal opens the note's own file in the user's editor. The
// file is the note, so there is nothing to copy back: when the editor
// exits, kb reads the file again.
func (a *App) editNoteExternal() tea.Cmd {
	note := a.noteView.note
	cmd, err := a.noteEditorCommand(note)
	if a.fail(err) {
		return nil
	}
	id := note.ID
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return noteEditedMsg{noteID: id, err: err} })
}

func (a *App) noteEditorCommand(note *model.Note) (*exec.Cmd, error) {
	path, err := a.db.Vault().Abs(note.Path)
	if err != nil {
		return nil, err
	}
	return editor.Command(path)
}

func (a *App) viewNoteDetail() string {
	v := &a.noteView
	note := v.note
	editHint := "e: edit"
	if name := editorName(); name != "" {
		editHint = fmt.Sprintf("e: edit (%s)", name)
	}
	hints := fmt.Sprintf(" j/k: scroll   %s   d: delete   b: back   q: quit", editHint)
	return a.frame(" "+note.Title+" ", hints, nil, func(w, h int) string {
		if v.confirming {
			return renderCenteredConfirm(w, h, fmt.Sprintf("Move note %q to the trash?", note.Title))
		}
		contentW := max(20, w-4)
		meta := helpStyle.Render(fmt.Sprintf("%s   Updated: %s", note.Path, relativeTime(note.UpdatedAt)))
		if tags := note.TagList(); len(tags) > 0 {
			meta += "   " + labelStyle.Render("#"+strings.Join(tags, " #"))
		}
		sections := []string{ansi.Truncate(meta, contentW, "…"), ""}
		if a.db.NotDownloaded(note.ID) {
			sections = append(sections, emptyColumnStyle.Render("(in iCloud and not downloaded yet; press e to open it, which downloads it)"))
		} else if note.Body != "" {
			sections = append(sections, lipgloss.NewStyle().Width(contentW).Render(note.Body))
		} else {
			sections = append(sections, emptyColumnStyle.Render("(empty note)"))
		}
		if len(v.backlinks) > 0 {
			sections = append(sections, "", lipgloss.NewStyle().Bold(true).Underline(true).Render(fmt.Sprintf("Backlinks (%d)", len(v.backlinks))))
			for _, bl := range v.backlinks {
				label := "[[" + bl.Title + "]]"
				if bl.SourceType == "card" {
					label = fmt.Sprintf("card %q on %s", bl.Title, bl.Board)
				}
				line := "  " + label
				if ctx := strings.TrimSpace(bl.Context); ctx != "" {
					line += "  " + helpStyle.Render("\""+ctx+"\"")
				}
				sections = append(sections, ansi.Truncate(line, contentW, "…"))
			}
		}
		lines := strings.Split(strings.Join(sections, "\n"), "\n")
		v.scroll = max(0, min(v.scroll, len(lines)-h))
		visible := lines[v.scroll:min(v.scroll+h, len(lines))]
		return "  " + strings.Join(visible, "\n  ")
	})
}
