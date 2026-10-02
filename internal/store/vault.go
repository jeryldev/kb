package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

type ScanResult struct {
	Added, Updated, Removed int
	// Problems names files that could only be read in part, such as a note
	// whose frontmatter is not valid YAML (it is indexed as plain text).
	Problems []string
}

type scannedFile struct {
	entry vault.Entry
	doc   *vault.Doc
	id    string
}

// pathID is the id of a note whose file names none: derived from its path,
// so a rebuilt index gives it the same id without writing to the file.
func pathID(rel string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kb-vault:"+rel)).String()
}

// Scan brings the index up to date with the vault: files that are new or
// whose mtime or size changed are read again, and rows whose file is gone
// are dropped.
func (d *DB) Scan() (ScanResult, error) {
	var res ScanResult

	type indexed struct {
		id          string
		mtime, size int64
	}
	byPath := map[string]indexed{}
	allIDs := map[string]bool{}
	rows, err := d.conn.Query("SELECT id, path, mtime, size FROM notes")
	if err != nil {
		return res, fmt.Errorf("reading index: %w", err)
	}
	for rows.Next() {
		var ix indexed
		var rel sql.NullString
		if err := rows.Scan(&ix.id, &rel, &ix.mtime, &ix.size); err != nil {
			rows.Close()
			return res, fmt.Errorf("reading index: %w", err)
		}
		allIDs[ix.id] = true
		if rel.Valid {
			byPath[rel.String] = ix
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, fmt.Errorf("reading index: %w", err)
	}

	seen := map[string]bool{}
	var changed []*scannedFile
	// incomplete means the walk may have missed files, so a note not seen
	// cannot be taken as deleted.
	incomplete := false
	files := 0
	err = d.vault.WalkSkipping(func(e vault.Entry) error {
		files++
		ix, known := byPath[e.Path]
		if known && ix.mtime == e.ModTime.UnixNano() && ix.size == e.Size {
			seen[ix.id] = true
			return nil
		}
		doc, err := d.vault.Read(e.Path)
		if err != nil {
			abs, _ := d.vault.Abs(e.Path)
			data, readErr := os.ReadFile(abs)
			if readErr != nil {
				res.Problems = append(res.Problems, fmt.Sprintf("%s: %v", e.Path, readErr))
				incomplete = true
				if known {
					seen[ix.id] = true
				}
				return nil
			}
			res.Problems = append(res.Problems, err.Error())
			doc = &vault.Doc{Body: string(data)}
		}
		changed = append(changed, &scannedFile{entry: e, doc: doc})
		return nil
	}, func(rel string, err error) {
		res.Problems = append(res.Problems, fmt.Sprintf("skipped %s: %v", rel, err))
		incomplete = true
	})
	if err != nil {
		return res, fmt.Errorf("walking %s: %w", d.vault.Root(), err)
	}
	// An empty vault over a full index is far more likely a wrong KB_VAULT
	// or an unmounted drive than every note deleted at once.
	if files == 0 && len(byPath) > 1 {
		res.Problems = append(res.Problems, fmt.Sprintf(
			"no notes found in %s; kept the %d indexed notes (is KB_VAULT right?)", d.vault.Root(), len(byPath)))
		incomplete = true
	}

	// A file keeps the id in its frontmatter unless another file claims it
	// too (a copy); the file already indexed under that id wins, and the
	// other falls back to an id from its path. A file at an indexed path
	// that names no id (its YAML is broken mid-edit, or the id line was
	// removed) keeps the id it had, and with it its links and history.
	for _, f := range changed {
		ix, known := byPath[f.entry.Path]
		if id := f.doc.ID(); known && (id == ix.id || id == "") && !seen[ix.id] {
			f.id = ix.id
			seen[ix.id] = true
		}
	}
	for _, f := range changed {
		if f.id != "" {
			continue
		}
		f.id = f.doc.ID()
		if f.id == "" || seen[f.id] {
			f.id = pathID(f.entry.Path)
		}
		seen[f.id] = true
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return res, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Drop vanished notes first, so a new file can take the slug a deleted
	// one had.
	for id := range allIDs {
		if !seen[id] && !incomplete {
			if err := removeNoteRows(tx, id); err != nil {
				return res, err
			}
			res.Removed++
		}
	}
	// A path can pass from one note to another (a file replaced by a
	// different note), so release the changed paths before re-claiming them.
	for _, f := range changed {
		if _, err := tx.Exec("UPDATE notes SET path = NULL WHERE path = ? AND id != ?", f.entry.Path, f.id); err != nil {
			return res, fmt.Errorf("releasing path: %w", err)
		}
	}
	notes := make([]*model.Note, 0, len(changed))
	for _, f := range changed {
		note, err := upsertNote(tx, f)
		if err != nil {
			return res, err
		}
		notes = append(notes, note)
		if allIDs[f.id] {
			res.Updated++
		} else {
			res.Added++
		}
	}
	// Links last, once every changed note is in the index to link to.
	for _, note := range notes {
		if err := syncLinksTx(tx, note); err != nil {
			return res, err
		}
	}
	if err := resolveLinks(tx); err != nil {
		return res, err
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("committing scan: %w", err)
	}
	return res, nil
}

// indexOne re-indexes a file kb has just written, and returns its note.
func (d *DB) indexOne(entry vault.Entry, doc *vault.Doc) (*model.Note, error) {
	f := &scannedFile{entry: entry, doc: doc, id: doc.ID()}
	if f.id == "" {
		f.id = pathID(entry.Path)
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()
	note, err := upsertNote(tx, f)
	if err != nil {
		return nil, err
	}
	if err := syncLinksTx(tx, note); err != nil {
		return nil, err
	}
	if err := resolveLinks(tx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing note: %w", err)
	}
	return note, nil
}

func upsertNote(tx *sql.Tx, f *scannedFile) (*model.Note, error) {
	doc, entry := f.doc, f.entry
	note := &model.Note{
		ID:         f.id,
		Title:      doc.Title(),
		Body:       doc.Body,
		Tags:       strings.Join(doc.Tags(), ","),
		Pinned:     doc.Pinned(),
		Path:       entry.Path,
		ArchivedAt: doc.Archived(),
		UpdatedAt:  entry.ModTime.UTC(),
	}
	if note.Title == "" {
		note.Title = fileStem(entry.Path)
	}
	if created := doc.Created(); created != nil {
		note.CreatedAt = *created
	} else {
		note.CreatedAt = note.UpdatedAt
	}

	var err error
	if note.Slug, err = uniqueSlug(tx, fileStem(entry.Path), note.ID); err != nil {
		return nil, err
	}
	if note.WorkspaceID, err = workspaceIDByName(tx, doc.Workspace()); err != nil {
		return nil, err
	}

	_, err = tx.Exec(
		`INSERT INTO notes (id, title, slug, body, tags, aliases, pinned, workspace_id,
		                    created_at, updated_at, archived_at, path, mtime, size)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   title = excluded.title, slug = excluded.slug, body = excluded.body,
		   tags = excluded.tags, aliases = excluded.aliases, pinned = excluded.pinned,
		   workspace_id = excluded.workspace_id, created_at = excluded.created_at,
		   updated_at = excluded.updated_at, archived_at = excluded.archived_at,
		   path = excluded.path, mtime = excluded.mtime, size = excluded.size`,
		note.ID, note.Title, note.Slug, note.Body, note.Tags, strings.Join(doc.Aliases(), ","),
		boolToInt(note.Pinned), note.WorkspaceID, note.CreatedAt, note.UpdatedAt, note.ArchivedAt,
		note.Path, entry.ModTime.UnixNano(), entry.Size,
	)
	if err != nil {
		return nil, fmt.Errorf("indexing %s: %w", entry.Path, err)
	}
	if _, err := tx.Exec("DELETE FROM note_aliases WHERE note_id = ?", note.ID); err != nil {
		return nil, fmt.Errorf("indexing %s: %w", entry.Path, err)
	}
	for _, alias := range doc.Aliases() {
		if _, err := tx.Exec("INSERT INTO note_aliases (note_id, alias) VALUES (?, ?)", note.ID, alias); err != nil {
			return nil, fmt.Errorf("indexing %s: %w", entry.Path, err)
		}
	}
	return note, nil
}

// uniqueSlug derives a slug from the file name, adding -2, -3 when another
// note has it. A note keeps a suffixed slug it already holds, since its own
// row is not counted against it.
func uniqueSlug(tx *sql.Tx, stem, id string) (string, error) {
	base := model.Slugify(stem)
	if base == "" {
		// Names with no ASCII letters or digits, such as "日本.md".
		base = strings.Trim("note-"+model.Slugify(id), "-")
	}
	candidate := base
	for n := 2; ; n++ {
		var taken int
		if err := tx.QueryRow("SELECT COUNT(*) FROM notes WHERE slug = ? AND id != ?", candidate, id).Scan(&taken); err != nil {
			return "", fmt.Errorf("checking slug: %w", err)
		}
		if taken == 0 {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
}

// workspaceIDByName resolves a file's workspace key. An absent or unknown
// name means the default workspace.
func workspaceIDByName(tx *sql.Tx, name string) (string, error) {
	var id string
	err := sql.ErrNoRows
	if name != "" {
		err = tx.QueryRow("SELECT id FROM workspaces WHERE name = ? COLLATE NOCASE", name).Scan(&id)
	}
	if err == sql.ErrNoRows {
		err = tx.QueryRow("SELECT id FROM workspaces WHERE name = ?", model.DefaultWorkspaceName).Scan(&id)
	}
	if err != nil {
		return "", fmt.Errorf("resolving workspace %q: %w", name, err)
	}
	return id, nil
}

func removeNoteRows(tx *sql.Tx, id string) error {
	for _, stmt := range []string{
		"DELETE FROM links WHERE source_type = 'note' AND source_id = ?",
		"DELETE FROM publish_log WHERE note_id = ?",
		"DELETE FROM note_aliases WHERE note_id = ?",
		"DELETE FROM notes WHERE id = ?",
	} {
		if _, err := tx.Exec(stmt, id); err != nil {
			return fmt.Errorf("removing note %s: %w", id, err)
		}
	}
	return nil
}

// exportLegacyNotes writes out notes that an older kb kept only in SQLite,
// so they become files like every other note. It runs on every open and
// finds nothing once they are out.
func (d *DB) exportLegacyNotes() error {
	rows, err := d.conn.Query(
		`SELECT id, title, slug, body, tags, pinned, workspace_id, created_at, updated_at, archived_at
		 FROM notes WHERE path IS NULL`,
	)
	if err != nil {
		return fmt.Errorf("reading notes: %w", err)
	}
	var legacy []*model.Note
	for rows.Next() {
		n := &model.Note{}
		var pinned int
		var wsID sql.NullString
		if err := rows.Scan(&n.ID, &n.Title, &n.Slug, &n.Body, &n.Tags, &pinned, &wsID, &n.CreatedAt, &n.UpdatedAt, &n.ArchivedAt); err != nil {
			rows.Close()
			return fmt.Errorf("reading notes: %w", err)
		}
		n.Pinned = pinned != 0
		n.WorkspaceID = wsID.String
		legacy = append(legacy, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading notes: %w", err)
	}

	for _, n := range legacy {
		wsName, err := d.workspaceNameForFile(n.WorkspaceID)
		if err != nil {
			return err
		}
		doc := &vault.Doc{Body: n.Body}
		doc.SetID(n.ID)
		doc.SetTitle(n.Title)
		doc.SetTags(n.TagList())
		doc.SetPinned(n.Pinned)
		doc.SetWorkspace(wsName)
		created := n.CreatedAt.UTC().Truncate(time.Second)
		doc.SetCreated(&created)
		doc.SetArchived(n.ArchivedAt)

		entry, err := d.createOrAdopt(n.Slug+".md", doc)
		if errors.Is(err, vault.ErrExists) {
			entry, err = d.createOrAdopt(n.Slug+"-"+shortID(n.ID)+".md", doc)
		}
		if err != nil {
			return err
		}
		if entry, err = d.backdate(entry, n.UpdatedAt); err != nil {
			return err
		}
		if _, err := d.conn.Exec(
			"UPDATE notes SET path = ?, mtime = ?, size = ?, updated_at = ? WHERE id = ?",
			entry.Path, entry.ModTime.UnixNano(), entry.Size, entry.ModTime.UTC(), n.ID,
		); err != nil {
			return fmt.Errorf("recording %s: %w", entry.Path, err)
		}
	}
	return nil
}

// createOrAdopt creates the file, or takes over one already holding this
// note: a previous export that wrote the file but stopped (or lost a race
// with another kb) before recording it.
func (d *DB) createOrAdopt(rel string, doc *vault.Doc) (vault.Entry, error) {
	entry, err := d.vault.Create(rel, doc)
	if !errors.Is(err, vault.ErrExists) {
		return entry, err
	}
	if existing, readErr := d.vault.Read(rel); readErr == nil && existing.ID() == doc.ID() {
		return d.vault.Stat(rel)
	}
	return entry, err
}

// backdate sets a file's mtime, which the index reads as the note's
// last-modified time.
func (d *DB) backdate(entry vault.Entry, t time.Time) (vault.Entry, error) {
	abs, err := d.vault.Abs(entry.Path)
	if err != nil {
		return entry, err
	}
	if err := os.Chtimes(abs, t, t); err != nil {
		return entry, fmt.Errorf("setting time on %s: %w", entry.Path, err)
	}
	return d.vault.Stat(entry.Path)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// rewriteWorkspaceInFiles records a renamed workspace in its notes' files.
func (d *DB) rewriteWorkspaceInFiles(workspaceID string) error {
	wsName, err := d.workspaceNameForFile(workspaceID)
	if err != nil {
		return err
	}
	rows, err := d.conn.Query("SELECT id, path FROM notes WHERE workspace_id = ? AND path IS NOT NULL", workspaceID)
	if err != nil {
		return fmt.Errorf("listing workspace notes: %w", err)
	}
	type target struct{ id, rel string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.rel); err != nil {
			rows.Close()
			return fmt.Errorf("listing workspace notes: %w", err)
		}
		targets = append(targets, t)
	}
	rows.Close()

	// Keep going past a bad file so one note cannot strand the rest under
	// the old name.
	var errs []error
	for _, t := range targets {
		rel, doc, err := d.fileForWrite(t.id, true, false)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", t.rel, err))
			continue
		}
		doc.SetWorkspace(wsName)
		if err := d.writeAndIndex(rel, doc); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Rebuild re-reads every file in the vault, as if each had just changed.
// Note ids, and so links and publish history, are kept.
// The search index is refilled too, in case its rowids drifted from the
// notes' (see migrate007).
func (d *DB) Rebuild() (ScanResult, error) {
	if _, err := d.conn.Exec(`
		UPDATE notes SET mtime = 0;
		DELETE FROM notes_fts;
		INSERT INTO notes_fts (rowid, id, title, body, tags) SELECT rowid, id, title, body, tags FROM notes;
	`); err != nil {
		return ScanResult{}, fmt.Errorf("resetting index: %w", err)
	}
	return d.Scan()
}
