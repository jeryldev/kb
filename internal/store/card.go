package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

type CardFilter struct {
	Priority string
	Column   string
	Search   string
	Label    string
}

func (f CardFilter) IsEmpty() bool {
	return f.Priority == "" && f.Column == "" && f.Search == "" && f.Label == ""
}

func (d *DB) CreateCard(columnID, title string, priority model.Priority) (*model.Card, error) {
	if err := model.ValidateCardTitle(title); err != nil {
		return nil, err
	}

	var maxPos int
	err := d.conn.QueryRow(
		"SELECT COALESCE(MAX(position), -1) FROM cards WHERE column_id = ? AND deleted_at IS NULL",
		columnID,
	).Scan(&maxPos)
	if err != nil {
		return nil, fmt.Errorf("getting max position: %w", err)
	}

	now := time.Now().UTC()
	card := &model.Card{
		ID:        uuid.New().String(),
		ColumnID:  columnID,
		Title:     title,
		Priority:  priority,
		Position:  maxPos + 1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = d.conn.Exec(
		`INSERT INTO cards (id, column_id, title, description, priority, position, labels, external_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		card.ID, card.ColumnID, card.Title, card.Description, string(card.Priority),
		card.Position, card.Labels, card.ExternalID, card.CreatedAt, card.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting card: %w", err)
	}

	return card, nil
}

func (d *DB) GetCard(id string) (*model.Card, error) {
	card := &model.Card{}
	var priority string
	err := d.conn.QueryRow(
		`SELECT id, column_id, title, description, priority, position, labels, external_id,
		        archived_at, deleted_at, created_at, updated_at
		 FROM cards WHERE id = ? AND deleted_at IS NULL`,
		id,
	).Scan(
		&card.ID, &card.ColumnID, &card.Title, &card.Description, &priority,
		&card.Position, &card.Labels, &card.ExternalID,
		&card.ArchivedAt, &card.DeletedAt, &card.CreatedAt, &card.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("card not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying card: %w", err)
	}
	card.Priority = model.Priority(priority)
	return card, nil
}

func (d *DB) ListCards(columnID string) ([]*model.Card, error) {
	rows, err := d.conn.Query(
		`SELECT id, column_id, title, description, priority, position, labels, external_id,
		        archived_at, deleted_at, created_at, updated_at
		 FROM cards
		 WHERE column_id = ? AND deleted_at IS NULL AND archived_at IS NULL
		 ORDER BY position`,
		columnID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing cards: %w", err)
	}
	defer rows.Close()

	var cards []*model.Card
	for rows.Next() {
		card := &model.Card{}
		var priority string
		if err := rows.Scan(
			&card.ID, &card.ColumnID, &card.Title, &card.Description, &priority,
			&card.Position, &card.Labels, &card.ExternalID,
			&card.ArchivedAt, &card.DeletedAt, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning card: %w", err)
		}
		card.Priority = model.Priority(priority)
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (d *DB) UpdateCard(card *model.Card) error {
	if err := model.ValidateCardTitle(card.Title); err != nil {
		return err
	}

	card.UpdatedAt = time.Now().UTC()
	_, err := d.conn.Exec(
		`UPDATE cards SET column_id = ?, title = ?, description = ?, priority = ?,
		 position = ?, labels = ?, external_id = ?, updated_at = ?
		 WHERE id = ? AND deleted_at IS NULL`,
		card.ColumnID, card.Title, card.Description, string(card.Priority),
		card.Position, card.Labels, card.ExternalID, card.UpdatedAt,
		card.ID,
	)
	if err != nil {
		return fmt.Errorf("updating card: %w", err)
	}
	return nil
}

func (d *DB) MoveCard(cardID, targetColumnID string) error {
	var maxPos int
	err := d.conn.QueryRow(
		"SELECT COALESCE(MAX(position), -1) FROM cards WHERE column_id = ? AND deleted_at IS NULL",
		targetColumnID,
	).Scan(&maxPos)
	if err != nil {
		return fmt.Errorf("getting max position: %w", err)
	}

	now := time.Now().UTC()
	_, err = d.conn.Exec(
		"UPDATE cards SET column_id = ?, position = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		targetColumnID, maxPos+1, now, cardID,
	)
	if err != nil {
		return fmt.Errorf("moving card: %w", err)
	}
	return nil
}

func (d *DB) ArchiveCard(id string) error {
	now := time.Now().UTC()
	result, err := d.conn.Exec(
		"UPDATE cards SET archived_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL",
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("archiving card: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("card not found or already archived")
	}
	return nil
}

func (d *DB) DeleteCard(id string) error {
	now := time.Now().UTC()
	result, err := d.conn.Exec(
		"UPDATE cards SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL",
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("deleting card: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("card not found or already deleted")
	}
	return nil
}

func (d *DB) ReorderCardsInColumn(columnID string, cardIDs []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	for i, id := range cardIDs {
		if _, err := tx.Exec(
			"UPDATE cards SET position = ?, updated_at = ? WHERE id = ? AND column_id = ?",
			i, now, id, columnID,
		); err != nil {
			return fmt.Errorf("updating card position: %w", err)
		}
	}

	return tx.Commit()
}

func (d *DB) ListBoardCards(boardID string) ([]*model.Card, error) {
	rows, err := d.conn.Query(
		`SELECT c.id, c.column_id, c.title, c.description, c.priority, c.position, c.labels,
		        c.external_id, c.archived_at, c.deleted_at, c.created_at, c.updated_at
		 FROM cards c
		 JOIN columns col ON c.column_id = col.id
		 WHERE col.board_id = ? AND c.deleted_at IS NULL AND c.archived_at IS NULL
		 ORDER BY col.position, c.position`,
		boardID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing board cards: %w", err)
	}
	defer rows.Close()

	var cards []*model.Card
	for rows.Next() {
		card := &model.Card{}
		var priority string
		if err := rows.Scan(
			&card.ID, &card.ColumnID, &card.Title, &card.Description, &priority,
			&card.Position, &card.Labels, &card.ExternalID,
			&card.ArchivedAt, &card.DeletedAt, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning card: %w", err)
		}
		card.Priority = model.Priority(priority)
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (d *DB) ListBoardCardsFiltered(boardID string, filter CardFilter) ([]*model.Card, error) {
	if filter.IsEmpty() {
		return d.ListBoardCards(boardID)
	}

	query := `SELECT c.id, c.column_id, c.title, c.description, c.priority, c.position, c.labels,
		        c.external_id, c.archived_at, c.deleted_at, c.created_at, c.updated_at
		 FROM cards c
		 JOIN columns col ON c.column_id = col.id
		 WHERE col.board_id = ? AND c.deleted_at IS NULL AND c.archived_at IS NULL`
	args := []interface{}{boardID}

	if filter.Priority != "" {
		query += " AND c.priority = ?"
		args = append(args, filter.Priority)
	}
	if filter.Column != "" {
		query += " AND LOWER(col.name) = LOWER(?)"
		args = append(args, filter.Column)
	}
	if filter.Search != "" {
		search := "%" + strings.ToLower(filter.Search) + "%"
		query += " AND (LOWER(c.title) LIKE ? OR LOWER(c.description) LIKE ?)"
		args = append(args, search, search)
	}

	query += " ORDER BY col.position, c.position"

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing filtered board cards: %w", err)
	}
	defer rows.Close()

	var cards []*model.Card
	for rows.Next() {
		card := &model.Card{}
		var priority string
		if err := rows.Scan(
			&card.ID, &card.ColumnID, &card.Title, &card.Description, &priority,
			&card.Position, &card.Labels, &card.ExternalID,
			&card.ArchivedAt, &card.DeletedAt, &card.CreatedAt, &card.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning card: %w", err)
		}
		card.Priority = model.Priority(priority)
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if filter.Label != "" {
		var filtered []*model.Card
		for _, card := range cards {
			if card.HasLabel(filter.Label) {
				filtered = append(filtered, card)
			}
		}
		return filtered, nil
	}

	return cards, nil
}
