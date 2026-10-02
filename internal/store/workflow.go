package store

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

// noteFileName is the vault file name for a new or renamed note: its
// title, as Obsidian names notes, cleaned of characters a file name cannot
// hold. A slug given explicitly (one that is not just the title's slug)
// names the file instead, so `kb note create --slug q3` gives q3.md.
func noteFileName(title, slug string) string {
	if slug != "" && slug != model.Slugify(title) {
		return slug + ".md"
	}
	if name := model.FileName(title); name != "" {
		return name + ".md"
	}
	if slug != "" {
		return slug + ".md"
	}
	return "Untitled.md"
}

// CreateNoteAt creates a note in a new file at rel, a vault path ending in
// .md, such as "daily/2026-10-02.md".
func (d *DB) CreateNoteAt(rel, title, body, workspaceID string) (*model.Note, error) {
	if err := model.ValidateNoteTitle(title); err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(rel), ".md") {
		return nil, fmt.Errorf("note path %q must end in .md", rel)
	}
	if _, err := d.vault.Abs(rel); err != nil {
		return nil, err
	}
	wsName, err := d.workspaceNameForFile(workspaceID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Truncate(time.Second)
	doc := &vault.Doc{Body: body}
	doc.SetID(uuid.New().String())
	if title != fileStem(rel) {
		doc.SetTitle(title)
	}
	doc.SetWorkspace(wsName)
	doc.SetCreated(&now)

	entry, err := d.vault.Create(rel, doc)
	if errors.Is(err, vault.ErrExists) {
		return nil, fmt.Errorf("a note already exists at %s", rel)
	}
	if err != nil {
		return nil, err
	}
	return d.indexOne(entry, doc)
}

// ResolveNoteRef finds a note by any name a wikilink could use: slug,
// title, file name or alias, ignoring case.
func (d *DB) ResolveNoteRef(ref string) (*model.Note, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	r, err := loadLinkResolver(tx)
	tx.Rollback()
	if err != nil {
		return nil, err
	}
	id := r.resolve(ref)
	if id == "" {
		return nil, fmt.Errorf("no note named %q", ref)
	}
	return d.GetNote(id)
}

type TagCount struct {
	Tag   string
	Count int
}

// ListTags counts live notes per tag, ignoring case, most used first.
func (d *DB) ListTags() ([]TagCount, error) {
	notes, err := d.ListNotes()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, n := range notes {
		for _, tag := range n.TagList() {
			counts[strings.ToLower(tag)]++
		}
	}
	tags := make([]TagCount, 0, len(counts))
	for tag, count := range counts {
		tags = append(tags, TagCount{tag, count})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count != tags[j].Count {
			return tags[i].Count > tags[j].Count
		}
		return tags[i].Tag < tags[j].Tag
	})
	return tags, nil
}

