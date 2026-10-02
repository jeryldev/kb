package fstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

// ListNotes lists live notes, pinned first, then most recently changed.
func (s *Store) ListNotes() []*Note {
	var out []*Note
	for _, n := range s.notes {
		if n.ArchivedAt == nil {
			out = append(out, n)
		}
	}
	sortNotes(out)
	return out
}

func sortNotes(notes []*Note) {
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].Pinned != notes[j].Pinned {
			return notes[i].Pinned
		}
		return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
	})
}

func (s *Store) GetNote(id string) (*Note, error) {
	if n, ok := s.byID[id]; ok && n.ArchivedAt == nil {
		return n, nil
	}
	return nil, fmt.Errorf("note %q: %w", id, ErrNotFound)
}

func (s *Store) GetNoteBySlug(slug string) (*Note, error) {
	if n, ok := s.bySlug[slug]; ok && n.ArchivedAt == nil {
		return n, nil
	}
	return nil, fmt.Errorf("note %q: %w", slug, ErrNotFound)
}

// GetNoteByPath finds the note in the file at rel, archived or not.
func (s *Store) GetNoteByPath(rel string) (*Note, error) {
	if n, ok := s.byPath[rel]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("no note at %s: %w", rel, ErrNotFound)
}

// ResolveNote finds a live note by slug, then by any name a wikilink could
// use (title, alias, file name), then by id or id prefix (4 characters or
// more). Names come before id prefixes, so a note titled "cafe" is never
// mistaken for an id that starts with those letters.
func (s *Store) ResolveNote(ref string) (*Note, error) {
	if n, err := s.GetNoteBySlug(ref); err == nil {
		return n, nil
	}
	if target := s.resolver().resolve(ref); target.typ == "note" {
		return s.GetNote(target.id)
	}
	if n, err := s.GetNote(ref); err == nil {
		return n, nil
	}
	if len(ref) >= 4 {
		var match *Note
		for _, n := range s.notes {
			if n.ArchivedAt == nil && strings.HasPrefix(n.ID, ref) {
				if match != nil {
					return nil, fmt.Errorf("%q matches more than one note id; use more characters", ref)
				}
				match = n
			}
		}
		if match != nil {
			return match, nil
		}
	}
	return nil, fmt.Errorf("note %q: %w", ref, ErrNotFound)
}

func (s *Store) ListNotesByTag(tag string) []*Note {
	var out []*Note
	for _, n := range s.ListNotes() {
		if n.HasTag(tag) {
			out = append(out, n)
		}
	}
	return out
}

func (s *Store) ListNotesByWorkspace(workspaceID string) []*Note {
	var out []*Note
	for _, n := range s.ListNotes() {
		if n.WorkspaceID == workspaceID {
			out = append(out, n)
		}
	}
	return out
}

type TagCount struct {
	Tag   string
	Count int
}

