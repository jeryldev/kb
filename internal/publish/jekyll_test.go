package publish

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jeryldev/kb/internal/model"
)

type mockResolver struct {
	notes map[string]*model.Note
}

func (m *mockResolver) GetNoteBySlug(slug string) (*model.Note, error) {
	if n, ok := m.notes[slug]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("not found")
}

func TestJekyllFileName(t *testing.T) {
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	got := JekyllFileName("my-great-post", date)
	want := "2026-02-24-my-great-post.md"
	if got != want {
		t.Errorf("JekyllFileName() = %q, want %q", got, want)
	}
}

func TestJekyllPermalink(t *testing.T) {
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	got := JekyllPermalink("my-post", date)
	want := "/blog/2026/02/24/my-post/"
	if got != want {
		t.Errorf("JekyllPermalink() = %q, want %q", got, want)
	}
}

func TestGenerateFrontMatter(t *testing.T) {
	note := &model.Note{
		Title: "My Great Post",
		Tags:  "go,pkm",
		Body:  "This is the first paragraph of my post.\n\nMore content here.",
	}
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)

	got := GenerateFrontMatter(note, date, false)

	if !strings.Contains(got, `title: "My Great Post"`) {
		t.Errorf("missing title in front matter: %s", got)
	}
	if !strings.Contains(got, "date: 2026-02-24") {
		t.Errorf("missing date in front matter: %s", got)
	}
	if !strings.Contains(got, "tags: [go, pkm]") {
		t.Errorf("missing tags in front matter: %s", got)
	}
	if !strings.Contains(got, `excerpt: "This is the first paragraph of my post."`) {
		t.Errorf("missing excerpt in front matter: %s", got)
	}
	if strings.Contains(got, "published: false") {
		t.Errorf("should not have published: false for non-draft: %s", got)
	}
}

func TestGenerateFrontMatterDraft(t *testing.T) {
	note := &model.Note{Title: "Draft Post", Body: "Content"}
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)

	got := GenerateFrontMatter(note, date, true)

	if !strings.Contains(got, "published: false") {
		t.Errorf("expected published: false for draft: %s", got)
	}
}

func TestGenerateFrontMatterNoTags(t *testing.T) {
	note := &model.Note{Title: "No Tags", Body: "Content"}
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)

	got := GenerateFrontMatter(note, date, false)

	if strings.Contains(got, "tags:") {
		t.Errorf("should not have tags line when note has no tags: %s", got)
	}
}

func TestResolveWikilinksPublished(t *testing.T) {
	resolver := &mockResolver{
		notes: map[string]*model.Note{
			"target-note": {Title: "Target Note"},
		},
	}
	published := map[string]string{
		"target-note": "/blog/2026/02/24/target-note/",
	}

	body := "Check out [[target-note]] for details."
	got := ResolveWikilinks(body, published, resolver)
	want := "Check out [Target Note](/blog/2026/02/24/target-note/) for details."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksUnpublished(t *testing.T) {
	resolver := &mockResolver{
		notes: map[string]*model.Note{
			"private-note": {Title: "Private Note"},
		},
	}
	published := map[string]string{}

	body := "See [[private-note]] for internal info."
	got := ResolveWikilinks(body, published, resolver)
	want := "See Private Note for internal info."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksWithDisplayText(t *testing.T) {
	published := map[string]string{
		"target": "/blog/2026/02/24/target/",
	}

	body := "Check [[target|my link text]] here."
	got := ResolveWikilinks(body, published, nil)
	want := "Check [my link text](/blog/2026/02/24/target/) here."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksCardPrefix(t *testing.T) {
	body := "Related to [[card:abc123]]."
	got := ResolveWikilinks(body, map[string]string{}, nil)
	want := "Related to abc123."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksBoardPrefix(t *testing.T) {
	body := "See [[board:my-board]]."
	got := ResolveWikilinks(body, map[string]string{}, nil)
	want := "See my-board."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksCardWithDisplayText(t *testing.T) {
	body := "See [[card:abc123|the task]]."
	got := ResolveWikilinks(body, map[string]string{}, nil)
	want := "See the task."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksMultiple(t *testing.T) {
	resolver := &mockResolver{
		notes: map[string]*model.Note{
			"note-a": {Title: "Note A"},
			"note-b": {Title: "Note B"},
		},
	}
	published := map[string]string{
		"note-a": "/blog/2026/02/24/note-a/",
	}

	body := "First [[note-a]], then [[note-b]]."
	got := ResolveWikilinks(body, published, resolver)
	want := "First [Note A](/blog/2026/02/24/note-a/), then Note B."

	if got != want {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, want)
	}
}

func TestResolveWikilinksNoLinks(t *testing.T) {
	body := "No wikilinks here, just text."
	got := ResolveWikilinks(body, map[string]string{}, nil)

	if got != body {
		t.Errorf("ResolveWikilinks() = %q, want %q", got, body)
	}
}

func TestGeneratePost(t *testing.T) {
	note := &model.Note{
		Title: "Test Post",
		Body:  "Hello [[world-note]]",
		Tags:  "test",
	}
	resolver := &mockResolver{
		notes: map[string]*model.Note{
			"world-note": {Title: "World Note"},
		},
	}
	published := map[string]string{
		"world-note": "/blog/2026/02/24/world-note/",
	}
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)

	got := GeneratePost(note, date, false, published, resolver)

	if !strings.HasPrefix(got, "---\n") {
		t.Error("expected front matter at start")
	}
	if !strings.Contains(got, "[World Note](/blog/2026/02/24/world-note/)") {
		t.Errorf("expected resolved wikilink in body: %s", got)
	}
}

func TestPostFilePath(t *testing.T) {
	date := time.Date(2026, 2, 24, 0, 0, 0, 0, time.UTC)
	got := PostFilePath("_posts", "my-post", date)
	want := "_posts/2026-02-24-my-post.md"

	if got != want {
		t.Errorf("PostFilePath() = %q, want %q", got, want)
	}
}

func TestExtractExcerptSkipsHeaders(t *testing.T) {
	body := "# Heading\n\nThis is the actual first paragraph."
	got := extractExcerpt(body)
	want := "This is the actual first paragraph."
	if got != want {
		t.Errorf("extractExcerpt() = %q, want %q", got, want)
	}
}

func TestExtractExcerptEmpty(t *testing.T) {
	got := extractExcerpt("")
	if got != "" {
		t.Errorf("extractExcerpt('') = %q, want ''", got)
	}
}

func TestExtractExcerptTruncates(t *testing.T) {
	long := strings.Repeat("a", 250)
	got := extractExcerpt(long)
	if len(got) != 203 { // 200 + "..."
		t.Errorf("extractExcerpt() len = %d, want 203", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected ... suffix, got %q", got[len(got)-5:])
	}
}
