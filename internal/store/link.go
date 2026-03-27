package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) SyncNoteLinks(note *model.Note) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM links WHERE source_type = 'note' AND source_id = ?", note.ID,
	); err != nil {
		return fmt.Errorf("clearing old links: %w", err)
	}

	parsed := model.ParseWikilinks(note.Body)
	for _, pl := range parsed {
		targetID := pl.TargetRef
		if pl.TargetType == "note" {
			var id string
			err := tx.QueryRow(
				"SELECT id FROM notes WHERE slug = ? AND archived_at IS NULL", pl.TargetRef,
			).Scan(&id)
			if err == nil {
				targetID = id
			}
		}

		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO links (id, source_type, source_id, target_type, target_id, context)
			 VALUES (?, 'note', ?, ?, ?, ?)`,
			uuid.New().String(), note.ID, pl.TargetType, targetID, pl.Context,
		); err != nil {
			return fmt.Errorf("inserting link: %w", err)
		}
	}

	return tx.Commit()
}

func (d *DB) GetForwardLinks(sourceType, sourceID string) ([]*model.Link, error) {
	rows, err := d.conn.Query(
		`SELECT id, source_type, source_id, target_type, target_id, context, created_at
		 FROM links WHERE source_type = ? AND source_id = ?
		 ORDER BY created_at`,
		sourceType, sourceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing forward links: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (d *DB) GetBacklinks(targetType, targetID string) ([]*model.Link, error) {
	rows, err := d.conn.Query(
		`SELECT id, source_type, source_id, target_type, target_id, context, created_at
		 FROM links WHERE target_type = ? AND target_id = ?
		 ORDER BY created_at`,
		targetType, targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing backlinks: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func (d *DB) ListAllLinks() ([]*model.Link, error) {
	rows, err := d.conn.Query(
		`SELECT id, source_type, source_id, target_type, target_id, context, created_at
		 FROM links ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing all links: %w", err)
	}
	defer rows.Close()
	return scanLinks(rows)
}

func scanLinks(rows *sql.Rows) ([]*model.Link, error) {
	var links []*model.Link
	for rows.Next() {
		link := &model.Link{}
		if err := rows.Scan(
			&link.ID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.Context, &link.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
