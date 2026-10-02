package vault

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkFindsMarkdownAndSkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.md", "a")
	write(t, dir, "sub/b.md", "b")
	write(t, dir, "sub/image.png", "x")
	write(t, dir, ".obsidian/workspace.md", "x")
	write(t, dir, ".trash/old.md", "x")
	write(t, dir, ".hidden.md", "x")

	v := New(dir)
	var got []string
	if err := v.Walk(func(e Entry) error {
		got = append(got, e.Path)
		if e.Size == 0 || e.ModTime.IsZero() {
			t.Errorf("%s: missing stat info", e.Path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "sub/b.md" {
		t.Errorf("walk = %v", got)
	}
}

func TestWalkOnMissingVaultCreatesIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	if err := New(dir).Walk(func(Entry) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("vault dir not created: %v", err)
	}
}

func TestWriteIsAtomicAndReadable(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)
	doc, _ := Parse([]byte("hello"))
	doc.SetTitle("Hi")
	entry, err := v.Write("deep/new.md", doc)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Path != "deep/new.md" || entry.Size == 0 {
		t.Errorf("entry = %+v", entry)
	}
	got, err := v.Read("deep/new.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title() != "Hi" || got.Body != "hello" {
		t.Errorf("read back title=%q body=%q", got.Title(), got.Body)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "deep", ".*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestPathsCannotEscapeTheVault(t *testing.T) {
	v := New(t.TempDir())
	doc, _ := Parse(nil)
	for _, rel := range []string{"../x.md", "/etc/x.md", "a/../../x.md"} {
		if _, err := v.Write(rel, doc); err == nil {
			t.Errorf("Write(%q) should fail", rel)
		}
		if _, err := v.Read(rel); err == nil {
			t.Errorf("Read(%q) should fail", rel)
		}
	}
}

func TestCreateRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "taken.md", "mine")
	v := New(dir)
	doc, _ := Parse([]byte("theirs"))
	if _, err := v.Create("taken.md", doc); err == nil {
		t.Fatal("expected an error")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "taken.md"))
	if string(data) != "mine" {
		t.Errorf("file was overwritten: %q", data)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "gone.md", "x")
	v := New(dir)
	if err := v.Remove("gone.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.md")); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if err := v.Remove("gone.md"); err != nil {
		t.Errorf("removing a missing file should be a no-op, got %v", err)
	}
}

func TestWriteKeepsModeAndFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "private.md", "x")
	os.Chmod(filepath.Join(dir, "private.md"), 0o600)
	write(t, dir, "real/target.md", "old")
	os.Symlink(filepath.Join(dir, "real", "target.md"), filepath.Join(dir, "link.md"))

	v := New(dir)
	doc, _ := Parse([]byte("new"))
	if _, err := v.Write("private.md", doc); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "private.md")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	if _, err := v.Write("link.md", doc); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(filepath.Join(dir, "link.md")); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a regular file")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "real", "target.md")); string(data) != "new" {
		t.Errorf("target = %q", data)
	}
}
