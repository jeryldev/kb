package graph

import (
	"sort"

	"github.com/jeryldev/kb/internal/model"
)

type Node struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"` // "note", "board" or "card"
	Slug        string `json:"slug,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Connections int    `json:"connections"`
	// Outside marks a node from another workspace, shown because it links
	// to (or from) the workspace the graph is for.
	Outside bool `json:"outside,omitempty"`
}

type Edge struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Context string `json:"context,omitempty"`
}

type GraphData struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Item is a board or a card in the graph.
type Item struct {
	ID          string
	Label       string
	WorkspaceID string
}

// Link is one link between two of the graph's notes, boards or cards.
type Link struct {
	SourceType, SourceID string
	TargetType, TargetID string
	Context              string
}

// Source is what the graph is built from.
type Source struct {
	Notes  []*model.Note
	Boards []Item
	Cards  []Item
	Links  []Link
}

// Build makes the graph. With a workspace, it holds that workspace's notes,
// boards and cards, plus anything elsewhere they link with (marked
// Outside), so a note linked from another workspace is no orphan.
func Build(src Source, workspaceID string) *GraphData {
	all := map[string]Node{}
	for _, n := range src.Notes {
		all[n.ID] = Node{ID: n.ID, Label: n.Title, Type: "note", Slug: n.Slug, WorkspaceID: n.WorkspaceID}
	}
	for _, b := range src.Boards {
		all[b.ID] = Node{ID: b.ID, Label: b.Label, Type: "board", WorkspaceID: b.WorkspaceID}
	}
	for _, c := range src.Cards {
		all[c.ID] = Node{ID: c.ID, Label: c.Label, Type: "card", WorkspaceID: c.WorkspaceID}
	}
	inScope := func(id string) bool {
		n, ok := all[id]
		return ok && (workspaceID == "" || n.WorkspaceID == workspaceID)
	}

	included := map[string]bool{}
	for id := range all {
		if inScope(id) && all[id].Type != "card" {
			included[id] = true
		}
	}
	g := &GraphData{Nodes: []Node{}, Edges: []Edge{}}
	connections := map[string]int{}
	for _, l := range src.Links {
		_, okS := all[l.SourceID]
		_, okT := all[l.TargetID]
		if !okS || !okT || (!inScope(l.SourceID) && !inScope(l.TargetID)) {
			continue
		}
		included[l.SourceID], included[l.TargetID] = true, true
		g.Edges = append(g.Edges, Edge{Source: l.SourceID, Target: l.TargetID, Context: l.Context})
		connections[l.SourceID]++
		connections[l.TargetID]++
	}
	for id := range included {
		n := all[id]
		n.Connections = connections[id]
		n.Outside = workspaceID != "" && n.WorkspaceID != workspaceID
		g.Nodes = append(g.Nodes, n)
	}
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Type != g.Nodes[j].Type {
			return g.Nodes[i].Type > g.Nodes[j].Type // notes, then cards, then boards
		}
		return g.Nodes[i].Label < g.Nodes[j].Label
	})
	return g
}

func (g *GraphData) Stats() (nodes, edges, orphans int) {
	nodes = len(g.Nodes)
	edges = len(g.Edges)
	for _, n := range g.Nodes {
		if n.Connections == 0 {
			orphans++
		}
	}
	return
}
