package graph

import (
	"github.com/jeryldev/kb/internal/model"
)

type Node struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Slug        string `json:"slug,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	Connections int    `json:"connections"`
}

type Edge struct {
	Source  string `json:"source"`
	Target string `json:"target"`
	Context string `json:"context,omitempty"`
}

type GraphData struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type DataSource interface {
	ListNotes() ([]*model.Note, error)
	ListNotesByWorkspace(workspaceID string) ([]*model.Note, error)
	ListAllLinks() ([]*model.Link, error)
}

func BuildGraph(ds DataSource, workspaceID string) (*GraphData, error) {
	var notes []*model.Note
	var err error

	if workspaceID != "" {
		notes, err = ds.ListNotesByWorkspace(workspaceID)
	} else {
		notes, err = ds.ListNotes()
	}
	if err != nil {
		return nil, err
	}

	noteSet := make(map[string]*model.Note, len(notes))
	for _, n := range notes {
		noteSet[n.ID] = n
	}

	allLinks, err := ds.ListAllLinks()
	if err != nil {
		return nil, err
	}

	connectionCount := make(map[string]int)
	var edges []Edge

	for _, link := range allLinks {
		if link.SourceType != "note" || link.TargetType != "note" {
			continue
		}
		_, sourceInSet := noteSet[link.SourceID]
		_, targetInSet := noteSet[link.TargetID]

		if !sourceInSet || !targetInSet {
			continue
		}

		edges = append(edges, Edge{
			Source:  link.SourceID,
			Target:  link.TargetID,
			Context: link.Context,
		})
		connectionCount[link.SourceID]++
		connectionCount[link.TargetID]++
	}

	nodes := make([]Node, 0, len(notes))
	for _, n := range notes {
		wsID := ""
		if n.WorkspaceID != nil {
			wsID = *n.WorkspaceID
		}
		nodes = append(nodes, Node{
			ID:          n.ID,
			Label:       n.Title,
			Type:        "note",
			Slug:        n.Slug,
			WorkspaceID: wsID,
			Connections: connectionCount[n.ID],
		})
	}

	if edges == nil {
		edges = []Edge{}
	}

	return &GraphData{Nodes: nodes, Edges: edges}, nil
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
