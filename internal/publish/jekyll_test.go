package publish

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jeryldev/kb/internal/model"
	"gopkg.in/yaml.v3"
)

type mockResolver struct {
	notes map[string]*model.Note
}

// ResolveNoteRef finds notes by their key, which also stands in for the
// note's id when the test leaves it empty.
func (m *mockResolver) ResolveNoteRef(ref string) (*model.Note, error) {
	if n, ok := m.notes[ref]; ok {
		if n.ID == "" {
			n.ID = ref
		}
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

	resolver := &mockResolver{notes: map[string]*model.Note{"target": {Title: "Target"}}}
	body := "Check [[target|my link text]] here."
	got := ResolveWikilinks(body, published, resolver)
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

type nameResolver map[string]*model.Note

// ResolveNoteRef ignores #heading and ^block, as the store's resolver does.
func (m nameResolver) ResolveNoteRef(ref string) (*model.Note, error) {
	if i := strings.IndexAny(ref, "#^"); i >= 0 {
		ref = ref[:i]
	}
	if n, ok := m[strings.ToLower(ref)]; ok {
		return n, nil
	}
	return nil, fmt.Errorf("not found")
}

func TestResolveWikilinksByAnyNameAndHeading(t *testing.T) {
	dt := &model.Note{ID: "id-dt", Title: "Dual Transformation"}
	other := &model.Note{ID: "id-other", Title: "Private Thoughts"}
	resolver := nameResolver{"dual transformation": dt, "dt": dt, "private thoughts": other}
	permalinks := map[string]string{"id-dt": "/blog/2026/05/13/dual-transformation/"}

	body := "[[Dual Transformation]], [[DT#Phase 2|phase two]], [[Private Thoughts]], [[Nowhere]], [[card:abc]]"
	got := ResolveWikilinks(body, permalinks, resolver)
	want := "[Dual Transformation](/blog/2026/05/13/dual-transformation/), " +
		"[phase two](/blog/2026/05/13/dual-transformation/), Private Thoughts, Nowhere, abc"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestResolveWikilinksStaysOnOneLine(t *testing.T) {
	body := "a stray [[ opener\nand [[Real]] link"
	got := ResolveWikilinks(body, nil, nameResolver{})
	if got != "a stray [[ opener\nand Real link" {
		t.Errorf("got %q", got)
	}
}

func TestPermalinkFromPostPath(t *testing.T) {
	got, ok := PermalinkFromPostPath("_posts/2026-05-13-dual-transformation.md")
	if !ok || got != "/blog/2026/05/13/dual-transformation/" {
		t.Errorf("got %q, %v", got, ok)
	}
	if _, ok := PermalinkFromPostPath("_posts/not-a-post.md"); ok {
		t.Error("a path without a date should not give a permalink")
	}
}

// Front matter must be YAML a site can read, whatever a title or tag holds.
func TestFrontMatterIsValidYAMLForAwkwardTitlesAndTags(t *testing.T) {
	note := &model.Note{Title: `Say "hi" \ to: you`, Tags: "go,*star,!bang,c++,a: b", Body: strings.Repeat("é", 300)}
	fm := GenerateFrontMatter(note, time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC), false)
	var parsed struct {
		Title   string   `yaml:"title"`
		Tags    []string `yaml:"tags"`
		Excerpt string   `yaml:"excerpt"`
	}
	body := strings.TrimSuffix(strings.TrimPrefix(fm, "---\n"), "---\n")
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("front matter is not YAML: %v\n%s", err, fm)
	}
	if parsed.Title != note.Title {
		t.Errorf("title = %q", parsed.Title)
	}
	if strings.Join(parsed.Tags, "|") != "go|*star|!bang|c++|a: b" {
		t.Errorf("tags = %q", parsed.Tags)
	}
	if !utf8.ValidString(parsed.Excerpt) || !strings.HasSuffix(parsed.Excerpt, "...") || utf8.RuneCountInString(parsed.Excerpt) != 203 {
		t.Errorf("excerpt = %q", parsed.Excerpt)
	}
	if !strings.Contains(fm, "tags: [go, ") {
		t.Errorf("plain tags stay plain:\n%s", fm)
	}
}

func TestPermalinkPatterns(t *testing.T) {
	path := "_posts/2026-05-13-dual-transformation.md"
	for pattern, want := range map[string]string{
		"":                                 "/blog/2026/05/13/dual-transformation/",
		"/:year/:month/:day/:title/":       "/2026/05/13/dual-transformation/",
		"/posts/:title":                    "/posts/dual-transformation",
		"/:year/:title.html":               "/2026/dual-transformation.html",
		"/notes/:year-:month-:day-:title/": "/notes/2026-05-13-dual-transformation/",
	} {
		got, ok := PermalinkFor(pattern, path)
		if !ok || got != want {
			t.Errorf("pattern %q: %q, want %q", pattern, got, want)
		}
	}
}
