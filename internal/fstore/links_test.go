package fstore

import (
	"sort"
	"strings"
	"testing"
)

func backlinkNames(t *testing.T, s *Store, slug string) string {
	t.Helper()
	n, err := s.GetNoteBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, b := range s.Backlinks(n.ID) {
		out = append(out, b.SourceType+":"+b.Title)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestLinksResolveTheWayObsidianReadsThem(t *testing.T) {
	s := testStore(t)
	put(t, s, "cpp.md", "---\ntitle: C++ Notes\n---\n")
	put(t, s, "3 Horizons of Growth.md", "")
	put(t, s, "dual.md", "---\naliases: [DT]\n---\n")
	put(t, s, "sub/Deep Note.md", "")
	put(t, s, "a.md", "[[C++ Notes]] [[3 horizons of growth#Part 2]] [[dt|display]] [[sub/Deep Note^blk]] [[Deep Note.md]]")
	reload(t, s)
	for slug, want := range map[string]string{
		"cpp":                  "note:a",
		"3-horizons-of-growth": "note:a",
		"dual":                 "note:a",
		"deep-note":            "note:a",
	} {
		if got := backlinkNames(t, s, slug); got != want {
			t.Errorf("backlinks of %s = %q", slug, got)
		}
	}
}

func TestLinksFollowTheVault(t *testing.T) {
	s := testStore(t)
	put(t, s, "early.md", "see [[Later Idea]]")
	reload(t, s)
	put(t, s, "Later Idea.md", "")
	reload(t, s)
	if got := backlinkNames(t, s, "later-idea"); got != "note:early" {
		t.Errorf("a link to a note created later: %q", got)
	}
}

func TestWhatIsNotALink(t *testing.T) {
	s := testStore(t)
	put(t, s, "Target.md", "")
	put(t, s, "Diagram.png.md", "")
	put(t, s, "src.md", "self [[#Heading]] and [[^blk]]\n`[[Target]]` in code\n```\n[[Target]]\n```\n![[Diagram.png]]\nreal ![[Target]]")
	put(t, s, "Self.md", "I link to [[Self]]")
	reload(t, s)
	if got := backlinkNames(t, s, "target"); got != "note:src" {
		t.Errorf("only the embed outside code links Target: %q", got)
	}
	if got := backlinkNames(t, s, "diagram-png"); got != "" {
		t.Errorf("an image embed is not a note link: %q", got)
	}
	if got := backlinkNames(t, s, "self"); got != "" {
		t.Errorf("a note does not backlink itself: %q", got)
	}
}

func TestRenameMovesTheFileAndRewritesLinksToIt(t *testing.T) {
	s := testStore(t)
	put(t, s, "ideas/Old Name.md", "---\ncssclass: wide\naliases: [ON]\n---\nthe note")
	put(t, s, "a.md", "see [[Old Name]] and [[old name#Part 2|that part]], [[ON]], [[card:Old Name]], not [[Old Names]]")
	put(t, s, "b.md", "also [[ideas/Old Name^blk]]")
	put(t, s, "c.md", "unrelated")
	reload(t, s)
	n, _ := s.GetNoteBySlug("old-name")
	cBefore := read(t, s, "c.md")

	renamed, changed, err := s.RenameNote(n.ID, "New Name")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != n.ID || renamed.Path != "ideas/New Name.md" || changed != 2 {
		t.Errorf("renamed = %+v, changed %d", renamed, changed)
	}
	if got := read(t, s, "a.md"); got != "see [[New Name]] and [[New Name#Part 2|that part]], [[ON]], [[card:Old Name]], not [[Old Names]]" {
		t.Errorf("a.md = %q", got)
	}
	if got := read(t, s, "b.md"); got != "also [[New Name^blk]]" {
		t.Errorf("b.md = %q", got)
	}
	if read(t, s, "c.md") != cBefore || !strings.Contains(read(t, s, "ideas/New Name.md"), "cssclass: wide") {
		t.Error("unrelated note or unknown keys disturbed")
	}

	put(t, s, "Plan.md", "---\ntitle: Taken Title\n---\n")
	reload(t, s)
	if _, _, err := s.RenameNote(n.ID, "taken title"); err == nil {
		t.Error("renaming to another note's title should fail")
	}
	put(t, s, "Draft.md", "body")
	reload(t, s)
	d, _ := s.GetNoteBySlug("draft")
	if r, _, err := s.RenameNote(d.ID, "draft"); err != nil || r.Title != "draft" {
		t.Errorf("a case-only rename: %+v, %v", r, err)
	}
}

// A card's source id is "<board path>#<card id>", and a board's file name
// can hold a #: the id splits at the last one.
func TestABoardNamedWithAHashKeepsItsLinks(t *testing.T) {
	s := testStore(t)
	put(t, s, "Target.md", "body")
	put(t, s, "Boards/C# work.md", "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] see [[Target]] ^abcd1234\n")
	put(t, s, "Boards/Plain.md", "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] see [[Target]] ^abcd5678\n")
	reload(t, s)
	n, err := s.ResolveNote("Target")
	if err != nil {
		t.Fatal(err)
	}
	var boards []string
	for _, b := range s.Backlinks(n.ID) {
		boards = append(boards, b.Board)
	}
	sort.Strings(boards)
	if strings.Join(boards, ",") != "C# work,Plain" {
		t.Fatalf("backlinks from boards %v, want C# work and Plain", boards)
	}
	if _, _, err := s.RenameNote(n.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"Boards/C# work.md", "Boards/Plain.md"} {
		if got := read(t, s, b); !strings.Contains(got, "[[Renamed]]") {
			t.Errorf("%s after the rename:\n%s", b, got)
		}
	}
}
