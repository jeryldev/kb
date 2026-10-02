-- The kb 0.3 schema (version 7), without the full-text index, from a real kb.db.
CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
CREATE TABLE boards (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
, workspace_id TEXT REFERENCES workspaces(id));
CREATE TABLE columns (
    id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    wip_limit INTEGER
);
CREATE INDEX idx_columns_board_id ON columns(board_id);
CREATE TABLE cards (
    id TEXT PRIMARY KEY,
    column_id TEXT NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL DEFAULT 'medium',
    position INTEGER NOT NULL DEFAULT 0,
    labels TEXT NOT NULL DEFAULT '',
    external_id TEXT,
    archived_at TIMESTAMP,
    deleted_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_cards_column_id ON cards(column_id);
CREATE INDEX idx_cards_archived_at ON cards(archived_at);
CREATE INDEX idx_cards_deleted_at ON cards(deleted_at);
CREATE TABLE notes (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			body TEXT NOT NULL DEFAULT '',
			tags TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			archived_at TIMESTAMP
		, workspace_id TEXT REFERENCES workspaces(id), path TEXT, mtime INTEGER NOT NULL DEFAULT 0, size INTEGER NOT NULL DEFAULT 0, aliases TEXT NOT NULL DEFAULT '');
CREATE INDEX idx_notes_slug ON notes(slug);
CREATE INDEX idx_notes_archived_at ON notes(archived_at);
CREATE TABLE links (
			id TEXT PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			target_type TEXT NOT NULL,
			target_id TEXT NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, target_ref TEXT NOT NULL DEFAULT '',
			UNIQUE(source_type, source_id, target_type, target_id)
		);
CREATE INDEX idx_links_source ON links(source_type, source_id);
CREATE INDEX idx_links_target ON links(target_type, target_id);
CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
CREATE INDEX idx_boards_workspace_id ON boards(workspace_id);
CREATE INDEX idx_notes_workspace_id ON notes(workspace_id);
CREATE TABLE publish_targets (
			id           TEXT PRIMARY KEY,
			workspace_id TEXT REFERENCES workspaces(id),
			name         TEXT NOT NULL UNIQUE,
			engine       TEXT NOT NULL,
			base_path    TEXT NOT NULL,
			posts_dir    TEXT NOT NULL DEFAULT '_posts',
			created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
CREATE TABLE publish_log (
			id           TEXT PRIMARY KEY,
			note_id      TEXT NOT NULL REFERENCES notes(id),
			target_id    TEXT NOT NULL REFERENCES publish_targets(id),
			file_path    TEXT NOT NULL,
			front_matter TEXT NOT NULL DEFAULT '',
			published_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
CREATE INDEX idx_publish_log_note_id ON publish_log(note_id);
CREATE INDEX idx_publish_log_target_id ON publish_log(target_id);
CREATE UNIQUE INDEX idx_notes_path ON notes(path);
CREATE TABLE note_aliases (
			note_id TEXT NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
			alias   TEXT NOT NULL COLLATE NOCASE
		);
CREATE INDEX idx_note_aliases_alias ON note_aliases(alias);
CREATE INDEX idx_note_aliases_note_id ON note_aliases(note_id);
INSERT INTO schema_migrations (version) VALUES (1),(2),(3),(4),(5),(6),(7);
