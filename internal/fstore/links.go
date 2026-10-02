package fstore

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

// Link is one wikilink from a note or a card to a note or a board.
// TargetID is empty for a link that names nothing in the vault.
type Link struct {
	SourceType string // "note" or "card"
	SourceID   string // a note id, or "<board path>#<card id>"
	TargetType string // "note", "board", or "card" for [[card:…]]
	TargetID   string
	TargetRef  string // the link as written
	Context    string // the line it is on
}

// Backlink is a live note or card that links to a note.
type Backlink struct {
	SourceType string
	SourceID   string
	Slug       string // notes only
	Title      string
	Board      string // cards only: the board's name
	Context    string
}

type target struct{ typ, id string }

type resolver struct {
	slug, title, path, alias map[string]target
}

// resolver finds what a wikilink names, the way Obsidian reads it: by
// slug, then title, then file path (whole or any trailing part), then
// alias, then the ref's slug, ignoring case. When several files match, the
// shortest path wins. Notes come before boards.
func (s *Store) resolver() *resolver {
	r := &resolver{
		slug: map[string]target{}, title: map[string]target{},
		path: map[string]target{}, alias: map[string]target{},
	}
	claim := func(m map[string]target, key string, t target) {
		if _, taken := m[key]; !taken && key != "" {
			m[key] = t
		}
	}
	notes := append([]*Note{}, s.notes...)
	sort.Slice(notes, func(i, j int) bool { return shorterFirst(notes[i].Path, notes[j].Path) })
	for _, n := range notes {
		if n.ArchivedAt != nil {
			continue
		}
		t := target{"note", n.ID}
		claim(r.slug, n.Slug, t)
		claim(r.title, strings.ToLower(n.Title), t)
		for _, a := range s.aliases[n.ID] {
			claim(r.alias, strings.ToLower(a), t)
		}
		for _, suffix := range pathSuffixes(n.Path) {
			claim(r.path, suffix, t)
		}
	}
	boards := append([]*boardFile{}, s.boards...)
	sort.Slice(boards, func(i, j int) bool { return shorterFirst(boards[i].path, boards[j].path) })
	for _, b := range boards {
		t := target{"board", b.path}
		claim(r.title, strings.ToLower(b.name()), t)
		for _, suffix := range pathSuffixes(b.path) {
			claim(r.path, suffix, t)
		}
	}
	return r
}

func pathSuffixes(rel string) []string {
	lower := strings.ToLower(rel)
	out := []string{lower}
	for {
		i := strings.Index(lower, "/")
		if i < 0 {
			return out
		}
		lower = lower[i+1:]
		out = append(out, lower)
	}
}

// refName is what a wikilink ref names: [[Name#Heading]], [[Name^block]]
// and [[Name.md]] all name "Name".
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

func (r *resolver) resolve(ref string) target {
	name := refName(ref)
	if name == "" {
		return target{}
	}
	lower := strings.ToLower(name)
	for _, t := range []target{
		r.slug[name], r.title[lower], r.path[lower+".md"], r.alias[lower], r.slug[model.Slugify(name)],
	} {
		if t.typ != "" {
			return t
		}
	}
	return target{}
}

// buildLinks reads every wikilink in live notes and cards.
func (s *Store) buildLinks() {
	r := s.resolver()
	s.links = nil
	add := func(sourceType, sourceID, text string) {
		for _, pl := range model.ParseWikilinks(text) {
			l := Link{SourceType: sourceType, SourceID: sourceID, TargetType: pl.TargetType, TargetRef: pl.TargetRef, Context: pl.Context}
			if pl.TargetType == "note" {
				if refName(pl.TargetRef) == "" {
					continue // [[#Heading]]: a place in the same note
				}
				t := r.resolve(pl.TargetRef)
				if t.typ == "note" && sourceType == "note" && t.id == sourceID {
					continue // a note linking to itself
				}
				l.TargetType, l.TargetID = t.typ, t.id
				if t.typ == "" {
					l.TargetType = "note"
				}
			}
			s.links = append(s.links, l)
		}
	}
	for _, n := range s.notes {
		if n.ArchivedAt == nil {
			add("note", n.ID, n.Body)
		}
	}
	for _, b := range s.boards {
		for _, lane := range b.b.Lanes {
			for _, it := range lane.Items() {
				add("card", b.path+"#"+it.ID, it.RawText())
			}
		}
	}
}

// Links lists every link, for the graph.
func (s *Store) Links() []Link { return s.links }

