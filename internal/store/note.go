package store

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

// Notes live in the vault as Markdown files. Reads come from the index;
// writes go to the file first and then re-index it, so the file is always
// the version that counts.

const noteColumns = `id, title, slug, body, tags, pinned, workspace_id, created_at, updated_at, archived_at, path`

func (d *DB) CreateNote(title, slug, body, workspaceID string) (*model.Note, error) {
	if err := model.ValidateNoteTitle(title); err != nil {
		return nil, err
	}
	if err := model.ValidateNoteSlug(slug); err != nil {
		return nil, err
	}
	var taken int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM notes WHERE slug = ?", slug).Scan(&taken); err != nil {
		return nil, fmt.Errorf("checking slug: %w", err)
	}
	if taken > 0 {
		return nil, fmt.Errorf("note with slug %q already exists", slug)
	}

	return d.CreateNoteAt(noteFileName(title, slug), title, body, workspaceID)
}

func (d *DB) GetNote(id string) (*model.Note, error) {
	note, err := scanNote(d.conn.QueryRow(
		`SELECT `+noteColumns+` FROM notes WHERE id = ? AND archived_at IS NULL`, id))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	return note, nil
}

func (d *DB) GetNoteBySlug(slug string) (*model.Note, error) {
	note, err := scanNote(d.conn.QueryRow(
		`SELECT `+noteColumns+` FROM notes WHERE slug = ? AND archived_at IS NULL`, slug))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("note %q not found", slug)
	}
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	return note, nil
}

// GetNoteByPath finds the live note in the file at rel.
func (d *DB) GetNoteByPath(rel string) (*model.Note, error) {
	note, err := scanNote(d.conn.QueryRow(
		`SELECT `+noteColumns+` FROM notes WHERE path = ? AND archived_at IS NULL`, rel))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no note at %s", rel)
	}
	if err != nil {
		return nil, fmt.Errorf("querying note: %w", err)
	}
	return note, nil
}

func (d *DB) ListNotes() ([]*model.Note, error) {
	rows, err := d.conn.Query(
		`SELECT ` + noteColumns + ` FROM notes WHERE archived_at IS NULL
		 ORDER BY pinned DESC, updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing notes: %w", err)
	}
	defer rows.Close()
	return scanNotes(rows)
}

// SearchNotes finds notes containing every word of the query, each as a
// word prefix, ignoring case and accents, best match first.
func (d *DB) SearchNotes(query string) ([]*model.Note, error) {
	match := ftsQuery(query)
	if match == "" {
		return nil, nil
	}
	rows, err := d.conn.Query(
		`SELECT `+qualifiedNoteColumns+` FROM notes n
		 JOIN (SELECT id, bm25(notes_fts) AS rank FROM notes_fts WHERE notes_fts MATCH ?) f ON f.id = n.id
		 WHERE n.archived_at IS NULL
		 ORDER BY f.rank`,
		match,
	)
	if err != nil {
		return nil, fmt.Errorf("searching notes: %w", err)
	}
	defer rows.Close()
	return scanNotes(rows)
}

var qualifiedNoteColumns = "n." + strings.ReplaceAll(noteColumns, ", ", ", n.")

// ftsQuery turns free text into an FTS5 query of quoted prefix terms, so
// that FTS syntax in the input (quotes, OR, -, parentheses) is just text.
func ftsQuery(query string) string {
	words := strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = `"` + w + `"*`
	}
	return strings.Join(terms, " ")
}

func (d *DB) ListNotesByWorkspace(workspaceID string) ([]*model.Note, error) {
	rows, err := d.conn.Query(
		`SELECT `+noteColumns+` FROM notes WHERE workspace_id = ? AND archived_at IS NULL
		 ORDER BY pinned DESC, updated_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing notes by workspace: %w", err)
	}
	defer rows.Close()
	return scanNotes(rows)
}

