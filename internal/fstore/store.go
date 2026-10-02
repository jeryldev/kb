// Package fstore keeps kb's data as files: notes and boards are Markdown in
// the vault, workspaces are in the vault's .kb/workspaces.yml, and
// machine-local settings (publish targets) are in the config directory.
// There is no database: Open reads the vault into memory, and every write
// changes one file, under a lock, and then reads the vault again.
package fstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jeryldev/kb/internal/board"
	"github.com/jeryldev/kb/internal/model"
	"github.com/jeryldev/kb/internal/vault"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict means a file changed after kb read it. The store has
	// been reloaded, so reading again gives the current version.
	ErrConflict = errors.New("changed on disk since it was read; reload it and try again")
)

type Note = model.Note

type Options struct {
	// ConfigDir holds machine-local settings, such as publish targets
	// (their paths are paths on this machine). Default: $XDG_CONFIG_HOME/kb
	// or ~/.config/kb.
	ConfigDir string
	// LockDir holds the lock files that serialise writes to one file
	// between kb processes; kept outside the vault so they never sync.
	// Default: $XDG_CACHE_HOME/kb/locks or ~/.cache/kb/locks.
	LockDir string
}

// DefaultOptions are the usual directories for this user.
func DefaultOptions() (Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Options{}, err
	}
	dir := func(env, fallback string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		return filepath.Join(home, fallback)
	}
	return Options{
		ConfigDir: filepath.Join(dir("XDG_CONFIG_HOME", ".config"), "kb"),
		LockDir:   filepath.Join(dir("XDG_CACHE_HOME", ".cache"), "kb", "locks"),
	}, nil
}

type Store struct {
	vault *vault.Vault
	opts  Options

	notes    []*Note
	byID     map[string]*Note
	byPath   map[string]*Note
	bySlug   map[string]*Note
	aliases  map[string][]string // note id → aliases
	docs     map[string]*vault.Doc
	boards   []*boardFile
	links    []Link
	problems []string

	workspaces []*model.Workspace
}

// Open reads the vault at vaultDir.
func Open(vaultDir string, opts Options) (*Store, error) {
	s := &Store{vault: vault.New(vaultDir), opts: opts}
	if err := s.Reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Vault() *vault.Vault { return s.vault }

// Problems lists files kb could only partly read, and other things worth
// a look (duplicate ids, notes not downloaded from iCloud).
func (s *Store) Problems() []string { return s.problems }

type candidate struct {
	entry    vault.Entry
	doc      *vault.Doc
	rev      string
	dataless bool
}

// Reload reads the whole vault again.
func (s *Store) Reload() error {
	var cands []*candidate
	var boards []*boardFile
	var problems []string
	err := s.vault.WalkSkipping(func(e vault.Entry) error {
		if e.Dataless {
			problems = append(problems, fmt.Sprintf("%s is in iCloud and not downloaded; kb shows its name only", e.Path))
			cands = append(cands, &candidate{entry: e, doc: &vault.Doc{}, dataless: true})
			return nil
		}
		abs, _ := s.vault.Abs(e.Path)
		data, err := os.ReadFile(abs)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", e.Path, err))
			return nil
		}
		if board.IsBoard(data) {
			bf, err := loadBoard(e.Path, data)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", e.Path, err))
				return nil
			}
			boards = append(boards, bf)
			return nil
		}
		doc, err := vault.Parse(data)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v (read as plain text)", e.Path, err))
			doc = &vault.Doc{Body: string(data)}
		}
		cands = append(cands, &candidate{entry: e, doc: doc, rev: revOf(data)})
		return nil
	}, func(rel string, err error) {
		problems = append(problems, fmt.Sprintf("skipped %s: %v", rel, err))
	})
	if err != nil {
		return fmt.Errorf("reading the vault %s: %w", s.vault.Root(), err)
	}

	workspaces, wsProblems, err := s.loadWorkspaces(cands, boards)
	if err != nil {
		return err
	}
	problems = append(problems, wsProblems...)

	s.problems = problems
	s.workspaces = workspaces
	s.boards = boards
	s.buildNotes(cands)
	s.buildLinks()
	return nil
}

func revOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:8])
}

// pathID is the id of a note whose file names none, derived from its path
// so that every load gives it the same id without writing to the file.
func pathID(rel string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kb-vault:"+rel)).String()
}

