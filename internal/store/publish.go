package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) CreatePublishTarget(name string, engine model.Engine, basePath, postsDir string, workspaceID *string) (*model.PublishTarget, error) {
	if name == "" {
		return nil, fmt.Errorf("publish target name cannot be empty")
	}
	if basePath == "" {
		return nil, fmt.Errorf("base path cannot be empty")
	}
	if postsDir == "" {
		postsDir = "_posts"
	}

	now := time.Now().UTC()
	pt := &model.PublishTarget{
		ID:          uuid.New().String(),
		WorkspaceID: workspaceID,
		Name:        name,
		Engine:      engine,
		BasePath:    basePath,
		PostsDir:    postsDir,
		CreatedAt:   now,
	}

	_, err := d.conn.Exec(
		`INSERT INTO publish_targets (id, workspace_id, name, engine, base_path, posts_dir, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		pt.ID, pt.WorkspaceID, pt.Name, string(pt.Engine), pt.BasePath, pt.PostsDir, pt.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("publish target %q already exists", name)
		}
		return nil, fmt.Errorf("inserting publish target: %w", err)
	}

	return pt, nil
}

func (d *DB) GetPublishTarget(id string) (*model.PublishTarget, error) {
	pt := &model.PublishTarget{}
	var engine string
	err := d.conn.QueryRow(
		`SELECT id, workspace_id, name, engine, base_path, posts_dir, created_at
		 FROM publish_targets WHERE id = ?`,
		id,
	).Scan(&pt.ID, &pt.WorkspaceID, &pt.Name, &engine, &pt.BasePath, &pt.PostsDir, &pt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("publish target not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying publish target: %w", err)
	}
	pt.Engine = model.Engine(engine)
	return pt, nil
}

func (d *DB) GetPublishTargetByName(name string) (*model.PublishTarget, error) {
	pt := &model.PublishTarget{}
	var engine string
	err := d.conn.QueryRow(
		`SELECT id, workspace_id, name, engine, base_path, posts_dir, created_at
		 FROM publish_targets WHERE LOWER(name) = LOWER(?)`,
		name,
	).Scan(&pt.ID, &pt.WorkspaceID, &pt.Name, &engine, &pt.BasePath, &pt.PostsDir, &pt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("publish target %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("querying publish target: %w", err)
	}
	pt.Engine = model.Engine(engine)
	return pt, nil
}

func (d *DB) ListPublishTargets() ([]*model.PublishTarget, error) {
	rows, err := d.conn.Query(
		`SELECT id, workspace_id, name, engine, base_path, posts_dir, created_at
		 FROM publish_targets ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing publish targets: %w", err)
	}
	defer rows.Close()

	var targets []*model.PublishTarget
	for rows.Next() {
		pt := &model.PublishTarget{}
		var engine string
		if err := rows.Scan(&pt.ID, &pt.WorkspaceID, &pt.Name, &engine, &pt.BasePath, &pt.PostsDir, &pt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning publish target: %w", err)
		}
		pt.Engine = model.Engine(engine)
		targets = append(targets, pt)
	}
	return targets, rows.Err()
}

func (d *DB) DeletePublishTarget(id string) error {
	result, err := d.conn.Exec("DELETE FROM publish_targets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting publish target: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("publish target not found")
	}
	return nil
}

func (d *DB) CreatePublishLog(noteID, targetID, filePath, frontMatter string) (*model.PublishLog, error) {
	now := time.Now().UTC()
	pl := &model.PublishLog{
		ID:          uuid.New().String(),
		NoteID:      noteID,
		TargetID:    targetID,
		FilePath:    filePath,
		FrontMatter: frontMatter,
		PublishedAt: now,
	}

	_, err := d.conn.Exec(
		`INSERT INTO publish_log (id, note_id, target_id, file_path, front_matter, published_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		pl.ID, pl.NoteID, pl.TargetID, pl.FilePath, pl.FrontMatter, pl.PublishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting publish log: %w", err)
	}

	return pl, nil
}

func (d *DB) GetLatestPublishLog(noteID, targetID string) (*model.PublishLog, error) {
	pl := &model.PublishLog{}
	err := d.conn.QueryRow(
		`SELECT id, note_id, target_id, file_path, front_matter, published_at
		 FROM publish_log WHERE note_id = ? AND target_id = ?
		 ORDER BY published_at DESC LIMIT 1`,
		noteID, targetID,
	).Scan(&pl.ID, &pl.NoteID, &pl.TargetID, &pl.FilePath, &pl.FrontMatter, &pl.PublishedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying publish log: %w", err)
	}
	return pl, nil
}

func (d *DB) ListPublishLogs(targetID string) ([]*model.PublishLog, error) {
	rows, err := d.conn.Query(
		`SELECT id, note_id, target_id, file_path, front_matter, published_at
		 FROM publish_log WHERE target_id = ?
		 ORDER BY published_at DESC`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing publish logs: %w", err)
	}
	defer rows.Close()

	var logs []*model.PublishLog
	for rows.Next() {
		pl := &model.PublishLog{}
		if err := rows.Scan(&pl.ID, &pl.NoteID, &pl.TargetID, &pl.FilePath, &pl.FrontMatter, &pl.PublishedAt); err != nil {
			return nil, fmt.Errorf("scanning publish log: %w", err)
		}
		logs = append(logs, pl)
	}
	return logs, rows.Err()
}

func (d *DB) GetPublishedNoteSlugs(targetID string) (map[string]string, error) {
	rows, err := d.conn.Query(
		`SELECT DISTINCT n.slug, pl.file_path
		 FROM publish_log pl
		 JOIN notes n ON n.id = pl.note_id
		 WHERE pl.target_id = ?`,
		targetID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying published note slugs: %w", err)
	}
	defer rows.Close()

	slugs := make(map[string]string)
	for rows.Next() {
		var slug, filePath string
		if err := rows.Scan(&slug, &filePath); err != nil {
			return nil, fmt.Errorf("scanning slug: %w", err)
		}
		slugs[slug] = filePath
	}
	return slugs, rows.Err()
}
