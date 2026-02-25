package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open() (*DB, error) {
	dbPath, err := dbPath()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

func OpenWithPath(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	_, err := d.conn.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	var version int
	err = d.conn.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("checking migration version: %w", err)
	}

	if version < 1 {
		if err := d.migrate001(); err != nil {
			return err
		}
	}
	if version < 2 {
		if err := d.migrate002(); err != nil {
			return err
		}
	}
	if version < 3 {
		if err := d.migrate003(); err != nil {
			return err
		}
	}
	if version < 4 {
		if err := d.migrate004(); err != nil {
			return err
		}
	}
	if version < 5 {
		if err := d.migrate005(); err != nil {
			return err
		}
	}

	return nil
}

func (d *DB) migrate001() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	schema := `
		CREATE TABLE IF NOT EXISTS boards (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS columns (
			id TEXT PRIMARY KEY,
			board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			wip_limit INTEGER
		);

		CREATE INDEX IF NOT EXISTS idx_columns_board_id ON columns(board_id);

		CREATE TABLE IF NOT EXISTS cards (
			id TEXT PRIMARY KEY,
			column_id TEXT NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL DEFAULT 'medium',
			position INTEGER NOT NULL DEFAULT 0,
			labels TEXT NOT NULL DEFAULT '',
			external_id TEXT NOT NULL DEFAULT '',
			archived_at TIMESTAMP,
			deleted_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_cards_column_id ON cards(column_id);
		CREATE INDEX IF NOT EXISTS idx_cards_archived_at ON cards(archived_at);
		CREATE INDEX IF NOT EXISTS idx_cards_deleted_at ON cards(deleted_at);
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 001: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (1)"); err != nil {
		return fmt.Errorf("recording migration 001: %w", err)
	}

	return tx.Commit()
}

func (d *DB) migrate002() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	schema := `
		CREATE TABLE IF NOT EXISTS notes (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			body TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			archived_at TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_notes_slug ON notes(slug);
		CREATE INDEX IF NOT EXISTS idx_notes_archived_at ON notes(archived_at);

		CREATE TABLE IF NOT EXISTS links (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(source_type, source_id, target_type, target_id)
		);

		CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_type, source_id);
		CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_type, target_id);
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 002: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (2)"); err != nil {
		return fmt.Errorf("recording migration 002: %w", err)
	}

	return tx.Commit()
}

func (d *DB) migrate003() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	schema := `
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		ALTER TABLE boards ADD COLUMN workspace_id TEXT REFERENCES workspaces(id);
		ALTER TABLE notes ADD COLUMN workspace_id TEXT REFERENCES workspaces(id);

		CREATE INDEX IF NOT EXISTS idx_boards_workspace_id ON boards(workspace_id);
		CREATE INDEX IF NOT EXISTS idx_notes_workspace_id ON notes(workspace_id);
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 003: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (3)"); err != nil {
		return fmt.Errorf("recording migration 003: %w", err)
	}

	return tx.Commit()
}

func (d *DB) migrate004() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	schema := `
		CREATE TABLE IF NOT EXISTS publish_targets (
			id           TEXT PRIMARY KEY,
			workspace_id TEXT REFERENCES workspaces(id),
			name         TEXT NOT NULL UNIQUE,
			engine       TEXT NOT NULL,
			base_path    TEXT NOT NULL,
			posts_dir    TEXT NOT NULL DEFAULT '_posts',
			created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS publish_log (
			id           TEXT PRIMARY KEY,
			note_id      TEXT NOT NULL REFERENCES notes(id),
			target_id    TEXT NOT NULL REFERENCES publish_targets(id),
			file_path    TEXT NOT NULL,
			front_matter TEXT NOT NULL DEFAULT '',
			published_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_publish_log_note_id ON publish_log(note_id);
		CREATE INDEX IF NOT EXISTS idx_publish_log_target_id ON publish_log(target_id);
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 004: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (4)"); err != nil {
		return fmt.Errorf("recording migration 004: %w", err)
	}

	return tx.Commit()
}

func (d *DB) migrate005() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	wsID := uuid.New().String()
	_, err = tx.Exec(
		`INSERT INTO workspaces (id, name, kind, description, path, position, created_at, updated_at)
		 VALUES (?, ?, ?, '', '', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		wsID, model.DefaultWorkspaceName, string(model.KindArea),
	)
	if err != nil {
		return fmt.Errorf("inserting default workspace: %w", err)
	}

	if _, err := tx.Exec("UPDATE boards SET workspace_id = ? WHERE workspace_id IS NULL", wsID); err != nil {
		return fmt.Errorf("updating boards with default workspace: %w", err)
	}
	if _, err := tx.Exec("UPDATE notes SET workspace_id = ? WHERE workspace_id IS NULL", wsID); err != nil {
		return fmt.Errorf("updating notes with default workspace: %w", err)
	}

	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (5)"); err != nil {
		return fmt.Errorf("recording migration 005: %w", err)
	}

	return tx.Commit()
}

func dbPath() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding home directory: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "kb", "kb.db"), nil
}
