package graph

import (
	"strings"
	"testing"

	"github.com/jeryldev/kb/internal/model"
)

func note(id, title, ws string) *model.Note {
	return &model.Note{ID: id, Title: title, Slug: strings.ToLower(title), WorkspaceID: ws}
}

func source() Source {
	return Source{
		Notes:  []*model.Note{note("n1", "Alpha", "w1"), note("n2", "Beta", "w1"), note("n3", "Gamma", "w2"), note("n4", "Lonely", "w1")},
		Boards: []Item{{ID: "Boards/Sprint.md", Label: "Sprint", WorkspaceID: "w1"}},
		Cards:  []Item{{ID: "Boards/Sprint.md#c1", Label: "Build it", WorkspaceID: "w1"}},
		Links: []Link{
			{SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2", Context: "a to b"},
			{SourceType: "note", SourceID: "n2", TargetType: "note", TargetID: "n3"},
			{SourceType: "note", SourceID: "n1", TargetType: "board", TargetID: "Boards/Sprint.md"},
			{SourceType: "card", SourceID: "Boards/Sprint.md#c1", TargetType: "note", TargetID: "n1"},
		},
	}
}

func nodeByID(g *GraphData, id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

func TestBuildIncludesNotesBoardsAndCards(t *testing.T) {
	g := Build(source(), "")
	if len(g.Nodes) != 6 || len(g.Edges) != 4 {
		t.Fatalf("nodes=%d edges=%d", len(g.Nodes), len(g.Edges))
	}
	if b := nodeByID(g, "Boards/Sprint.md"); b == nil || b.Type != "board" || b.Connections != 1 {
		t.Errorf("board node = %+v", b)
	}
	if c := nodeByID(g, "Boards/Sprint.md#c1"); c == nil || c.Type != "card" {
		t.Errorf("card node = %+v", c)
	}
	if a := nodeByID(g, "n1"); a.Connections != 3 {
		t.Errorf("Alpha connections = %d", a.Connections)
	}
	nodes, edges, orphans := g.Stats()
	if nodes != 6 || edges != 4 || orphans != 1 {
		t.Errorf("stats = %d %d %d", nodes, edges, orphans)
	}
}

func TestAWorkspaceGraphKeepsItsNeighbours(t *testing.T) {
	g := Build(source(), "w2")
	gamma := nodeByID(g, "n3")
	beta := nodeByID(g, "n2")
	if gamma == nil || gamma.Outside || gamma.Connections != 1 {
		t.Errorf("Gamma = %+v", gamma)
	}
	if beta == nil || !beta.Outside {
		t.Errorf("Beta, linked from another workspace, should be shown as outside: %+v", beta)
	}
	if nodeByID(g, "n4") != nil {
		t.Error("an unconnected note from another workspace should not appear")
	}
	if _, _, orphans := g.Stats(); orphans != 0 {
		t.Errorf("Gamma is linked, so not an orphan: %d", orphans)
	}
}

func TestBuildEmpty(t *testing.T) {
	g := Build(Source{}, "")
	if g.Nodes == nil || g.Edges == nil || len(g.Nodes)+len(g.Edges) != 0 {
		t.Errorf("empty graph = %+v", g)
	}
}

// A note linking to another twice is one edge, and one connection each.
func TestRepeatedLinksAreOneEdge(t *testing.T) {
	src := Source{
		Notes: []*model.Note{note("n1", "Alpha", ""), note("n2", "Beta", "")},
		Links: []Link{
			{SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2", Context: "first"},
			{SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2", Context: "second"},
		},
	}
	g := Build(src, "")
	if len(g.Edges) != 1 || g.Edges[0].Context != "first" {
		t.Errorf("edges = %+v", g.Edges)
	}
	if a := nodeByID(g, "n1"); a.Connections != 1 {
		t.Errorf("Alpha connections = %d", a.Connections)
	}
}

// Links both ways between two notes are one line; a self-link draws
// nothing; nodes come out in the same order every time.
func TestEdgesArePairsAndOrderIsStable(t *testing.T) {
	src := Source{
		Notes: []*model.Note{note("a", "Same", ""), note("b", "Same", ""), note("c", "Same", ""), note("d", "Lonely", "")},
		Links: []Link{
			{SourceType: "note", SourceID: "a", TargetType: "note", TargetID: "b"},
			{SourceType: "note", SourceID: "b", TargetType: "note", TargetID: "a"},
			{SourceType: "note", SourceID: "c", TargetType: "note", TargetID: "c"},
		},
	}
	g := Build(src, "")
	if len(g.Edges) != 1 {
		t.Errorf("edges = %+v", g.Edges)
	}
	if c := nodeByID(g, "c"); c.Connections != 0 {
		t.Errorf("a self-link counts %d connections", c.Connections)
	}
	first := ""
	for _, n := range g.Nodes {
		first += n.ID
	}
	for i := 0; i < 20; i++ {
		order := ""
		for _, n := range Build(src, "").Nodes {
			order += n.ID
		}
		if order != first {
			t.Fatalf("order %s, then %s", first, order)
		}
	}
}
