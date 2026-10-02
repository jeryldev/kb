package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
)

func (d *DB) SyncNoteLinks(note *model.Note) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()
	if err := syncLinksTx(tx, note); err != nil {
		return err
	}
	if err := resolveLinks(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// syncLinksTx records the links written in a note. Note links are stored
// unresolved (their target is the ref text); resolveLinks gives them their
// targets, and must run before the transaction commits.
func syncLinksTx(tx *sql.Tx, note *model.Note) error {
	if _, err := tx.Exec(
		"DELETE FROM links WHERE source_type = 'note' AND source_id = ?", note.ID,
	); err != nil {
		return fmt.Errorf("clearing old links: %w", err)
	}

	for _, pl := range model.ParseWikilinks(note.Body) {
		if pl.TargetType == "note" && refName(pl.TargetRef) == "" {
			continue // [[#Heading]] or [[^block]]: a place in this same note
		}
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO links (id, source_type, source_id, target_type, target_id, target_ref, context)
			 VALUES (?, 'note', ?, ?, ?, ?, ?)`,
			uuid.New().String(), note.ID, pl.TargetType, pl.TargetRef, pl.TargetRef, pl.Context,
		); err != nil {
			return fmt.Errorf("inserting link: %w", err)
		}
	}
	return nil
}

// refName is the note a wikilink ref names: [[Name#Heading]],
// [[Name^block]] and [[Name.md]] all name "Name".
func refName(ref string) string {
	if i := strings.IndexAny(ref, "#^"); i >= 0 {
		ref = ref[:i]
	}
	ref = strings.TrimSpace(ref)
	if len(ref) > 3 && strings.EqualFold(ref[len(ref)-3:], ".md") {
		ref = ref[:len(ref)-3]
	}
	return ref
}

// linkResolver finds the note a wikilink names, the way Obsidian reads it:
// by slug, then title, then file path (whole or any trailing part, so
// [[folder/Name]] and [[Name]] both work), then alias, then the ref's slug,
// ignoring case. When several notes match, the shortest path wins.
type linkResolver struct {
	slug, title, path, alias map[string]string
}

func loadLinkResolver(tx *sql.Tx) (*linkResolver, error) {
	r := &linkResolver{
		slug: map[string]string{}, title: map[string]string{},
		path: map[string]string{}, alias: map[string]string{},
	}
	// Shortest path first, so the first note to claim a key keeps it.
	rows, err := tx.Query(
		`SELECT n.id, n.slug, n.title, COALESCE(n.path, ''), COALESCE(a.alias, '')
		 FROM notes n LEFT JOIN note_aliases a ON a.note_id = n.id
		 WHERE n.archived_at IS NULL
		 ORDER BY length(n.path), n.path`,
	)
	if err != nil {
		return nil, fmt.Errorf("loading notes for links: %w", err)
	}
	defer rows.Close()
	claim := func(m map[string]string, key, id string) {
		if _, taken := m[key]; !taken && key != "" {
			m[key] = id
		}
	}
	for rows.Next() {
		var id, slug, title, rel, alias string
		if err := rows.Scan(&id, &slug, &title, &rel, &alias); err != nil {
			return nil, fmt.Errorf("loading notes for links: %w", err)
		}
		claim(r.slug, slug, id)
		claim(r.title, strings.ToLower(title), id)
		claim(r.alias, strings.ToLower(alias), id)
		lower := strings.ToLower(rel)
		for {
			claim(r.path, lower, id)
			i := strings.Index(lower, "/")
			if i < 0 {
				break
			}
			lower = lower[i+1:]
		}
	}
	return r, rows.Err()
}

// resolve returns the id of the note ref names, or "".
func (r *linkResolver) resolve(ref string) string {
	name := refName(ref)
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	for _, id := range []string{
		r.slug[name], r.title[lower], r.path[lower+".md"], r.alias[lower], r.slug[model.Slugify(name)],
	} {
		if id != "" {
			return id
		}
	}
	return ""
}

// resolveLinks points every note link at the note its ref names now, so
// links follow the vault as notes appear, move, are renamed or deleted. A
// link that names no note, or only the note it is written in, keeps its
// ref text as its target.
func resolveLinks(tx *sql.Tx) error {
	r, err := loadLinkResolver(tx)
	if err != nil {
		return err
	}
	rows, err := tx.Query(
		"SELECT id, source_type, source_id, target_id, target_ref FROM links WHERE target_type = 'note'",
	)
	if err != nil {
		return fmt.Errorf("reading links: %w", err)
	}
	type change struct{ id, target string }
	var changes []change
	for rows.Next() {
		var id, sourceType, source, target, ref string
		if err := rows.Scan(&id, &sourceType, &source, &target, &ref); err != nil {
			rows.Close()
			return fmt.Errorf("reading links: %w", err)
		}
		want := r.resolve(ref)
		if want == "" || (sourceType == "note" && want == source) {
			want = ref
		}
		if want != target {
			changes = append(changes, change{id, want})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading links: %w", err)
	}

	for _, c := range changes {
		// OR REPLACE: the source may already link to that note under another
		// name, and one link per pair is all the table keeps.
		if _, err := tx.Exec("UPDATE OR REPLACE links SET target_id = ? WHERE id = ?", c.target, c.id); err != nil {
			return fmt.Errorf("resolving link: %w", err)
		}
	}
	return nil
}

func (d *DB) GetForwardLinks(sourceType, sourceID string) ([]*model.Link, error) {
	rows, err := d.conn.Query(
		`SELECT id, source_type, source_id, target_type, target_id, target_ref, context, created_at
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
		`SELECT id, source_type, source_id, target_type, target_id, target_ref, context, created_at
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
		`SELECT id, source_type, source_id, target_type, target_id, target_ref, context, created_at
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
			&link.TargetType, &link.TargetID, &link.TargetRef, &link.Context, &link.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
