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
