package board

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These tests hand boards kb wrote to the Obsidian Kanban plugin's own
// parser and writer (tools/kanban-compat/harness.js, built by build.sh) and
// check that the plugin reads the lanes and cards kb meant to write, and
// that a board the plugin then saves reads back the same in kb. They run
// when the harness has been built and node is installed, and are skipped
// otherwise.

func harness(t *testing.T) string {
	t.Helper()
	path, _ := filepath.Abs("../../tools/kanban-compat/harness.js")
	if _, err := os.Stat(path); err != nil {
		t.Skip("plugin harness not built (tools/kanban-compat/build.sh)")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	return path
}

func runHarness(t *testing.T, path, mode, lang string, input []byte) []byte {
	t.Helper()
	cmd := exec.Command("node", path, mode)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), "KB_LANG="+lang)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("harness %s: %v\n%s", mode, err, stderr.String())
	}
	return out
}

type scenario struct {
	name string
	lang string
	make func(t *testing.T) *Board
}

func scenarios() []scenario {
	return []scenario{
		{"new board with every kind of card", "en", func(t *testing.T) *Board {
			b := New([]string{"Todo", "Doing", "Done"})
			b.Lanes[1].MaxItems = 2
			a, _ := b.Add("Todo", "Plain card", "")
			c, _ := b.Add("Todo", "Fix #3 bug", "first line\n\nsecond paragraph\n- a list")
			c.SetLabels([]string{"some label", "ui"})
			c.SetPriority("urgent")
			d, _ := b.Add("Doing", "Linked to [[Some Note]]", "")
			d.SetField("ext", "linear:DEV-42")
			b.Move(a.ID, "Done", 0)
			b.ArchiveItem(d.ID)
			return b
		}},
		{"edits on a plugin board", "en", func(t *testing.T) *Board {
			b := mustParse(t, plugin(t, "basic"))
			for _, l := range b.Lanes {
				for _, it := range l.Items() {
					it.SetTitle(it.Title + " (edited)")
				}
			}
			b.Lanes[0].Items()[1].SetPriority("high")
			b.ArchiveItem(b.Lanes[1].Items()[0].ID)
			return b
		}},
		{"cards added to a star-bullet board", "en", func(t *testing.T) *Board {
			src, _ := os.ReadFile("testdata/src/bullets-star.md")
			b := mustParse(t, string(src))
			b.Add("Starred", "third", "with a description")
			return b
		}},
		{"lazy lines survive a move", "en", func(t *testing.T) *Board {
			src, _ := os.ReadFile("testdata/src/lazy.md")
			b := mustParse(t, string(src))
			b.Move(b.Lanes[0].Items()[0].ID, "Done", 0)
			return b
		}},
		{"archive created on a German board", "de", func(t *testing.T) *Board {
			b := mustParse(t, plugin(t, "german.de"))
			b.ArchiveItem(b.Lanes[0].Items()[0].ID)
			return b
		}},
	}
}

func TestThePluginReadsWhatKbWrites(t *testing.T) {
	path := harness(t)
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			intended := sc.make(t)
			written := intended.Render()
			mine := mustParse(t, string(written))

			// What kb meant to write: if kb and the plugin agree on a reading
			// that differs from this (a card swallowed by its neighbour, a
			// card the plugin ignores), the board was written wrong.
			if len(mine.Lanes) != len(intended.Lanes) {
				t.Fatalf("wrote %d lanes, reads back %d:\n%s", len(intended.Lanes), len(mine.Lanes), written)
			}
			for i, l := range intended.Lanes {
				compareCards(t, sc.name+" as written, lane "+l.Title, mine.Lanes[i].Items(), l.Items())
			}
			if intended.Archive != nil {
				if mine.Archive == nil {
					t.Fatalf("wrote an archive, reads back none:\n%s", written)
				}
				compareCards(t, sc.name+" as written, archive", mine.Archive.Items(), intended.Archive.Items())
			}

			var theirs pluginReading
			if err := json.Unmarshal(runHarness(t, path, "json", sc.lang, written), &theirs); err != nil {
				t.Fatal(err)
			}
			compareBoard(t, sc.name, mine, theirs)

			saved := runHarness(t, path, "roundtrip", sc.lang, written)
			again := mustParse(t, string(saved))
			if len(again.Lanes) != len(mine.Lanes) {
				t.Fatalf("plugin save changed the lanes:\n%s", saved)
			}
			for i, l := range mine.Lanes {
				compareCards(t, sc.name+" after a plugin save, lane "+l.Title, again.Lanes[i].Items(), l.Items())
			}
			if (mine.Archive == nil) != (again.Archive == nil) {
				t.Fatalf("plugin save changed the archive:\n%s", saved)
			}
			if mine.Archive != nil {
				compareCards(t, sc.name+" after a plugin save, archive", again.Archive.Items(), mine.Archive.Items())
			}
		})
	}
}

func compareCards(t *testing.T, where string, got, want []*Item) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d cards, want %d", where, len(got), len(want))
		return
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.ID != w.ID || g.Title != w.Title || g.Description != w.Description ||
			g.Priority() != w.Priority() || g.Field("ext") != w.Field("ext") ||
			string(g.Check) != string(w.Check) || len(g.Labels()) != len(w.Labels()) {
			t.Errorf("%s card %d:\n got  %+v\n want %+v", where, i, g, w)
		}
	}
}
