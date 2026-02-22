package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) ListColumns(boardID string) ([]*model.Column, error) {
	rows, err := d.conn.Query(
		"SELECT id, board_id, name, position, wip_limit FROM columns WHERE board_id = ? ORDER BY position",
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing columns: %w", err)
	}
	defer rows.Close()

	var columns []*model.Column
	for rows.Next() {
		col := &model.Column{}
		if err := rows.Scan(&col.ID, &col.BoardID, &col.Name, &col.Position, &col.WIPLimit); err != nil {
			return nil, fmt.Errorf("scanning column: %w", err)
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (d *DB) CreateColumn(boardID, name string) (*model.Column, error) {
	if err := model.ValidateColumnName(name); err != nil {
		return nil, err
	}

	var maxPos int
	err := d.conn.QueryRow(
		"SELECT COALESCE(MAX(position), -1) FROM columns WHERE board_id = ?",
		boardID,
	).Scan(&maxPos)
	if err != nil {
		return nil, fmt.Errorf("getting max position: %w", err)
	}

	col := &model.Column{
		ID:       uuid.New().String(),
		BoardID:  boardID,
		Name:     name,
		Position: maxPos + 1,
	}

	_, err = d.conn.Exec(
		"INSERT INTO columns (id, board_id, name, position) VALUES (?, ?, ?, ?)",
		col.ID, col.BoardID, col.Name, col.Position,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting column: %w", err)
	}

	return col, nil
}

func (d *DB) UpdateColumnWIPLimit(id string, limit *int) error {
	_, err := d.conn.Exec("UPDATE columns SET wip_limit = ? WHERE id = ?", limit, id)
	if err != nil {
		return fmt.Errorf("updating WIP limit: %w", err)
	}
	return nil
}

func (d *DB) ReorderColumns(boardID string, columnIDs []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for i, id := range columnIDs {
		_, err := tx.Exec(
			"UPDATE columns SET position = ? WHERE id = ? AND board_id = ?",
			i, id, boardID,
		)
		if err != nil {
			return fmt.Errorf("reordering column %s: %w", id, err)
		}
	}

	return tx.Commit()
}

func (d *DB) DeleteColumn(id string) error {
	result, err := d.conn.Exec("DELETE FROM columns WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting column: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("column not found")
	}
	return nil
}

func (d *DB) CountCardsInColumn(columnID string) (int, error) {
	var count int
	err := d.conn.QueryRow(
		"SELECT COUNT(*) FROM cards WHERE column_id = ? AND deleted_at IS NULL AND archived_at IS NULL",
		columnID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting cards: %w", err)
	}
	return count, nil
}