// ListTags counts live notes per tag, ignoring case, most used first.
func (s *Store) ListTags() []TagCount {
	counts := map[string]int{}
	for _, n := range s.ListNotes() {
		for _, t := range n.TagList() {
			counts[strings.ToLower(t)]++
		}
	}
	var out []TagCount
	for t, c := range counts {
		out = append(out, TagCount{t, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// noteFileName is the file name for a new or renamed note: its title, as
// Obsidian names notes, cleaned of characters a file name cannot hold. An
// explicit slug that is not just the title's slug names the file instead.
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

// CreateNote creates a note named after its title (or slug, when one is
// given that differs from the title's).
func (s *Store) CreateNote(title, slug, body, workspaceID string) (*Note, error) {
	if err := model.ValidateNoteTitle(title); err != nil {
		return nil, err
	}
	if slug != "" {
		if err := model.ValidateNoteSlug(slug); err != nil {
			return nil, err
		}
	}
	want := slug
	if want == "" {
		want = model.Slugify(title)
	}
	if n, ok := s.bySlug[want]; ok {
		return nil, fmt.Errorf("a note named %q already exists (%s)", want, n.Path)
	}
	return s.CreateNoteAt(noteFileName(title, slug), title, body, workspaceID)
}

// CreateNoteAt creates a note in a new file at rel, a vault path ending in
// .md, such as "daily/2026-10-02.md".
func (s *Store) CreateNoteAt(rel, title, body, workspaceID string) (*Note, error) {
	if err := model.ValidateNoteTitle(title); err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(rel), ".md") {
		return nil, fmt.Errorf("note path %q must end in .md", rel)
	}
	now := time.Now().Truncate(time.Second)
	doc := &vault.Doc{Body: body}
	id := uuid.New().String()
	doc.SetID(id)
	if title != stem(rel) {
		doc.SetTitle(title)
	}
	doc.SetWorkspace(s.workspaceNameForFile(workspaceID))
	doc.SetCreated(&now)
	err := s.writeLocked(rel, func() error {
		_, err := s.vault.Create(rel, doc)
		return err
	})
	if errors.Is(err, vault.ErrExists) {
		return nil, fmt.Errorf("a note already exists at %s", rel)
	}
	if err != nil {
		return nil, err
	}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s.byID[id], nil
}

// readForWrite reads a note's file afresh for a change, under the caller's
// lock. With rev set, the file must still be the content that rev names.
func (s *Store) readForWrite(n *Note, rev string) (*vault.Doc, error) {
	abs, err := s.vault.Abs(n.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", n.Path, err)
	}
	if rev != "" && revOf(data) != rev {
		return nil, ErrConflict
	}
	doc, err := vault.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w; fix its frontmatter first", n.Path, err)
	}
	// A copy that carries another note's id gets its own id written the
	// first time kb changes it.
	doc.SetID(n.ID)
	return doc, nil
}

// UpdateNote writes the note's fields into its file. The note carries a
// whole body, so if the file changed since the note was read (its Rev),
// the save is refused with ErrConflict rather than overwrite that edit.
// Frontmatter keys kb does not manage are kept, and the workspace key is
// only rewritten when the note's workspace changed.
func (s *Store) UpdateNote(note *Note) error {
	if err := model.ValidateNoteTitle(note.Title); err != nil {
		return err
	}
	current, ok := s.byID[note.ID]
	if !ok {
		return fmt.Errorf("note %q: %w", note.ID, ErrNotFound)
	}
	err := s.writeLocked(current.Path, func() error {
		doc, err := s.readForWrite(current, note.Rev)
		if err != nil {
			return err
		}
		if doc.Title() != "" || note.Title != stem(current.Path) {
			doc.SetTitle(note.Title)
		}
		doc.SetTags(note.TagList())
		doc.SetPinned(note.Pinned)
		s.setWorkspaceKey(doc, note.WorkspaceID)
		doc.Body = note.Body
		_, err = s.vault.Write(current.Path, doc)
		return err
	})
	if reloadErr := s.Reload(); reloadErr != nil && err == nil {
		err = reloadErr
	}
	if err != nil {
		return err
	}
	*note = *s.byID[note.ID]
	return nil
}

// patchNote changes a note's file through fn, with no stale-read check:
// for edits that set one key of the file as it is now.
func (s *Store) patchNote(id string, includeArchived bool, fn func(*vault.Doc)) error {
	n, ok := s.byID[id]
	if !ok || (!includeArchived && n.ArchivedAt != nil) {
		return fmt.Errorf("note %q: %w", id, ErrNotFound)
	}
	err := s.writeLocked(n.Path, func() error {
		doc, err := s.readForWrite(n, "")
		if err != nil {
			return err
		}
		fn(doc)
		_, err = s.vault.Write(n.Path, doc)
		return err
	})
	if reloadErr := s.Reload(); reloadErr != nil && err == nil {
		err = reloadErr
	}
	return err
}

func (s *Store) ArchiveNote(id string) error {
	now := time.Now().Truncate(time.Second)
	return s.patchNote(id, false, func(d *vault.Doc) { d.SetArchived(&now) })
}

func (s *Store) SetNoteWorkspace(id, workspaceID string) error {
	return s.patchNote(id, false, func(d *vault.Doc) { s.setWorkspaceKey(d, workspaceID) })
}

// setWorkspaceKey records a workspace in a file, leaving the key alone
// when it already names that workspace (in whatever case or spelling the
// file uses), so that kb never strips a name it does not know.
func (s *Store) setWorkspaceKey(doc *vault.Doc, workspaceID string) {
	if s.workspaceIDForName(doc.Workspace()) == workspaceID {
		return
	}
	doc.SetWorkspace(s.workspaceNameForFile(workspaceID))
}

// TrashNote moves a note's file into the vault's .trash folder, as
// Obsidian does, and returns where it went. A file kb cannot parse can be
// trashed too.
func (s *Store) TrashNote(id string) (string, error) {
	n, ok := s.byID[id]
	if !ok {
		return "", fmt.Errorf("note %q: %w", id, ErrNotFound)
	}
	var dest string
	err := s.writeLocked(n.Path, func() error {
		var err error
		dest, err = s.moveToTrash(n.Path)
		return err
	})
	if reloadErr := s.Reload(); reloadErr != nil && err == nil {
		err = reloadErr
	}
	return dest, err
}

// moveToTrash moves a vault file to .trash/, keeping its folder, as
// "Name 2.md" and so on when the name is taken there.
func (s *Store) moveToTrash(rel string) (string, error) {
	src, err := s.vault.Abs(rel)
	if err != nil {
		return "", err
	}
	dir, base := path.Split(rel)
	name := strings.TrimSuffix(base, path.Ext(base))
	for n := 1; ; n++ {
		candidate := path.Join(".trash", dir, base)
		if n > 1 {
			candidate = path.Join(".trash", dir, fmt.Sprintf("%s %d%s", name, n, path.Ext(base)))
		}
		dst := filepath.Join(s.vault.Root(), filepath.FromSlash(candidate))
		if _, err := os.Lstat(dst); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return "", err
			}
			if err := os.Rename(src, dst); err != nil {
				return "", fmt.Errorf("moving %s to the trash: %w", rel, err)
			}
			return candidate, nil
		}
	}
}
