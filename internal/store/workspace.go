package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) CreateWorkspace(name string, kind model.WorkspaceKind, description, path string) (*model.Workspace, error) {
	if err := model.ValidateWorkspaceName(name); err != nil {
		return nil, err
	}

	var maxPos int
	err := d.conn.QueryRow("SELECT COALESCE(MAX(position), -1) FROM workspaces").Scan(&maxPos)
	if err != nil {
		return nil, fmt.Errorf("getting max position: %w", err)
	}

	now := time.Now().UTC()
	ws := &model.Workspace{
		ID:          uuid.New().String(),
		Name:        name,
		Kind:        kind,
		Description: description,
		Path:        path,
		Position:    maxPos + 1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	_, err = d.conn.Exec(
		`INSERT INTO workspaces (id, name, kind, description, path, position, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ws.ID, ws.Name, string(ws.Kind), ws.Description, ws.Path, ws.Position, ws.CreatedAt, ws.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("workspace %q already exists", name)
		}
		return nil, fmt.Errorf("inserting workspace: %w", err)
	}

	return ws, nil
}

func (d *DB) GetWorkspace(id string) (*model.Workspace, error) {
	ws := &model.Workspace{}
	var kind string
	err := d.conn.QueryRow(
		`SELECT id, name, kind, description, path, position, created_at, updated_at
		 FROM workspaces WHERE id = ?`,
		id,
	).Scan(&ws.ID, &ws.Name, &kind, &ws.Description, &ws.Path, &ws.Position, &ws.CreatedAt, &ws.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace: %w", err)
	}
	ws.Kind = model.WorkspaceKind(kind)
	return ws, nil
}

func (d *DB) GetWorkspaceByName(name string) (*model.Workspace, error) {
	ws := &model.Workspace{}
	var kind string
	err := d.conn.QueryRow(
		`SELECT id, name, kind, description, path, position, created_at, updated_at
		 FROM workspaces WHERE LOWER(name) = LOWER(?)`,
		name,
	).Scan(&ws.ID, &ws.Name, &kind, &ws.Description, &ws.Path, &ws.Position, &ws.CreatedAt, &ws.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace: %w", err)
	}
	ws.Kind = model.WorkspaceKind(kind)
	return ws, nil
}

func (d *DB) ListWorkspaces() ([]*model.Workspace, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, kind, description, path, position, created_at, updated_at
		 FROM workspaces ORDER BY position`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

func (d *DB) ListWorkspacesByKind(kind model.WorkspaceKind) ([]*model.Workspace, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, kind, description, path, position, created_at, updated_at
		 FROM workspaces WHERE kind = ? ORDER BY position`,
		string(kind),
	)
	if err != nil {
		return nil, fmt.Errorf("listing workspaces by kind: %w", err)
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

func (d *DB) UpdateWorkspace(ws *model.Workspace) error {
	if err := model.ValidateWorkspaceName(ws.Name); err != nil {
		return err
	}

	ws.UpdatedAt = time.Now().UTC()
	_, err := d.conn.Exec(
		`UPDATE workspaces SET name = ?, kind = ?, description = ?, path = ?, updated_at = ?
		 WHERE id = ?`,
		ws.Name, string(ws.Kind), ws.Description, ws.Path, ws.UpdatedAt, ws.ID,
	)
	if err != nil {
		return fmt.Errorf("updating workspace: %w", err)
	}
	return nil
}

func (d *DB) ArchiveWorkspace(id string) error {
	ws, err := d.GetWorkspace(id)
	if err != nil {
		return err
	}
	ws.Kind = model.KindArchive
	return d.UpdateWorkspace(ws)
}

func (d *DB) DeleteWorkspace(id string) error {
	result, err := d.conn.Exec("DELETE FROM workspaces WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting workspace: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("workspace not found")
	}
	return nil
}

func scanWorkspaces(rows *sql.Rows) ([]*model.Workspace, error) {
	var workspaces []*model.Workspace
	for rows.Next() {
		ws := &model.Workspace{}
		var kind string
		if err := rows.Scan(
			&ws.ID, &ws.Name, &kind, &ws.Description, &ws.Path, &ws.Position, &ws.CreatedAt, &ws.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning workspace: %w", err)
		}
		ws.Kind = model.WorkspaceKind(kind)
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}
