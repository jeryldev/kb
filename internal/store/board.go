package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) CreateBoard(name, description string) (*model.Board, error) {
	if err := model.ValidateBoardName(name); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	board := &model.Board{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO boards (id, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		board.ID, board.Name, board.Description, board.CreatedAt, board.UpdatedAt,
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
	err := d.conn.QueryRow(
		"SELECT id, name, description, created_at, updated_at FROM boards WHERE id = ?",
		id,
	).Scan(&board.ID, &board.Name, &board.Description, &board.CreatedAt, &board.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("board not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying board: %w", err)
	}
	return board, nil
}

func (d *DB) GetBoardByName(name string) (*model.Board, error) {
	board := &model.Board{}
	err := d.conn.QueryRow(
		"SELECT id, name, description, created_at, updated_at FROM boards WHERE name = ?",
		name,
	).Scan(&board.ID, &board.Name, &board.Description, &board.CreatedAt, &board.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying board by name: %w", err)
	}
	return board, nil
}

func (d *DB) ListBoards() ([]*model.Board, error) {
	rows, err := d.conn.Query(
		"SELECT id, name, description, created_at, updated_at FROM boards ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("listing boards: %w", err)
	}
	defer rows.Close()

	var boards []*model.Board
	for rows.Next() {
		board := &model.Board{}
		if err := rows.Scan(&board.ID, &board.Name, &board.Description, &board.CreatedAt, &board.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning board: %w", err)
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
