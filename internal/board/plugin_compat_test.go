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
		missing(t, "plugin harness not built (tools/kanban-compat/build.sh)")
	}
	if _, err := exec.LookPath("node"); err != nil {
		missing(t, "node not installed")
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
		{"a star card moved into a dash lane, and back", "en", func(t *testing.T) *Board {
			b := mustParse(t, "---\nkanban-plugin: board\n---\n\n## Star\n\n* [ ] s1\n* [ ] s2\n\n## Dash\n\n- [ ] d1\n- [ ] d2\n\n## Ordered\n\n1. [ ] o1\n2. [ ] o2\n")
			s1 := b.Lanes[0].Items()[0]
			d1 := b.Lanes[1].Items()[0]
			o1 := b.Lanes[2].Items()[0]
			b.MoveBefore(s1.ID, "Dash", b.Lanes[1].Items()[1].ID)
			b.MoveBefore(d1.ID, "Star", "")
			b.MoveBefore(o1.ID, "Dash", "")
			b.MoveBefore(b.Lanes[1].Items()[0].ID, "Ordered", "")
			b.ArchiveItem(b.Lanes[0].Items()[0].ID)
			return b
		}},
		{"cards moved into indented and ordered lanes", "en", func(t *testing.T) *Board {
			src, _ := os.ReadFile("testdata/src/indented.md")
			b := mustParse(t, string(src))
			ord, _ := os.ReadFile("testdata/src/ordered.md")
			o := mustParse(t, string(ord))
			b.Move(b.Lanes[0].Items()[0].ID, "Done", 0)
			b.Add("Todo", "added under indented cards", "")
			o.Add("Next", "added to an ordered lane", "two\nlines")
			b.Lanes[0].Items()[2].SetTitle("tab marker, edited")
			return b
		}},
		{"a lane after the archive, edited", "en", func(t *testing.T) *Board {
			b := mustParse(t, "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] a ^abcd1234\n\n***\n\n## Archive\n\n- [x] old ^abcd5678\n\n## After\n\n- [ ] c ^abcd9999\n")
			b.Add("Todo", "new", "")
			b.Move("abcd9999", "Todo", 0)
			return b
		}},
		{"a setext lane renamed, twins", "en", func(t *testing.T) *Board {
			b := mustParse(t, "---\nkanban-plugin: board\n---\n\nTodo\n====\n\n- [ ] x\n- [ ] x\n- [ ] y\n\n## Done\n\n")
			b.RenameLane("Todo", "Next")
			b.Delete(b.Lanes[0].Items()[0].ID)
			b.Add("Done", "z", "")
			return b
		}},
		{"carets anywhere in card text", "en", func(t *testing.T) *Board {
			b := New([]string{"Todo"})
			b.Add("Todo", "mid caret", "see ^ref\nmore")
			b.Add("Todo", "no space", "e = mc^2")
			b.Add("Todo", "two in a word", "a^b^c")
			b.Add("Todo", "in code", "run `x ^y`")
			b.Add("Todo", "in a fence", "```\na ^b\n```")
			b.Add("Todo", "in a link", "see [[a^b]]")
			b.Add("Todo", "title mc^2", "")
			c, _ := b.Add("Todo", "edited later", "")
			c.SetDescription("first ^one\nlast^two")
			return b
		}},
		{"a description ending in a caret word", "en", func(t *testing.T) *Board {
			b := New([]string{"Todo"})
			c, _ := b.Add("Todo", "card", "see the note ^ref")
			c.SetPriority("high")
			b.Add("Todo", "one line ^not-an-id-either", "")
			return b
		}},
		{"a card added to a lane holding only text", "en", func(t *testing.T) *Board {
			b := mustParse(t, "---\nkanban-plugin: board\n---\n\n## Todo\n\nSome notes about this lane\n\n## Done\n\n**Complete**\nwrap-up text\n")
			b.Add("Todo", "new card", "")
			b.Add("Done", "finished", "")
			return b
		}},
		{"a CRLF board edited", "en", func(t *testing.T) *Board {
			src, _ := os.ReadFile("testdata/src/crlf.md")
			b := mustParse(t, string(src))
			b.Lanes[0].Items()[0].SetDescription("new desc")
			b.Move(b.Lanes[0].Items()[0].ID, "Done", 0)
			b.Add("Todo", "fresh", "")
			return b
		}},
		{"lanes named C# and Done ## edited", "en", func(t *testing.T) *Board {
			src, _ := os.ReadFile("testdata/src/heading-hash.md")
			b := mustParse(t, string(src))
			b.Add("C#", "added", "")
			b.Move(b.Lanes[1].Items()[0].ID, "C#", 0)
			if err := b.RenameLane("F#", "F# done"); err != nil {
				t.Fatal(err)
			}
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

// Adding a lane, or reordering so another lane is last, on a board with an
// archive keeps the archive an archive, for kb and for the plugin.
func TestLaneChangesKeepTheArchive(t *testing.T) {
	path := harness(t)
	start := New([]string{"Todo", "Doing", "Done"})
	it, _ := start.Add("Todo", "old card", "")
	start.ArchiveItem(it.ID)
	data := start.Render()

	for name, change := range map[string]func(*Board) error{
		"add":     func(b *Board) error { return b.AddLane("QA") },
		"reorder": func(b *Board) error { return b.ReorderLanes([]string{"Done", "Todo", "Doing"}) },
	} {
		b, err := Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := change(b); err != nil {
			t.Fatal(err)
		}
		out := b.Render()
		again, _ := Parse(out)
		if again.Archive == nil || len(again.Archive.Items()) != 1 || again.Lane("Archive") != nil {
			t.Errorf("%s: kb reads the archive wrong:\n%s", name, out)
		}
		var got struct {
			Lanes   []struct{ Title string }
			Archive []json.RawMessage
		}
		if err := json.Unmarshal(runHarness(t, path, "json", "en", out), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Archive) != 1 || len(got.Lanes) != len(again.Lanes) {
			t.Errorf("%s: the plugin reads %d lanes and %d archived cards:\n%s", name, len(got.Lanes), len(got.Archive), out)
		}
	}
}

// missing skips a test that needs a tool this machine lacks, except in CI
// (KB_CI set), where every test must run.
func missing(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("KB_CI") != "" {
		t.Fatalf("%s, and KB_CI is set", what)
	}
	t.Skip(what)
}
