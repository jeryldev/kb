package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Vault is a folder of notes. Paths given to and returned by a Vault are
// relative to its root and slash-separated.
type Vault struct {
	root string
}

// ErrExists is returned by Create when the file is already there.
var ErrExists = errors.New("file already exists in the vault")

type Entry struct {
	Path    string
	ModTime time.Time
	Size    int64
	// Dataless files are present but evicted to iCloud: reading one blocks
	// while it downloads, so callers should not read it unasked.
	Dataless bool
}

func New(root string) *Vault {
	return &Vault{root: root}
}

func (v *Vault) Root() string {
	return v.root
}

// Abs returns the file's path on disk, refusing paths outside the vault.
func (v *Vault) Abs(rel string) (string, error) {
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("note path %q is outside the vault", rel)
	}
	return filepath.Join(v.root, local), nil
}

// Walk calls fn for every Markdown file, skipping dot-files and
// dot-directories (.obsidian, .git, .trash). A missing vault is created.
// Any error stops the walk.
func (v *Vault) Walk(fn func(Entry) error) error {
	return v.WalkSkipping(fn, nil)
}

// WalkSkipping is Walk, except that a file or folder that cannot be read is
// reported to skipped and passed over instead of stopping the walk. An
// unreadable vault root is still an error.
func (v *Vault) WalkSkipping(fn func(Entry) error, skipped func(rel string, err error)) error {
	if err := os.MkdirAll(v.root, 0o755); err != nil {
		return fmt.Errorf("creating vault: %w", err)
	}
	skip := func(path string, d fs.DirEntry, err error) error {
		if skipped == nil {
			return err
		}
		rel, _ := filepath.Rel(v.root, path)
		skipped(filepath.ToSlash(rel), err)
		if d != nil && d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	return filepath.WalkDir(v.root, func(path string, d fs.DirEntry, err error) error {
		if path == v.root {
			return err
		}
		if err != nil {
			return skip(path, d, err)
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		info, err := d.Info()
		if err == nil && d.Type()&fs.ModeSymlink != 0 {
			// A symlinked note changes when its target does.
			info, err = os.Stat(path)
		}
		if err != nil {
			return skip(path, d, err)
		}
		rel, err := filepath.Rel(v.root, path)
		if err != nil {
			return err
		}
		return fn(Entry{Path: filepath.ToSlash(rel), ModTime: info.ModTime(), Size: info.Size(), Dataless: IsDataless(info)})
	})
}

func (v *Vault) Stat(rel string) (Entry, error) {
	path, err := v.Abs(rel)
	if err != nil {
		return Entry{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Path: rel, ModTime: info.ModTime(), Size: info.Size()}, nil
}

func (v *Vault) Read(rel string) (*Doc, error) {
	path, err := v.Abs(rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return doc, nil
}

// Write replaces the file atomically: a reader (or a sync client) sees the
// old content or the new, never a half-written file. A symlinked note is
// written through to its target, and an existing file keeps its mode.
func (v *Vault) Write(rel string, doc *Doc) (Entry, error) {
	path, err := v.Abs(rel)
	if err != nil {
		return Entry{}, err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 {
			if path, err = filepath.EvalSymlinks(path); err != nil {
				return Entry{}, fmt.Errorf("writing %s: %w", rel, err)
			}
			if info, err = os.Stat(path); err != nil {
				return Entry{}, fmt.Errorf("writing %s: %w", rel, err)
			}
		}
		mode = info.Mode().Perm()
	}
	tmp, err := v.writeTemp(path, doc, mode)
	if err != nil {
		return Entry{}, fmt.Errorf("writing %s: %w", rel, err)
	}
	defer os.Remove(tmp)
	if err := os.Rename(tmp, path); err != nil {
		return Entry{}, fmt.Errorf("writing %s: %w", rel, err)
	}
	return v.Stat(rel)
}

// Create writes a new file and fails with ErrExists if one is already at
// rel. The file appears complete or not at all: it is written aside and
// hard-linked into place, which (unlike rename) refuses to replace a file.
func (v *Vault) Create(rel string, doc *Doc) (Entry, error) {
	path, err := v.Abs(rel)
	if err != nil {
		return Entry{}, err
	}
	tmp, err := v.writeTemp(path, doc, 0o644)
	if err != nil {
		return Entry{}, fmt.Errorf("creating %s: %w", rel, err)
	}
	defer os.Remove(tmp)
	err = link(tmp, path)
	if errors.Is(err, fs.ErrExist) {
		return Entry{}, fmt.Errorf("%s: %w", rel, ErrExists)
	}
	if err != nil {
		// Some filesystems (exFAT, many network shares) have no hard
		// links. Creating the file exclusively still never replaces one,
		// though a crash part way can leave it short.
		if err := createExclusive(path, doc); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return Entry{}, fmt.Errorf("%s: %w", rel, ErrExists)
			}
			return Entry{}, fmt.Errorf("creating %s: %w", rel, err)
		}
	}
	return v.Stat(rel)
}

// link is os.Link, replaced in tests to act like a filesystem without
// hard links.
var link = os.Link

func createExclusive(path string, doc *Doc) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(doc.Render()); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

// writeTemp writes doc to a hidden temporary file beside path, so a rename
// or link into place stays on one filesystem.
func (v *Vault) writeTemp(path string, doc *Doc, mode fs.FileMode) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".kb-*.tmp")
	if err != nil {
		return "", err
	}
	_, err = tmp.Write(doc.Render())
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// Remove deletes the file; a file that is already gone is not an error.
func (v *Vault) Remove(rel string) error {
	path, err := v.Abs(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", rel, err)
	}
	return nil
}

// IsDataless reports whether a file's contents are in iCloud and not on
// this machine. Tests replace it to act out such files.
var IsDataless = isDataless

// Rename moves the file at oldRel to newRel, itself: a symlink stays a
// symlink, and the mode, Finder tags and creation time stay with it. It
// never replaces another file (ErrExists), but a new name that is the same
// file on this disk (another letter case on a case-insensitive disk,
// another Unicode form on APFS) renames it to that spelling.
func (v *Vault) Rename(oldRel, newRel string) error {
	from, err := v.Abs(oldRel)
	if err != nil {
		return err
	}
	to, err := v.Abs(newRel)
	if err != nil {
		return err
	}
	if fromInfo, err := os.Lstat(from); err == nil {
		if toInfo, err := os.Lstat(to); err == nil && os.SameFile(fromInfo, toInfo) {
			return os.Rename(from, to)
		}
	}
	err = renameNoReplace(from, to)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s: %w", newRel, ErrExists)
	}
	if err != nil {
		return fmt.Errorf("renaming %s to %s: %w", oldRel, newRel, err)
	}
	return nil
}

// checkThenRename renames a file unless to exists; the check and the
// rename are two steps, for systems with no call that does both.
func checkThenRename(from, to string) error {
	if _, err := os.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: fs.ErrExist}
	}
	return os.Rename(from, to)
}
