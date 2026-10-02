package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The corpus in testdata was produced by running the Obsidian Kanban
// plugin's own parser and writer over testdata/src (see
// tools/kanban-compat). plugin/<name>.md is the plugin's save of a source
// board, and the .json files are the lanes and cards the plugin reads. kb
// must read every board exactly as the plugin does, and write back an
// unchanged board byte for byte.

type pluginItem struct {
	TitleRaw  string   `json:"titleRaw"`
	CheckChar string   `json:"checkChar"`
	BlockID   *string  `json:"blockId"`
	Tags      []string `json:"tags"`
}

type pluginReading struct {
	Frontmatter map[string]any `json:"frontmatter"`
	Lanes       []struct {
		Title    string       `json:"title"`
		MaxItems int          `json:"maxItems"`
		Complete bool         `json:"complete"`
		Items    []pluginItem `json:"items"`
	} `json:"lanes"`
	Archive []pluginItem `json:"archive"`
}

func readJSON(t *testing.T, path string) pluginReading {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b pluginReading
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return b
}

func compareItems(t *testing.T, where string, got []*Item, want []pluginItem) {
	t.Helper()
	if len(got) != len(want) {
		var titles []string
		for _, it := range got {
			titles = append(titles, it.RawText())
		}
		t.Errorf("%s: %d cards %q, plugin reads %d", where, len(got), titles, len(want))
		return
	}
	for i, w := range want {
		g := got[i]
		if g.RawText() != w.TitleRaw {
			t.Errorf("%s card %d: text %q, plugin reads %q", where, i, g.RawText(), w.TitleRaw)
		}
		if string(g.Check) != w.CheckChar {
			t.Errorf("%s card %d: check %q, plugin reads %q", where, i, string(g.Check), w.CheckChar)
		}
		if w.BlockID != nil && g.ID != *w.BlockID {
			t.Errorf("%s card %d: id %q, plugin reads %q", where, i, g.ID, *w.BlockID)
		}
		var tags []string
		for _, tag := range g.Tags {
			tags = append(tags, "#"+tag)
		}
		wantTags := append([]string{}, w.Tags...)
		sort.Strings(tags)
		sort.Strings(wantTags)
		if strings.Join(tags, " ") != strings.Join(wantTags, " ") {
			t.Errorf("%s card %d: tags %v, plugin reads %v", where, i, tags, wantTags)
		}
	}
}

func compareBoard(t *testing.T, name string, b *Board, want pluginReading) {
	t.Helper()
	if len(b.Lanes) != len(want.Lanes) {
		var titles []string
		for _, l := range b.Lanes {
			titles = append(titles, l.Title)
		}
		t.Errorf("%s: lanes %q, plugin reads %d lanes", name, titles, len(want.Lanes))
		return
	}
	for i, wl := range want.Lanes {
		l := b.Lanes[i]
		where := name + " lane " + wl.Title
		if l.Title != wl.Title || l.MaxItems != wl.MaxItems || l.Complete != wl.Complete {
			t.Errorf("%s: (%q, max %d, complete %v), plugin reads (%q, %d, %v)",
				where, l.Title, l.MaxItems, l.Complete, wl.Title, wl.MaxItems, wl.Complete)
		}
		compareItems(t, where, l.Items(), wl.Items)
	}
	var archived []*Item
	if b.Archive != nil {
		archived = b.Archive.Items()
	}
	compareItems(t, name+" archive", archived, want.Archive)
}

func corpus(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob("testdata/src/*.md")
	if err != nil || len(names) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	for i, n := range names {
		names[i] = strings.TrimSuffix(filepath.Base(n), ".md")
	}
	return names
}

func TestKbReadsSourceBoardsLikeThePlugin(t *testing.T) {
	for _, name := range corpus(t) {
		data, _ := os.ReadFile("testdata/src/" + name + ".md")
		b, err := Parse(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		compareBoard(t, name, b, readJSON(t, "testdata/plugin/"+name+".src.json"))
	}
}

func TestKbReadsPluginSavedBoardsLikeThePlugin(t *testing.T) {
	for _, name := range corpus(t) {
		data, _ := os.ReadFile("testdata/plugin/" + name + ".md")
		b, err := Parse(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		compareBoard(t, name, b, readJSON(t, "testdata/plugin/"+name+".json"))
	}
}

func TestEveryCorpusBoardRoundTripsByteForByte(t *testing.T) {
	for _, dir := range []string{"src", "plugin"} {
		for _, name := range corpus(t) {
			path := "testdata/" + dir + "/" + name + ".md"
			data, _ := os.ReadFile(path)
			b, err := Parse(data)
			if err != nil {
				t.Errorf("%s: %v", path, err)
				continue
			}
			if got := string(b.Render()); got != string(data) {
				t.Errorf("%s changed on a no-op round trip", path)
			}
		}
	}
}