// RenameNote gives a note a new title and a file named after it, in the
// same folder, and rewrites every link to the note so it still resolves:
// [[Old Name#Part|text]] becomes [[New Name#Part|text]]. It returns the
// renamed note and how many other notes had links rewritten.
func (d *DB) RenameNote(id, newTitle string) (*model.Note, int, error) {
	if err := model.ValidateNoteTitle(newTitle); err != nil {
		return nil, 0, err
	}
	newSlug := model.Slugify(newTitle)
	if newSlug == "" {
		return nil, 0, fmt.Errorf("%q has no letters or digits to name a file after", newTitle)
	}
	var taken int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM notes WHERE slug = ? AND id != ?", newSlug, id).Scan(&taken); err != nil {
		return nil, 0, fmt.Errorf("checking name: %w", err)
	}
	if taken > 0 {
		return nil, 0, fmt.Errorf("a note named %q already exists", newSlug)
	}
	// Links are rewritten to the new title, so it must name only this note:
	// another note with that title or alias would capture them.
	if err := d.conn.QueryRow(
		`SELECT COUNT(*) FROM notes n WHERE n.id != ? AND n.archived_at IS NULL
		   AND (n.title = ? COLLATE NOCASE
		        OR EXISTS (SELECT 1 FROM note_aliases a WHERE a.note_id = n.id AND a.alias = ?))`,
		id, newTitle, newTitle,
	).Scan(&taken); err != nil {
		return nil, 0, fmt.Errorf("checking name: %w", err)
	}
	if taken > 0 {
		return nil, 0, fmt.Errorf("another note is already called %q", newTitle)
	}
	old, err := d.GetNote(id)
	if err != nil {
		return nil, 0, err
	}

	rel, doc, err := d.fileForWrite(id, false, false)
	if err != nil {
		return nil, 0, err
	}
	// The scan after the move finds the note again by this id.
	doc.SetID(id)

	// Which links name this note has to be decided before it is renamed.
	tx, err := d.conn.Begin()
	if err != nil {
		return nil, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	r, err := loadLinkResolver(tx)
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}
	rows, err := tx.Query(
		`SELECT DISTINCT source_id FROM links
		 WHERE source_type = 'note' AND target_type = 'note' AND target_id = ? AND source_id != ?`,
		id, id,
	)
	if err != nil {
		tx.Rollback()
		return nil, 0, fmt.Errorf("finding links: %w", err)
	}
	var sources []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			rows.Close()
			tx.Rollback()
			return nil, 0, fmt.Errorf("finding links: %w", err)
		}
		sources = append(sources, src)
	}
	rows.Close()
	tx.Rollback()

	// Rewrite links that name the note by its title, slug or file, which
	// the rename changes. A link through an alias still resolves, so it
	// keeps the words its author wrote; card and board links are not notes.
	oldPath := strings.ToLower(rel)
	namesNote := func(name string) bool {
		lower := strings.ToLower(name)
		return name == old.Slug || lower == strings.ToLower(old.Title) ||
			oldPath == lower+".md" || strings.HasSuffix(oldPath, "/"+lower+".md")
	}
	rewrite := func(target string) (string, bool) {
		if strings.HasPrefix(target, "card:") || strings.HasPrefix(target, "board:") {
			return "", false
		}
		name := refName(target)
		if name == "" || !namesNote(name) || r.resolve(target) != id {
			return "", false
		}
		return newTitle + strings.TrimSpace(target)[len(strings.TrimSpace(name)):], true
	}

	newRel := path.Join(path.Dir(rel), noteFileName(newTitle, ""))
	// Names differing only in case are one file on a case-insensitive disk
	// (macOS, Windows), so that is a retitle in place, not a move.
	inPlace := strings.EqualFold(newRel, rel)
	if inPlace {
		newRel = rel
	}
	doc.SetTitle("")
	if newTitle != fileStem(newRel) {
		doc.SetTitle(newTitle)
	}
	doc.Body, _ = model.RewriteWikilinks(doc.Body, rewrite)
	if inPlace {
		if _, err := d.vault.Write(rel, doc); err != nil {
			return nil, 0, err
		}
	} else {
		if _, err := d.vault.Create(newRel, doc); err != nil {
			if errors.Is(err, vault.ErrExists) {
				return nil, 0, fmt.Errorf("a file already exists at %s", newRel)
			}
			return nil, 0, err
		}
		if err := d.vault.Remove(rel); err != nil {
			// Undo, or the note would exist twice under one id.
			if undoErr := d.vault.Remove(newRel); undoErr != nil {
				return nil, 0, fmt.Errorf("%w; the note is now in both %s and %s", err, rel, newRel)
			}
			return nil, 0, err
		}
	}

	changed := 0
	var errs []error
	for _, src := range sources {
		srcRel, srcDoc, err := d.fileForWrite(src, true, false)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		body, n := model.RewriteWikilinks(srcDoc.Body, rewrite)
		if n == 0 {
			continue
		}
		srcDoc.Body = body
		if _, err := d.vault.Write(srcRel, srcDoc); err != nil {
			errs = append(errs, err)
			continue
		}
		changed++
	}

	// The scan sees the move (the file keeps its id) and the rewritten
	// links, and re-resolves everything.
	if _, err := d.Scan(); err != nil {
		return nil, changed, err
	}
	note, err := d.GetNote(id)
	if err != nil {
		return nil, changed, err
	}
	return note, changed, errors.Join(errs...)
}
