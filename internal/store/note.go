package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) CreateNote(title, slug, body string) (*model.Note, error) {
	if err := model.ValidateNoteTitle(title); err != nil {
		return nil, err
	}
	if err := model.ValidateNoteSlug(slug); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	note := &model.Note{
		ID:        uuid.New().String(),
		Title:     title,
		Slug:      slug,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := d.conn.Exec(
		`INSERT INTO notes (id, title, slug, body, tags, pinned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		note.ID, note.Title, note.Slug, note.Body, note.Tags, 0, note.CreatedAt, note.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("note with slug %q already exists", slug)
		}
		return nil, fmt.Errorf("inserting note: %w", err)
	}

	return note, nil
}

func (d *DB) GetNote(id string) (*model.Note, error) {
	note := &model.Note{}
	var pinned int
	err := d.conn.QueryRow(
		`SELECT id, title, slug, body, tags, pinned, created_at, updated_at, archived_at
		 FROM notes WHERE id = ? AND archived_at IS NULL`,
		id,
	).Scan(&note.ID, &note.Title, &note.Slug, &note.Body, &note.Tags,
		&pinned, &note.CreatedAt, &note.UpdatedAt, &note.ArchivedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	note.Pinned = pinned != 0
	return note, nil
}

func (d *DB) GetNoteBySlug(slug string) (*model.Note, error) {
	note := &model.Note{}
	var pinned int
	err := d.conn.QueryRow(
		`SELECT id, title, slug, body, tags, pinned, created_at, updated_at, archived_at
		 FROM notes WHERE slug = ? AND archived_at IS NULL`,
		slug,
	).Scan(&note.ID, &note.Title, &note.Slug, &note.Body, &note.Tags,
		&pinned, &note.CreatedAt, &note.UpdatedAt, &note.ArchivedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note %q not found", slug)
	}
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	note.Pinned = pinned != 0
	return note, nil
}

func (d *DB) ListNotes() ([]*model.Note, error) {
	rows, err := d.conn.Query(
		`SELECT id, title, slug, body, tags, pinned, created_at, updated_at, archived_at
		 FROM notes WHERE archived_at IS NULL
		 ORDER BY pinned DESC, updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	defer rows.Close()
	return scanNotes(rows)
}

func (d *DB) SearchNotes(query string) ([]*model.Note, error) {
	search := "%" + strings.ToLower(query) + "%"
	rows, err := d.conn.Query(
		`SELECT id, title, slug, body, tags, pinned, created_at, updated_at, archived_at
		 FROM notes WHERE archived_at IS NULL
		 AND (LOWER(title) LIKE ? OR LOWER(body) LIKE ? OR LOWER(tags) LIKE ?)
		 ORDER BY updated_at DESC`,
		search, search, search,
	)
	if err != nil {
		return nil, fmt.Errorf("searching notes: %w", err)
	}
	defer rows.Close()
	return scanNotes(rows)
}

func (d *DB) ListNotesByTag(tag string) ([]*model.Note, error) {
	notes, err := d.ListNotes()
	if err != nil {
		return nil, err
	}
	var filtered []*model.Note
	for _, n := range notes {
		if n.HasTag(tag) {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

func (d *DB) UpdateNote(note *model.Note) error {
	if err := model.ValidateNoteTitle(note.Title); err != nil {
		return err
	}

	note.UpdatedAt = time.Now().UTC()
	_, err := d.conn.Exec(
		`UPDATE notes SET title = ?, slug = ?, body = ?, tags = ?, pinned = ?, updated_at = ?
		 WHERE id = ? AND archived_at IS NULL`,
		note.Title, note.Slug, note.Body, note.Tags, boolToInt(note.Pinned), note.UpdatedAt, note.ID,
	)
	if err != nil {
		return fmt.Errorf("updating note: %w", err)
	}
	return nil
}

func (d *DB) ArchiveNote(id string) error {
	now := time.Now().UTC()
	result, err := d.conn.Exec(
		"UPDATE notes SET archived_at = ?, updated_at = ? WHERE id = ? AND archived_at IS NULL",
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("archiving note: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("note not found or already archived")
	}
	return nil
}

func (d *DB) ResolveNoteID(prefix string) (string, error) {
	rows, err := d.conn.Query(
		"SELECT id FROM notes WHERE archived_at IS NULL",
	)
	if err != nil {
		return "", fmt.Errorf("listing notes: %w", err)
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", fmt.Errorf("scanning note id: %w", err)
		}
		if id == prefix || (len(prefix) >= 4 && strings.HasPrefix(id, prefix)) {
			matches = append(matches, id)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no note found matching %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous note ID %q matches %d notes; use more characters", prefix, len(matches))
	}
}

func (d *DB) DeleteNote(id string) error {
	result, err := d.conn.Exec("DELETE FROM notes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting note: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("note not found")
	}
	return nil
}

func scanNotes(rows *sql.Rows) ([]*model.Note, error) {
	var notes []*model.Note
	for rows.Next() {
		note := &model.Note{}
		var pinned int
		if err := rows.Scan(
			&note.ID, &note.Title, &note.Slug, &note.Body, &note.Tags,
			&pinned, &note.CreatedAt, &note.UpdatedAt, &note.ArchivedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
		note.Pinned = pinned != 0
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
