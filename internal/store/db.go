package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn  *sql.DB
	vault *vault.Vault
	// opened is what the scan on open found, for the caller to report.
	opened ScanResult
}

// Open opens the index at the default location over the vault named by
// KB_VAULT, or ~/notes.
func Open() (*DB, error) {
	dbPath, err := dbPath()
	if err != nil {
		return nil, err
	}
	vaultDir, err := VaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}
	return OpenWithPath(dbPath, vaultDir)
}

// OpenWithPath opens (or creates) the index at path, migrates it, moves any
// notes still stored only in SQLite out to the vault, and scans the vault.
func OpenWithPath(path, vaultDir string) (*DB, error) {
	// One connection: SQLite has a single writer anyway, and ":memory:" is a
	// separate database per connection. busy_timeout makes a second kb
	// process wait for the lock instead of failing with SQLITE_BUSY, and
	// immediate transactions take that lock up front, so a read-then-write
	// transaction cannot deadlock against another process.
	conn, err := sql.Open("sqlite", path+
		"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)&_pragma=busy_timeout(10000)&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn, vault: vault.New(vaultDir)}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	if err := db.exportLegacyNotes(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("moving notes to the vault: %w", err)
	}
	res, err := db.Scan()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("scanning the vault: %w", err)
	}
	db.opened = res

	return db, nil
}

func (d *DB) Vault() *vault.Vault {
	return d.vault
}

// Problems lists files the scan on open could only partly read.
func (d *DB) Problems() []string {
	return d.opened.Problems
}

// OpenScan is what the scan on open changed in the index.
func (d *DB) OpenScan() ScanResult {
	return d.opened
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

	// Use BEGIN IMMEDIATE to prevent concurrent migration races.
	// Without this, two processes could both read version=0 and
	// attempt the same migration simultaneously.
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration lock: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec("SELECT 1 FROM schema_migrations LIMIT 1"); err == nil {
		// Table exists; try to acquire write lock via a dummy write
		_, _ = tx.Exec("DELETE FROM schema_migrations WHERE version = -1")
	}

	var version int
	err = tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return fmt.Errorf("checking migration version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration version check: %w", err)
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
	if version < 6 {
		if err := d.migrate006(); err != nil {
			return err
		}
	}
	if version < 7 {
		if err := d.migrate007(); err != nil {
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

// migrate006 makes the notes table an index of the vault: each row records
// the file it came from and the mtime and size it had when it was read.
func (d *DB) migrate006() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	schema := `
		ALTER TABLE notes ADD COLUMN path TEXT;
		ALTER TABLE notes ADD COLUMN mtime INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE notes ADD COLUMN size INTEGER NOT NULL DEFAULT 0;
		ALTER TABLE notes ADD COLUMN aliases TEXT NOT NULL DEFAULT '';
		CREATE UNIQUE INDEX IF NOT EXISTS idx_notes_path ON notes(path);
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 006: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (6)"); err != nil {
		return fmt.Errorf("recording migration 006: %w", err)
	}

	return tx.Commit()
}

// migrate007 adds what link resolution and search need: the text of each
// link as written (so a dangling link can be resolved again later), note
// aliases, and a full-text index of the notes. Zeroing mtime makes the next
// scan re-read every file to fill them in.
func (d *DB) migrate007() error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	// The FTS table keeps its own copy of the text, with each row's rowid
	// matching its note's so the triggers can find it without a scan. It
	// does not use external content: notes has no INTEGER PRIMARY KEY, so
	// VACUUM may renumber its rowids. If that happens, search still joins on
	// the id column and so never returns the wrong note, and Rebuild refills
	// the table.
	schema := `
		ALTER TABLE links ADD COLUMN target_ref TEXT NOT NULL DEFAULT '';
		UPDATE links SET target_ref = target_id;

		CREATE TABLE note_aliases (
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			alias   TEXT NOT NULL COLLATE NOCASE
		);
		CREATE INDEX idx_note_aliases_alias ON note_aliases(alias);
		CREATE INDEX idx_note_aliases_note_id ON note_aliases(note_id);

		CREATE VIRTUAL TABLE notes_fts USING fts5(
			id UNINDEXED, title, body, tags,
			tokenize = 'unicode61 remove_diacritics 2'
		);
		CREATE TRIGGER notes_fts_insert AFTER INSERT ON notes BEGIN
			INSERT INTO notes_fts (rowid, id, title, body, tags) VALUES (new.rowid, new.id, new.title, new.body, new.tags);
		END;
		CREATE TRIGGER notes_fts_update AFTER UPDATE OF title, body, tags ON notes BEGIN
			DELETE FROM notes_fts WHERE rowid = old.rowid;
			INSERT INTO notes_fts (rowid, id, title, body, tags) VALUES (new.rowid, new.id, new.title, new.body, new.tags);
		END;
		CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
			DELETE FROM notes_fts WHERE rowid = old.rowid;
		END;
		INSERT INTO notes_fts (rowid, id, title, body, tags) SELECT rowid, id, title, body, tags FROM notes;

		UPDATE notes SET mtime = 0;
	`
	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("applying migration 007: %w", err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (7)"); err != nil {
		return fmt.Errorf("recording migration 007: %w", err)
	}

	return tx.Commit()
}

// VaultDir is KB_VAULT, or ~/notes.
func VaultDir() (string, error) {
	if dir := os.Getenv("KB_VAULT"); dir != "" {
		if rest, ok := strings.CutPrefix(dir, "~/"); ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("finding home directory: %w", err)
			}
			return filepath.Join(home, rest), nil
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, "notes"), nil
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
