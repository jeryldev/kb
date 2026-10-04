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
		"[phase two](/blog/2026/05/13/dual-transformation/#phase-2), Private Thoughts, Nowhere, abc"
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

// A tag YAML would read as something other than text (a boolean, null, a
// number, a date) is quoted, so the site gets the tag as written.
func TestTagsYAMLWouldReadAsOtherValuesAreQuoted(t *testing.T) {
	tags := []string{"true", "null", "2024", "yes", "no", "on", "1.5", "~", "2026-01-02", "0x1F", "go"}
	note := &model.Note{Title: "t", Tags: strings.Join(tags, ",")}
	fm := GenerateFrontMatter(note, time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC), false)
	var parsed struct {
		Tags []any `yaml:"tags"`
	}
	if err := yaml.Unmarshal([]byte(strings.Trim(fm, "-\n")), &parsed); err != nil {
		t.Fatalf("%v\n%s", err, fm)
	}
	if len(parsed.Tags) != len(tags) {
		t.Fatalf("tags = %v\n%s", parsed.Tags, fm)
	}
	for i, want := range tags {
		if got, ok := parsed.Tags[i].(string); !ok || got != want {
			t.Errorf("tag %q reads back as %#v\n%s", want, parsed.Tags[i], fm)
		}
	}
	if !strings.Contains(fm, ", go]") {
		t.Errorf("plain tags stay plain:\n%s", fm)
	}
}

// kb fills in :year, :month, :day and :title; a pattern with anything
// else (a Jekyll style name, :categories) would give links that go nowhere.
func TestPermalinkPatternsKbCannotFillAreRefused(t *testing.T) {
	for _, ok := range []string{"", "/blog/:year/:month/:day/:title/", "/:title.html", "/notes/:year-:month-:day-:title/"} {
		if err := ValidatePermalink(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"pretty", "date", "/:categories/:title/", "/:year/:i_month/:title/", "blog/:title"} {
		if err := ValidatePermalink(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

// The excerpt is the first line of prose: never a line of code, whatever
// fence the code block uses.
func TestTheExcerptSkipsCodeBlocks(t *testing.T) {
	for body, want := range map[string]string{
		"```go\nfmt.Println()\n```\nFirst prose line":           "First prose line",
		"~~~\n```\nstill code\n~~~\nAfter the block":            "After the block",
		"# Title\n\n````\n```\ncode\n```\n````\n\nThe real one": "The real one",
	} {
		if got := extractExcerpt(body); got != want {
			t.Errorf("%q: excerpt %q, want %q", body, got, want)
		}
	}
}

// A link to a heading with no display text reads as Obsidian shows it.
func TestALinkToAHeadingReadsAsObsidianShowsIt(t *testing.T) {
	dt := &model.Note{ID: "id-dt", Title: "Dual Transformation"}
	got := ResolveWikilinks("[[Nowhere#Part 2]] and [[DT#Phase 1]]", nil, nameResolver{"dt": dt})
	if want := "Nowhere > Part 2 and Dual Transformation > Phase 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A link to a heading in the same note reads as the heading; a link to a
// published note's heading goes to the heading's anchor on the post.
func TestHeadingLinksReadAndPointRight(t *testing.T) {
	dt := &model.Note{ID: "id-dt", Title: "Dual Transformation"}
	permalinks := map[string]string{"id-dt": "/blog/dt/"}
	got := ResolveWikilinks("[[#Petty Cash]] and [[DT#Phase 2: Go!]]", permalinks, nameResolver{"dt": dt})
	if want := "Petty Cash and [Dual Transformation > Phase 2: Go!](/blog/dt/#phase-2-go)"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// An embedded note reads like a link to it; an embedded file, by its name.
func TestEmbedsReadAsTheirNames(t *testing.T) {
	other := &model.Note{ID: "id-o", Title: "Other Note"}
	got := ResolveWikilinks("A ![[Other Note]] and ![[photo.png|300]] here", nil, nameResolver{"other note": other})
	if want := "A Other Note and photo.png here"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Text Liquid would run ({{ }}, {% %}) is kept as text, on Jekyll 3 and 4.
func TestLiquidInANoteIsKeptAsText(t *testing.T) {
	note := &model.Note{Title: "Templates", Body: "Use {{ .Name }} and {% if x %} here."}
	post := GeneratePost(note, time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC), false, nil, nameResolver{})
	if !strings.Contains(post, "{% raw %}Use {{ .Name }} and {% if x %} here.{% endraw %}") {
		t.Errorf("post:\n%s", post)
	}
	plain := GeneratePost(&model.Note{Title: "Plain", Body: "no liquid"}, time.Now(), false, nil, nameResolver{})
	if strings.Contains(plain, "raw") {
		t.Errorf("a note with no Liquid should not be wrapped:\n%s", plain)
	}
}

// The excerpt is prose: not a comment, a table or indented code.
func TestTheExcerptSkipsWhatIsNotProse(t *testing.T) {
	for body, want := range map[string]string{
		"%% a comment %%\nProse":                   "Prose",
		"%%\nmulti\nline\n%%\nProse":               "Prose",
		"<!-- hidden -->\nProse":                   "Prose",
		"<!--\nhidden\n-->\nProse":                 "Prose",
		"| a | b |\n|---|---|\n| 1 | 2 |\n\nProse": "Prose",
		"    indented code\n\nProse":               "Prose",
	} {
		if got := extractExcerpt(body); got != want {
			t.Errorf("%q: excerpt %q, want %q", body, got, want)
		}
	}
}
