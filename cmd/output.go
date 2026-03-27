package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

var jsonOutput bool

type boardJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	WorkspaceID string `json:"workspace_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type cardJSON struct {
	ID          string `json:"id"`
	Column      string `json:"column"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Labels      string `json:"labels"`
	ExternalID  string `json:"external_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type columnJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit"`
	Cards    int    `json:"cards"`
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func toBoardJSON(b *model.Board) boardJSON {
	return boardJSON{
		ID:          b.ID,
		Name:        b.Name,
		Description: b.Description,
		WorkspaceID: b.WorkspaceID,
		CreatedAt:   formatTime(b.CreatedAt),
		UpdatedAt:   formatTime(b.UpdatedAt),
	}
}

func toCardJSON(c *model.Card, colName string) cardJSON {
	return cardJSON{
		ID:          c.ID,
		Column:      colName,
		Title:       c.Title,
		Description: c.Description,
		Priority:    string(c.Priority),
		Labels:      c.Labels,
		ExternalID:  c.ExternalID,
		CreatedAt:   formatTime(c.CreatedAt),
		UpdatedAt:   formatTime(c.UpdatedAt),
	}
}

func toColumnJSON(col *model.Column, cardCount int) columnJSON {
	return columnJSON{
		ID:       col.ID,
		Name:     col.Name,
		Position: col.Position,
		WIPLimit: col.WIPLimit,
		Cards:    cardCount,
	}
}

type workspaceJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Position    int    `json:"position"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toWorkspaceJSON(ws *model.Workspace) workspaceJSON {
	return workspaceJSON{
		ID:          ws.ID,
		Name:        ws.Name,
		Kind:        string(ws.Kind),
		Description: ws.Description,
		Path:        ws.Path,
		Position:    ws.Position,
		CreatedAt:   formatTime(ws.CreatedAt),
		UpdatedAt:   formatTime(ws.UpdatedAt),
	}
}

type noteJSON struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Body        string `json:"body"`
	Tags        string `json:"tags"`
	Pinned      bool   `json:"pinned"`
	WorkspaceID string `json:"workspace_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type backlinkJSON struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Context    string `json:"context"`
}

func toNoteJSON(n *model.Note) noteJSON {
	return noteJSON{
		ID:          n.ID,
		Title:       n.Title,
		Slug:        n.Slug,
		Body:        n.Body,
		Tags:        n.Tags,
		Pinned:      n.Pinned,
		WorkspaceID: n.WorkspaceID,
		CreatedAt:   formatTime(n.CreatedAt),
		UpdatedAt:   formatTime(n.UpdatedAt),
	}
}

type publishTargetJSON struct {
	ID          string  `json:"id"`
	WorkspaceID *string `json:"workspace_id"`
	Name        string  `json:"name"`
	Engine      string  `json:"engine"`
	BasePath    string  `json:"base_path"`
	PostsDir    string  `json:"posts_dir"`
	CreatedAt   string  `json:"created_at"`
}

func toPublishTargetJSON(pt *model.PublishTarget) publishTargetJSON {
	return publishTargetJSON{
		ID:          pt.ID,
		WorkspaceID: pt.WorkspaceID,
		Name:        pt.Name,
		Engine:      string(pt.Engine),
		BasePath:    pt.BasePath,
		PostsDir:    pt.PostsDir,
		CreatedAt:   formatTime(pt.CreatedAt),
	}
}

type publishLogJSON struct {
	ID          string `json:"id"`
	NoteSlug    string `json:"note_slug"`
	TargetID    string `json:"target_id"`
	FilePath    string `json:"file_path"`
	PublishedAt string `json:"published_at"`
}

func toPublishLogJSON(pl *model.PublishLog, noteSlug string) publishLogJSON {
	return publishLogJSON{
		ID:          pl.ID,
		NoteSlug:    noteSlug,
		TargetID:    pl.TargetID,
		FilePath:    pl.FilePath,
		PublishedAt: formatTime(pl.PublishedAt),
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(rootCmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func resolveColumnByName(boardID, name string) (*model.Column, error) {
	columns, err := db.ListColumns(boardID)
	if err != nil {
		return nil, err
	}
	for _, col := range columns {
		if strings.EqualFold(col.Name, name) {
			return col, nil
		}
	}
	return nil, fmt.Errorf("column %q not found", name)
}
