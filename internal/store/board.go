package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) CreateBoard(name, description, workspaceID string) (*model.Board, error) {
	if err := model.ValidateBoardName(name); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	board := &model.Board{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		WorkspaceID: workspaceID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO boards (id, name, description, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		board.ID, board.Name, board.Description, board.WorkspaceID, board.CreatedAt, board.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting board: %w", err)
	}

	for i, colName := range model.DefaultColumns {
		_, err = tx.Exec(
			"INSERT INTO columns (id, board_id, name, position) VALUES (?, ?, ?, ?)",
			uuid.New().String(), board.ID, colName, i,
		)
		if err != nil {
			return nil, fmt.Errorf("inserting default column %q: %w", colName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return board, nil
}

func (d *DB) GetBoard(id string) (*model.Board, error) {
	board := &model.Board{}
	var wsID *string
	err := d.conn.QueryRow(
		"SELECT id, name, description, workspace_id, created_at, updated_at FROM boards WHERE id = ?",
		id,
	).Scan(&board.ID, &board.Name, &board.Description, &wsID, &board.CreatedAt, &board.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("board not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying board: %w", err)
	}
	if wsID != nil {
		board.WorkspaceID = *wsID
	}
	return board, nil
}

func (d *DB) GetBoardByName(name string) (*model.Board, error) {
	board := &model.Board{}
	var wsID *string
	err := d.conn.QueryRow(
		"SELECT id, name, description, workspace_id, created_at, updated_at FROM boards WHERE name = ?",
		name,
	).Scan(&board.ID, &board.Name, &board.Description, &wsID, &board.CreatedAt, &board.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying board by name: %w", err)
	}
	if wsID != nil {
		board.WorkspaceID = *wsID
	}
	return board, nil
}

func (d *DB) ListBoards() ([]*model.Board, error) {
	rows, err := d.conn.Query(
		"SELECT id, name, description, workspace_id, created_at, updated_at FROM boards ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("listing boards: %w", err)
	}
	defer rows.Close()
	return scanBoards(rows)
}

func (d *DB) ListBoardsByWorkspace(workspaceID string) ([]*model.Board, error) {
	rows, err := d.conn.Query(
		"SELECT id, name, description, workspace_id, created_at, updated_at FROM boards WHERE workspace_id = ? ORDER BY name",
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing boards by workspace: %w", err)
	}
	defer rows.Close()
	return scanBoards(rows)
}

func (d *DB) SetBoardWorkspace(boardID, workspaceID string) error {
	result, err := d.conn.Exec(
		"UPDATE boards SET workspace_id = ?, updated_at = ? WHERE id = ?",
		workspaceID, time.Now().UTC(), boardID,
	)
	if err != nil {
		return fmt.Errorf("setting board workspace: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("board not found")
	}
	return nil
}

func scanBoards(rows *sql.Rows) ([]*model.Board, error) {
	var boards []*model.Board
	for rows.Next() {
		board := &model.Board{}
		var wsID *string
		if err := rows.Scan(&board.ID, &board.Name, &board.Description, &wsID, &board.CreatedAt, &board.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning board: %w", err)
		}
		if wsID != nil {
			board.WorkspaceID = *wsID
		}
		boards = append(boards, board)
	}
	return boards, rows.Err()
}

func (d *DB) DeleteBoard(id string) error {
	result, err := d.conn.Exec("DELETE FROM boards WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting board: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("board not found")
	}
	return nil
}