var icloudCopyRe = regexp.MustCompile(` \d+$`)

// buildNotes gives every note its id and slug. A file's own id wins, and
// when several files carry the same id (a copy, or an iCloud conflict copy
// "Note 2.md"), the original keeps it: the shortest path, then the name
// without a " N" suffix, then the first in order. The others, and files
// with no id, get an id derived from their path.
func (s *Store) buildNotes(cands []*candidate) {
	sort.Slice(cands, func(i, j int) bool { return cands[i].entry.Path < cands[j].entry.Path })
	claims := map[string][]*candidate{}
	for _, c := range cands {
		if id := c.doc.ID(); id != "" {
			claims[id] = append(claims[id], c)
		}
	}
	ids := map[*candidate]string{}
	taken := map[string]bool{}
	for id, group := range claims {
		sort.Slice(group, func(i, j int) bool { return originalFirst(group[i].entry.Path, group[j].entry.Path) })
		ids[group[0]] = id
		taken[id] = true
		for _, dup := range group[1:] {
			s.problems = append(s.problems, fmt.Sprintf("%s has the same id as %s; kb treats it as a separate note", dup.entry.Path, group[0].entry.Path))
		}
	}
	for _, c := range cands {
		if _, ok := ids[c]; ok {
			continue
		}
		id := pathID(c.entry.Path)
		for n := 2; taken[id]; n++ {
			id = pathID(fmt.Sprintf("%s\x00%d", c.entry.Path, n))
		}
		ids[c] = id
		taken[id] = true
	}

	// Slugs: the shortest path keeps the plain slug, so a rebuilt load
	// gives every note the same slug whatever order the files are read in.
	groups := map[string][]*candidate{}
	for _, c := range cands {
		base := model.Slugify(stem(c.entry.Path))
		if base == "" {
			base = strings.Trim("note-"+model.Slugify(ids[c]), "-")
		}
		groups[base] = append(groups[base], c)
	}
	slug := map[*candidate]string{}
	for base, group := range groups {
		sort.Slice(group, func(i, j int) bool { return shorterFirst(group[i].entry.Path, group[j].entry.Path) })
		for i, c := range group {
			slug[c] = base
			if i > 0 {
				slug[c] = fmt.Sprintf("%s-%d", base, i+1)
			}
		}
	}

	s.notes = nil
	s.byID, s.byPath, s.bySlug = map[string]*Note{}, map[string]*Note{}, map[string]*Note{}
	s.aliases, s.docs = map[string][]string{}, map[string]*vault.Doc{}
	for _, c := range cands {
		n := &Note{
			ID:          ids[c],
			Slug:        slug[c],
			Title:       c.doc.Title(),
			Body:        c.doc.Body,
			Tags:        strings.Join(c.doc.Tags(), ","),
			Pinned:      c.doc.Pinned(),
			Path:        c.entry.Path,
			Rev:         c.rev,
			ArchivedAt:  c.doc.Archived(),
			UpdatedAt:   c.entry.ModTime,
			WorkspaceID: s.workspaceIDForName(c.doc.Workspace()),
		}
		if n.Title == "" {
			n.Title = stem(c.entry.Path)
		}
		n.CreatedAt = n.UpdatedAt
		if created := c.doc.Created(); created != nil {
			n.CreatedAt = *created
		}
		s.notes = append(s.notes, n)
		s.byID[n.ID] = n
		s.byPath[n.Path] = n
		s.bySlug[n.Slug] = n
		s.aliases[n.ID] = c.doc.Aliases()
		s.docs[n.ID] = c.doc
	}
}

func stem(rel string) string {
	base := path.Base(rel)
	return strings.TrimSuffix(base, path.Ext(base))
}

func shorterFirst(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// originalFirst orders the files claiming one id: the shortest path, then
// a name without iCloud's " N" conflict suffix, then alphabetically.
func originalFirst(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	if ca, cb := icloudCopyRe.MatchString(stem(a)), icloudCopyRe.MatchString(stem(b)); ca != cb {
		return !ca
	}
	return a < b
}

// writeLocked runs fn holding the lock for one vault file, so that kb
// processes never interleave a read-modify-write of the same file.
func (s *Store) writeLocked(rel string, fn func() error) error {
	abs, err := s.vault.Abs(rel)
	if err != nil {
		return err
	}
	unlock, err := lockFile(s.opts.LockDir, abs)
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}