// Backlinks lists the live notes and cards linking to a note, notes first.
func (s *Store) Backlinks(noteID string) []Backlink {
	var out []Backlink
	seen := map[string]bool{}
	for _, l := range s.links {
		if l.TargetType != "note" || l.TargetID != noteID || seen[l.SourceType+l.SourceID] {
			continue
		}
		seen[l.SourceType+l.SourceID] = true
		switch l.SourceType {
		case "note":
			if n, ok := s.byID[l.SourceID]; ok {
				out = append(out, Backlink{SourceType: "note", SourceID: n.ID, Slug: n.Slug, Title: n.Title, Context: l.Context})
			}
		case "card":
			boardPath, cardID, _ := strings.Cut(l.SourceID, "#")
			if b := s.boardByPath(boardPath); b != nil {
				if it, _ := b.b.Find(cardID); it != nil {
					out = append(out, Backlink{SourceType: "card", SourceID: cardID, Title: it.Title, Board: b.name(), Context: l.Context})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SourceType != out[j].SourceType {
			return out[i].SourceType == "note"
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// RenameNote gives a note a new title and a file named after it, in the
// same folder, and rewrites every link that names it by its old title,
// slug or file so it still resolves: [[Old#Part|text]] becomes
// [[New#Part|text]]. Links through an alias still resolve and are left as
// written. It returns the renamed note and how many other files changed.
func (s *Store) RenameNote(id, newTitle string) (*Note, int, error) {
	if err := model.ValidateNoteTitle(newTitle); err != nil {
		return nil, 0, err
	}
	old, err := s.GetNote(id)
	if err != nil {
		return nil, 0, err
	}
	newSlug := model.Slugify(newTitle)
	if newSlug == "" {
		return nil, 0, fmt.Errorf("%q has no letters or digits to name a file after", newTitle)
	}
	for _, n := range s.ListNotes() {
		if n.ID == id {
			continue
		}
		if n.Slug == newSlug || strings.EqualFold(n.Title, newTitle) {
			return nil, 0, fmt.Errorf("another note is already called %q (%s)", newTitle, n.Path)
		}
		for _, a := range s.aliases[n.ID] {
			if strings.EqualFold(a, newTitle) {
				return nil, 0, fmt.Errorf("another note answers to %q (%s)", newTitle, n.Path)
			}
		}
	}

	r := s.resolver()
	oldPath := strings.ToLower(old.Path)
	namesNote := func(name string) bool {
		lower := strings.ToLower(name)
		return name == old.Slug || lower == strings.ToLower(old.Title) ||
			oldPath == lower+".md" || strings.HasSuffix(oldPath, "/"+lower+".md")
	}
	rewrite := func(t string) (string, bool) {
		if strings.HasPrefix(t, "card:") || strings.HasPrefix(t, "board:") {
			return "", false
		}
		name := refName(t)
		if name == "" || !namesNote(name) || r.resolve(t) != (target{"note", id}) {
			return "", false
		}
		return newTitle + strings.TrimSpace(t)[len(strings.TrimSpace(name)):], true
	}

	newRel := path.Join(path.Dir(old.Path), noteFileName(newTitle, ""))
	inPlace := strings.EqualFold(newRel, old.Path) // one file on a case-insensitive disk
	if inPlace {
		newRel = old.Path
	}
	err = s.writeLocked(old.Path, func() error {
		doc, err := s.readForWrite(old, "")
		if err != nil {
			return err
		}
		doc.SetTitle("")
		if newTitle != stem(newRel) {
			doc.SetTitle(newTitle)
		}
		doc.Body, _ = model.RewriteWikilinks(doc.Body, rewrite)
		if inPlace {
			_, err = s.vault.Write(old.Path, doc)
			return err
		}
		if _, err := s.vault.Create(newRel, doc); err != nil {
			if errors.Is(err, vault.ErrExists) {
				return fmt.Errorf("a file already exists at %s", newRel)
			}
			return err
		}
		if err := s.vault.Remove(old.Path); err != nil {
			if undo := s.vault.Remove(newRel); undo != nil {
				return fmt.Errorf("%w; the note is now in both %s and %s", err, old.Path, newRel)
			}
			return err
		}
		return nil
	})
	if err != nil {
		s.Reload()
		return nil, 0, err
	}

	changed, errs := s.rewriteSources(id, rewrite)
	if err := s.Reload(); err != nil {
		return nil, changed, err
	}
	note, err := s.GetNote(id)
	if err != nil {
		return nil, changed, err
	}
	return note, changed, errors.Join(errs...)
}

// rewriteSources rewrites the links to a note in the notes and boards that
// link to it. A source changed since it was read is skipped and reported.
func (s *Store) rewriteSources(id string, rewrite func(string) (string, bool)) (int, []error) {
	notes := map[string]bool{}
	boards := map[string]bool{}
	for _, l := range s.links {
		if l.TargetType != "note" || l.TargetID != id {
			continue
		}
		if l.SourceType == "note" && l.SourceID != id {
			notes[l.SourceID] = true
		}
		if l.SourceType == "card" {
			p, _, _ := strings.Cut(l.SourceID, "#")
			boards[p] = true
		}
	}
	changed := 0
	var errs []error
	for srcID := range notes {
		src := s.byID[srcID]
		err := s.writeLocked(src.Path, func() error {
			doc, err := s.readForWrite(src, src.Rev)
			if err != nil {
				return err
			}
			body, n := model.RewriteWikilinks(doc.Body, rewrite)
			if n == 0 {
				return nil
			}
			doc.Body = body
			if doc.ID() == pathID(src.Path) {
				// Keep a file with no id of its own as it was.
				doc.SetID("")
			}
			if _, err := s.vault.Write(src.Path, doc); err != nil {
				return err
			}
			changed++
			return nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: links not updated: %w", src.Path, err))
		}
	}
	for p := range boards {
		n, err := s.rewriteBoardLinks(p, rewrite)
		changed += n
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: links not updated: %w", p, err))
		}
	}
	return changed, errs
}

// rewriteNoteFile is used by workspace renames: it rewrites one key of a
// note file as it is now.
func (s *Store) readNoteFile(rel string) (*vault.Doc, error) {
	abs, err := s.vault.Abs(rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return vault.Parse(data)
}