// SetNoteWorkspace changes only the workspace key of the file as it is now.
func (d *DB) SetNoteWorkspace(noteID, workspaceID string) error {
	wsName, err := d.workspaceNameForFile(workspaceID)
	if err != nil {
		return err
	}
	rel, doc, err := d.fileForWrite(noteID, false, false)
	if err != nil {
		return err
	}
	doc.SetID(noteID)
	doc.SetWorkspace(wsName)
	return d.writeAndIndex(rel, doc)
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

// UpdateNote writes the note's fields into its file. Frontmatter keys kb
// does not manage are kept. The slug, and so the file name, never changes.
// The note carries a whole body, so if the file changed since it was read
// the save is refused rather than overwrite that edit.
func (d *DB) UpdateNote(note *model.Note) error {
	if err := model.ValidateNoteTitle(note.Title); err != nil {
		return err
	}
	wsName, err := d.workspaceNameForFile(note.WorkspaceID)
	if err != nil {
		return err
	}
	rel, doc, err := d.fileForWrite(note.ID, false, true)
	if err != nil {
		return err
	}
	doc.SetID(note.ID)

	// A file's name is its title unless the frontmatter says otherwise, so
	// an untitled file keeps no title key while the two agree.
	if doc.Title() != "" || note.Title != fileStem(rel) {
		doc.SetTitle(note.Title)
	}
	doc.SetTags(note.TagList())
	doc.SetPinned(note.Pinned)
	doc.SetWorkspace(wsName)
	doc.Body = note.Body

	entry, err := d.vault.Write(rel, doc)
	if err != nil {
		return err
	}
	indexed, err := d.indexOne(entry, doc)
	if err != nil {
		return err
	}
	*note = *indexed
	return nil
}

// ArchiveNote marks the file as it is now; nothing else in it changes.
func (d *DB) ArchiveNote(id string) error {
	rel, doc, err := d.fileForWrite(id, false, false)
	if err != nil {
		return err
	}
	doc.SetID(id)
	now := time.Now().UTC().Truncate(time.Second)
	doc.SetArchived(&now)
	return d.writeAndIndex(rel, doc)
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

// DeleteNote removes the note's file and everything the index holds about
// it, including its publish history.
func (d *DB) DeleteNote(id string) error {
	_, _, err := d.fileForWrite(id, true, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		rel, _ := d.notePath(id)
		if err := d.vault.Remove(rel); err != nil {
			return err
		}
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()
	if err := removeNoteRows(tx, id); err != nil {
		return err
	}
	if err := resolveLinks(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrConflict means a note's file changed after kb read it. The index has
// been refreshed, so reading the note again gives the current version.
var ErrConflict = errors.New("note changed on disk since it was read; reload it and try again")

// fileForWrite reads a note's file for a change, refusing a file that now
// holds a different note (replaced or renamed over). With unchanged it also
// refuses a file edited since the index read it. A caller changing the note
// itself should SetID on the doc, pinning its identity to the file so it
// survives a rename. Either refusal rescans, so
// the index catches up with the vault. A missing file is fs.ErrNotExist.
func (d *DB) fileForWrite(id string, includeArchived, unchanged bool) (string, *vault.Doc, error) {
	query := "SELECT path, mtime, size FROM notes WHERE id = ?"
	if !includeArchived {
		query += " AND archived_at IS NULL"
	}
	var rel sql.NullString
	var mtime, size int64
	err := d.conn.QueryRow(query, id).Scan(&rel, &mtime, &size)
	if err == sql.ErrNoRows {
		return "", nil, fmt.Errorf("note not found or already archived")
	}
	if err != nil {
		return "", nil, fmt.Errorf("querying note: %w", err)
	}
	if !rel.Valid {
		return "", nil, fmt.Errorf("note %s has no file in the vault", id)
	}
	entry, err := d.vault.Stat(rel.String)
	if err != nil {
		return "", nil, fmt.Errorf("note file %s: %w", rel.String, err)
	}
	if unchanged && (entry.ModTime.UnixNano() != mtime || entry.Size != size) {
		d.Scan()
		return "", nil, fmt.Errorf("%s: %w", rel.String, ErrConflict)
	}
	doc, err := d.vault.Read(rel.String)
	if err != nil {
		return "", nil, fmt.Errorf("reading note file: %w", err)
	}
	if other := doc.ID(); other != "" && other != id {
		d.Scan()
		return "", nil, fmt.Errorf("%s now holds a different note (id %s): %w", rel.String, other, ErrConflict)
	}
	return rel.String, doc, nil
}

func (d *DB) writeAndIndex(rel string, doc *vault.Doc) error {
	entry, err := d.vault.Write(rel, doc)
	if err != nil {
		return err
	}
	_, err = d.indexOne(entry, doc)
	return err
}

// notePath is the vault path of the note with this id.
func (d *DB) notePath(id string) (string, error) {
	var rel sql.NullString
	if err := d.conn.QueryRow("SELECT path FROM notes WHERE id = ?", id).Scan(&rel); err != nil {
		return "", fmt.Errorf("querying note: %w", err)
	}
	return rel.String, nil
}

// workspaceNameForFile is the name a note file records for its workspace:
// nothing for the default workspace, so most files carry no workspace key.
func (d *DB) workspaceNameForFile(workspaceID string) (string, error) {
	if workspaceID == "" {
		return "", nil
	}
	ws, err := d.GetWorkspace(workspaceID)
	if err != nil {
		return "", err
	}
	if ws.Name == model.DefaultWorkspaceName {
		return "", nil
	}
	return ws.Name, nil
}

func fileStem(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanNote(row rowScanner) (*model.Note, error) {
	note := &model.Note{}
	var pinned int
	var wsID, rel sql.NullString
	if err := row.Scan(
		&note.ID, &note.Title, &note.Slug, &note.Body, &note.Tags,
		&pinned, &wsID, &note.CreatedAt, &note.UpdatedAt, &note.ArchivedAt, &rel,
	); err != nil {
		return nil, err
	}
	note.Pinned = pinned != 0
	note.WorkspaceID = wsID.String
	note.Path = rel.String
	return note, nil
}

func scanNotes(rows *sql.Rows) ([]*model.Note, error) {
	var notes []*model.Note
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning note: %w", err)
		}
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
