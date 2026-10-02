package fstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishTargetsAreMachineLocal(t *testing.T) {
	s := testStore(t)
	site := t.TempDir()
	tgt, err := s.CreatePublishTarget("blog", "jekyll", site, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Name != "blog" || tgt.BasePath != site || tgt.PostsDir != "_posts" {
		t.Errorf("target = %+v", tgt)
	}
	if _, err := os.Stat(filepath.Join(s.Vault().Root(), ".kb", "publish.yml")); err == nil {
		t.Error("targets must not be written into the synced vault")
	}
	if _, err := s.CreatePublishTarget("blog", "jekyll", site, "", ""); err == nil {
		t.Error("a duplicate target name should be refused")
	}
	if _, err := s.CreatePublishTarget("rel", "jekyll", "./site", "", ""); err == nil {
		t.Error("a relative site path should be refused")
	}
	home, _ := os.UserHomeDir()
	if got, err := s.CreatePublishTarget("home", "jekyll", "~/kb-test-site", "", ""); err != nil || got.BasePath != filepath.Join(home, "kb-test-site") {
		t.Errorf("~ not expanded: %+v %v", got, err)
	}
	if again := sibling(t, s).ListPublishTargets(); len(again) != 2 {
		t.Errorf("targets after reopening = %d", len(again))
	}
	if err := s.DeletePublishTarget("home"); err != nil || len(s.ListPublishTargets()) != 1 {
		t.Errorf("delete: %v", err)
	}
}

func TestPublishHistoryLivesInTheNote(t *testing.T) {
	s := testStore(t)
	put(t, s, "Post.md", "plain note, no frontmatter")
	reload(t, s)
	n, _ := s.GetNoteBySlug("post")
	if err := s.RecordPublish(n.ID, "blog", "_posts/2026-05-13-post.md", false); err != nil {
		t.Fatal(err)
	}
	content := read(t, s, "Post.md")
	if !strings.Contains(content, "id: "+n.ID) || !strings.Contains(content, "blog: _posts/2026-05-13-post.md") {
		t.Errorf("file:\n%s", content)
	}
	// Renamed in Obsidian, the note keeps its id, and so its history.
	os.Rename(filepath.Join(s.Vault().Root(), "Post.md"), filepath.Join(s.Vault().Root(), "Renamed.md"))
	reload(t, s)
	post, ok := s.LatestPost(n.ID, "blog")
	if !ok || post.Path != "_posts/2026-05-13-post.md" || post.Draft {
		t.Errorf("post after an outside rename = %+v, %v", post, ok)
	}

	s.RecordPublish(n.ID, "drafts", "_posts/2026-05-13-post.md", true)
	if p, _ := s.LatestPost(n.ID, "drafts"); !p.Draft {
		t.Error("a draft publish should be recorded as a draft")
	}
	posts := s.PublishedPosts("blog")
	if posts[n.ID].Path != "_posts/2026-05-13-post.md" || len(posts) != 1 {
		t.Errorf("posts = %+v", posts)
	}
	if list := s.Publications("blog"); len(list) != 1 || list[0].Note.ID != n.ID {
		t.Errorf("publications = %+v", list)
	}
}
