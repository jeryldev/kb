package graph

import (
	"strings"
	"testing"
)

func TestGenerateHTMLBasic(t *testing.T) {
	data := &GraphData{
		Nodes: []Node{
			{ID: "n1", Label: "Note A", Type: "note", Connections: 1},
			{ID: "n2", Label: "Note B", Type: "note", Connections: 1},
		},
		Edges: []Edge{
			{Source: "n1", Target: "n2"},
		},
	}

	html, err := GenerateHTML(data, "Test Graph")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	checks := []struct {
		name    string
		substr  string
	}{
		{"doctype", "<!DOCTYPE html>"},
		{"title", "<title>Test Graph</title>"},
		{"d3 script", "d3js.org/d3.v7.min.js"},
		{"svg element", `<svg id="graph">`},
		{"node id n1", `"id":"n1"`},
		{"node id n2", `"id":"n2"`},
		{"source n1", `"source":"n1"`},
		{"target n2", `"target":"n2"`},
		{"stat-nodes", `stat-nodes`},
		{"stat-edges", `stat-edges`},
		{"stat-orphans", `stat-orphans`},
		{"closing html", "</html>"},
	}

	for _, c := range checks {
		if !strings.Contains(html, c.substr) {
			t.Errorf("missing %s: expected %q in output", c.name, c.substr)
		}
	}
}

func TestGenerateHTMLEmpty(t *testing.T) {
	data := &GraphData{
		Nodes: []Node{},
		Edges: []Edge{},
	}

	html, err := GenerateHTML(data, "Empty")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	if !strings.Contains(html, "<title>Empty</title>") {
		t.Error("missing title")
	}
	if !strings.Contains(html, "const nodes = []") {
		t.Error("expected empty nodes array")
	}
	if !strings.Contains(html, "const links = []") {
		t.Error("expected empty links array")
	}
}

func TestGenerateHTMLTitleEscaping(t *testing.T) {
	data := &GraphData{Nodes: []Node{}, Edges: []Edge{}}

	html, err := GenerateHTML(data, "My KB Graph")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}
	if !strings.Contains(html, "<title>My KB Graph</title>") {
		t.Error("title not rendered correctly")
	}
}

func TestGenerateHTMLNodeData(t *testing.T) {
	ws := "ws-1"
	data := &GraphData{
		Nodes: []Node{
			{ID: "n1", Label: "Hub", Type: "note", Slug: "hub", WorkspaceID: ws, Connections: 3},
		},
		Edges: []Edge{},
	}

	html, err := GenerateHTML(data, "Graph")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	checks := []string{
		`"label":"Hub"`,
		`"slug":"hub"`,
		`"workspace_id":"ws-1"`,
		`"connections":3`,
	}
	for _, s := range checks {
		if !strings.Contains(html, s) {
			t.Errorf("missing node field: %s", s)
		}
	}
}

func TestGenerateHTMLEdgeContext(t *testing.T) {
	data := &GraphData{
		Nodes: []Node{
			{ID: "n1", Label: "A", Type: "note"},
			{ID: "n2", Label: "B", Type: "note"},
		},
		Edges: []Edge{
			{Source: "n1", Target: "n2", Context: "related to"},
		},
	}

	html, err := GenerateHTML(data, "Graph")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}
	if !strings.Contains(html, `"context":"related to"`) {
		t.Error("missing edge context")
	}
}

func TestGenerateHTMLForceSimulation(t *testing.T) {
	data := &GraphData{Nodes: []Node{}, Edges: []Edge{}}

	html, err := GenerateHTML(data, "Graph")
	if err != nil {
		t.Fatalf("GenerateHTML: %v", err)
	}

	forces := []string{
		"d3.forceSimulation",
		"d3.forceLink",
		"d3.forceManyBody",
		"d3.forceCenter",
		"d3.forceCollide",
		"d3.zoom",
		"d3.drag",
	}
	for _, f := range forces {
		if !strings.Contains(html, f) {
			t.Errorf("missing D3 force: %s", f)
		}
	}
}
