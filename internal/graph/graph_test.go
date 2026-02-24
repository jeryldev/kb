package graph

import (
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

type mockDataSource struct {
	notes []*model.Note
	links []*model.Link
}

func (m *mockDataSource) ListNotes() ([]*model.Note, error) {
	return m.notes, nil
}

func (m *mockDataSource) ListNotesByWorkspace(workspaceID string) ([]*model.Note, error) {
	var filtered []*model.Note
	for _, n := range m.notes {
		if n.WorkspaceID != nil && *n.WorkspaceID == workspaceID {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

func (m *mockDataSource) ListAllLinks() ([]*model.Link, error) {
	return m.links, nil
}

func TestBuildGraphBasic(t *testing.T) {
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "Note A", Slug: "note-a"},
			{ID: "n2", Title: "Note B", Slug: "note-b"},
			{ID: "n3", Title: "Note C", Slug: "note-c"},
		},
		links: []*model.Link{
			{ID: "l1", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2", CreatedAt: time.Now()},
			{ID: "l2", SourceType: "note", SourceID: "n2", TargetType: "note", TargetID: "n3", CreatedAt: time.Now()},
		},
	}

	g, err := BuildGraph(ds, "")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(g.Nodes))
	}
	if len(g.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(g.Edges))
	}
}

func TestBuildGraphConnectionCount(t *testing.T) {
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "Hub", Slug: "hub"},
			{ID: "n2", Title: "Spoke 1", Slug: "spoke-1"},
			{ID: "n3", Title: "Spoke 2", Slug: "spoke-2"},
		},
		links: []*model.Link{
			{ID: "l1", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2"},
			{ID: "l2", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n3"},
		},
	}

	g, err := BuildGraph(ds, "")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}

	for _, node := range g.Nodes {
		if node.ID == "n1" && node.Connections != 2 {
			t.Errorf("hub connections = %d, want 2", node.Connections)
		}
		if node.ID == "n2" && node.Connections != 1 {
			t.Errorf("spoke-1 connections = %d, want 1", node.Connections)
		}
	}
}

func TestBuildGraphOrphanNode(t *testing.T) {
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "Connected", Slug: "connected"},
			{ID: "n2", Title: "Also Connected", Slug: "also-connected"},
			{ID: "n3", Title: "Orphan", Slug: "orphan"},
		},
		links: []*model.Link{
			{ID: "l1", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2"},
		},
	}

	g, _ := BuildGraph(ds, "")
	nodes, edges, orphans := g.Stats()
	if nodes != 3 {
		t.Errorf("nodes = %d, want 3", nodes)
	}
	if edges != 1 {
		t.Errorf("edges = %d, want 1", edges)
	}
	if orphans != 1 {
		t.Errorf("orphans = %d, want 1", orphans)
	}
}

func TestBuildGraphIgnoresNonNoteLinks(t *testing.T) {
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "Note", Slug: "note"},
		},
		links: []*model.Link{
			{ID: "l1", SourceType: "note", SourceID: "n1", TargetType: "card", TargetID: "c1"},
			{ID: "l2", SourceType: "note", SourceID: "n1", TargetType: "url", TargetID: "https://example.com"},
		},
	}

	g, _ := BuildGraph(ds, "")
	if len(g.Edges) != 0 {
		t.Errorf("edges = %d, want 0 (non-note links ignored)", len(g.Edges))
	}
}

func TestBuildGraphWorkspaceScoped(t *testing.T) {
	ws1 := "ws-1"
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "In WS", Slug: "in-ws", WorkspaceID: &ws1},
			{ID: "n2", Title: "In WS 2", Slug: "in-ws-2", WorkspaceID: &ws1},
			{ID: "n3", Title: "No WS", Slug: "no-ws"},
		},
		links: []*model.Link{
			{ID: "l1", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n2"},
			{ID: "l2", SourceType: "note", SourceID: "n1", TargetType: "note", TargetID: "n3"},
		},
	}

	g, _ := BuildGraph(ds, "ws-1")
	if len(g.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2 (workspace scoped)", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Errorf("edges = %d, want 1 (cross-workspace link excluded)", len(g.Edges))
	}
}

func TestBuildGraphEmpty(t *testing.T) {
	ds := &mockDataSource{}

	g, err := BuildGraph(ds, "")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if len(g.Nodes) != 0 {
		t.Errorf("nodes = %d, want 0", len(g.Nodes))
	}
	if len(g.Edges) != 0 {
		t.Errorf("edges = %d, want 0", len(g.Edges))
	}
}

func TestBuildGraphNodeFields(t *testing.T) {
	ws := "ws-1"
	ds := &mockDataSource{
		notes: []*model.Note{
			{ID: "n1", Title: "My Note", Slug: "my-note", WorkspaceID: &ws},
		},
	}

	g, _ := BuildGraph(ds, "")
	if len(g.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(g.Nodes))
	}
	node := g.Nodes[0]
	if node.ID != "n1" {
		t.Errorf("ID = %q, want 'n1'", node.ID)
	}
	if node.Label != "My Note" {
		t.Errorf("Label = %q, want 'My Note'", node.Label)
	}
	if node.Slug != "my-note" {
		t.Errorf("Slug = %q, want 'my-note'", node.Slug)
	}
	if node.WorkspaceID != "ws-1" {
		t.Errorf("WorkspaceID = %q, want 'ws-1'", node.WorkspaceID)
	}
	if node.Type != "note" {
		t.Errorf("Type = %q, want 'note'", node.Type)
	}
}

func TestStatsAllOrphans(t *testing.T) {
	g := &GraphData{
		Nodes: []Node{
			{ID: "n1", Connections: 0},
			{ID: "n2", Connections: 0},
		},
		Edges: []Edge{},
	}

	nodes, edges, orphans := g.Stats()
	if nodes != 2 || edges != 0 || orphans != 2 {
		t.Errorf("Stats() = (%d, %d, %d), want (2, 0, 2)", nodes, edges, orphans)
	}
}
