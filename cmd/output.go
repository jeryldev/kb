package cmd

import (
	"encoding/json"
	"time"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
)

// The --json shapes. Lists are never null: an empty list prints [].

type boardJSON struct {
	ID          string `json:"id"` // the board's vault path
	Name        string `json:"name"`
	Description string `json:"description"`
	Workspace   string `json:"workspace"`
}

type cardJSON struct {
	ID          string   `json:"id"`
	Board       string   `json:"board"`
	Column      string   `json:"column"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
	ExternalID  string   `json:"external_id"`
	Archived    bool     `json:"archived"`
}

type columnJSON struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
	WIPLimit *int   `json:"wip_limit"`
	Cards    int    `json:"cards"`
}

type workspaceJSON struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Position    int    `json:"position"`
}

type noteJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Path        string   `json:"path"`
	Body        string   `json:"body"`
	Tags        []string `json:"tags"`
	Pinned      bool     `json:"pinned"`
	Workspace   string   `json:"workspace"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	WorkspaceID string   `json:"workspace_id"`
}

type backlinkJSON struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Title      string `json:"title"`
	Slug       string `json:"slug,omitempty"`
	Board      string `json:"board,omitempty"`
	Context    string `json:"context"`
}

type publishTargetJSON struct {
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	BasePath  string `json:"base_path"`
	PostsDir  string `json:"posts_dir"`
	Permalink string `json:"permalink"`
	Workspace string `json:"workspace"`
}

type publicationJSON struct {
	Note     string `json:"note"` // the note's slug
	Target   string `json:"target"`
	FilePath string `json:"file_path"`
	Draft    bool   `json:"draft"`
}

type tagJSON struct {
	Tag   string `json:"tag"`
	Notes int    `json:"notes"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func orEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

func workspaceName(id string) string {
	if ws, err := db.GetWorkspace(id); err == nil {
		return ws.Name
	}
	return ""
}

func toBoardJSON(b *model.Board) boardJSON {
	return boardJSON{ID: b.ID, Name: b.Name, Description: b.Description, Workspace: workspaceName(b.WorkspaceID)}
}

func toCardJSON(c *model.Card) cardJSON {
	return cardJSON{
		ID:          c.ID,
		Board:       boardNameOf(c.BoardID),
		Column:      c.ColumnID,
		Title:       c.Title,
		Description: c.Description,
		Priority:    string(c.Priority),
		Labels:      orEmpty(c.LabelList()),
		ExternalID:  c.ExternalID,
		Archived:    c.ArchivedAt != nil,
	}
}

func boardNameOf(id string) string {
	if b, err := db.GetBoard(id); err == nil {
		return b.Name
	}
	return id
}

func toColumnJSON(l fstore.Lane) columnJSON {
	return columnJSON{Name: l.Name, Position: l.Position, WIPLimit: l.WIPLimit, Cards: l.Cards}
}

func toWorkspaceJSON(ws *model.Workspace) workspaceJSON {
	return workspaceJSON{ID: ws.ID, Name: ws.Name, Kind: string(ws.Kind), Description: ws.Description, Path: ws.Path, Position: ws.Position}
}

func toNoteJSON(n *model.Note) noteJSON {
	return noteJSON{
		ID:          n.ID,
		Title:       n.Title,
		Slug:        n.Slug,
		Path:        n.Path,
		Body:        n.Body,
		Tags:        orEmpty(n.TagList()),
		Pinned:      n.Pinned,
		Workspace:   workspaceName(n.WorkspaceID),
		WorkspaceID: n.WorkspaceID,
		CreatedAt:   formatTime(n.CreatedAt),
		UpdatedAt:   formatTime(n.UpdatedAt),
	}
}

func toPublishTargetJSON(pt *model.PublishTarget) publishTargetJSON {
	out := publishTargetJSON{Name: pt.Name, Engine: string(pt.Engine), BasePath: pt.BasePath, PostsDir: pt.PostsDir, Permalink: pt.Permalink}
	if pt.WorkspaceID != nil {
		out.Workspace = workspaceName(*pt.WorkspaceID)
	}
	return out
}

func printJSON(v any) error {
	enc := json.NewEncoder(rootCmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// jsonList prints a list, as [] when it is empty.
func jsonList[T, J any](items []T, conv func(T) J) error {
	out := make([]J, 0, len(items))
	for _, it := range items {
		out = append(out, conv(it))
	}
	return printJSON(out)
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
