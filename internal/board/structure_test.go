package board

import (
	"strings"
	"testing"
)

const front = "---\nkanban-plugin: board\n---\n\n"

// A # in a wikilink or a code span is not a tag, and a backslash there
// would be text: SetTitle leaves them alone.
func TestSetTitleLeavesLinksAndCodeAlone(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] old ^abcd1234\n")
	it := b.Lanes[0].Items()[0]
	it.SetTitle("see [[New#Intro]] and `#include` then #tag")
	line := lineOf(t, b, "abcd1234")
	if !strings.Contains(line, "[[New#Intro]]") || !strings.Contains(line, "`#include`") {
		t.Errorf("line = %q", line)
	}
	if !strings.Contains(line, `\#tag`) {
		t.Errorf("a new # word should still be escaped: %q", line)
	}
	again := mustParse(t, string(b.Render()))
	if got := again.Lanes[0].Items()[0].Title; got != "see [[New#Intro]] and `#include` then #tag" {
		t.Errorf("title reads back as %q", got)
	}
}

// Each # is judged by its own word, even when the same pair of characters
// comes twice.
func TestSetTitleJudgesEachHashByItsOwnWord(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] see #uk office ^abcd1234\n")
	it := b.Lanes[0].Items()[0]
	it.SetTitle("see #ui and #uk office")
	again := mustParse(t, string(b.Render())).Lanes[0].Items()[0]
	if got := strings.Join(again.Labels(), ","); got != "uk" {
		t.Errorf("labels = %q, want uk (line %q)", got, lineOf(t, b, "abcd1234"))
	}
}

// A #word in a code span is code, as the plugin reads it: not a label, and
// never deleted when the labels change.
func TestTagsInCodeSpansAreCode(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] run `x #foo y` now #real ^abcd1234\n")
	it := b.Lanes[0].Items()[0]
	if got := strings.Join(it.Labels(), ","); got != "real" {
		t.Errorf("labels = %q, want real", got)
	}
	it.SetLabels([]string{"other"})
	line := lineOf(t, b, "abcd1234")
	if !strings.Contains(line, "`x #foo y`") || strings.Contains(line, "#real") || !strings.Contains(line, "#other") {
		t.Errorf("line = %q", line)
	}
}

// Spaces other than ASCII ones (a no-break space, an ideographic space)
// start and end tags as they do for the plugin.
func TestUnicodeSpacesSeparateTags(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] 日本\u3000#タグ and #a\u00a0b ^abcd1234\n")
	if got := strings.Join(b.Lanes[0].Items()[0].Labels(), ","); got != "タグ,a" {
		t.Errorf("labels = %q, want タグ,a", got)
	}
	if got := NormalizeLabel("a\u00a0b"); got != "a-b" {
		t.Errorf("NormalizeLabel = %q", got)
	}
}

// A board saved with a byte-order mark keeps one frontmatter and its mark.
func TestABoardWithAByteOrderMark(t *testing.T) {
	src := "\ufeff" + front + "## Todo\n\n- [ ] a ^abcd1234\n"
	b := mustParse(t, src)
	if got := string(b.Render()); got != src {
		t.Errorf("a no-op round trip changed it:\n%q", got)
	}
	b.Add("Todo", "b", "")
	out := string(b.Render())
	if strings.Count(out, "kanban-plugin") != 1 || !strings.HasPrefix(out, "\ufeff---\n") {
		t.Errorf("after an edit:\n%q", out)
	}
}

// A lane after the Archive stays after it, and the archive stays an
// archive, through a no-op save and an edit.
func TestALaneAfterTheArchiveKeepsItsPlace(t *testing.T) {
	src := front + "## Todo\n\n- [ ] a ^abcd1234\n\n***\n\n## Archive\n\n- [x] old ^abcd5678\n\n## After\n\n- [ ] c ^abcd9999\n"
	b := mustParse(t, src)
	if got := string(b.Render()); got != src {
		t.Errorf("a no-op round trip changed it:\n%s", got)
	}
	b.Add("Todo", "new", "")
	again := mustParse(t, string(b.Render()))
	if again.Archive == nil || len(again.Archive.Items()) != 1 || again.Lane("Archive") != nil {
		t.Errorf("the archive is no longer one:\n%s", b.Render())
	}
}

// Deleting one of two cards that share a title and have no ^id does not
// hand its id to the other.
func TestTwinCardsKeepTheirIDs(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] x\n- [ ] x\n")
	first, second := b.Lanes[0].Items()[0].ID, b.Lanes[0].Items()[1].ID
	if err := b.Delete(first); err != nil {
		t.Fatal(err)
	}
	again := mustParse(t, string(b.Render()))
	if got := again.Lanes[0].Items()[0].ID; got != second {
		t.Errorf("the remaining card's id is %s, was %s (the deleted one had %s)", got, second, first)
	}
}

// A lane is removed only when nothing but blank lines and its Complete
// marker would go with it.
func TestRemovingALaneKeepsItsText(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\nlane notes here\n\n## Done\n\n**Complete**\n\n")
	if err := b.RemoveLane("Todo"); err == nil {
		t.Error("removing a lane with text in it: want an error")
	}
	if err := b.RemoveLane("Done"); err != nil {
		t.Errorf("a lane with only its Complete marker: %v", err)
	}
}

// A card with no words of its own (only a tag, or only its id) can still
// have its description changed.
func TestACardWithoutProseCanBeEdited(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] #onlytag ^t1\n")
	it := b.Lanes[0].Items()[0]
	if it.ID != "t1" || it.Title != "" {
		t.Fatalf("id %q title %q", it.ID, it.Title)
	}
	it.SetDescription("hello")
	again := mustParse(t, string(b.Render())).Lanes[0].Items()[0]
	if again.ID != "t1" || again.Description != "hello" || strings.Join(again.Labels(), ",") != "onlytag" {
		t.Errorf("card = %+v", again)
	}
}

// Lane names that a heading would read differently are refused.
func TestLaneNamesAHeadingWouldChangeAreRefused(t *testing.T) {
	for _, bad := range []string{"Done #", "#", "##"} {
		if err := ValidateLaneName(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
	b := New([]string{"Todo"})
	if err := b.AddLane("  Padded  "); err != nil {
		t.Fatal(err)
	}
	if again := mustParse(t, string(b.Render())); again.Lane("Padded") == nil || again.Lanes[1].Title != "Padded" {
		t.Errorf("lanes: %+v", again.Lanes)
	}
}

// A label spelled like a priority tag is a priority, not a label that
// piles up on every save.
func TestAPriorityShapedLabelDoesNotPileUp(t *testing.T) {
	b := New([]string{"Todo"})
	it, _ := b.Add("Todo", "card", "")
	it.SetLabels([]string{"priority/low", "ui"})
	it.SetLabels([]string{"priority/low", "ui"})
	line := lineOf(t, b, it.ID)
	if strings.Count(line, "priority/") > 1 || !strings.Contains(line, "#ui") {
		t.Errorf("line = %q", line)
	}
}

// "- [x]foo", with no space after the box, is the text "[x]foo" to the
// plugin, not a checked card.
func TestABoxWithoutASpaceIsText(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [x]foo ^abcd1234\n")
	it := b.Lanes[0].Items()[0]
	if it.Check != ' ' || it.Title != "[x]foo" {
		t.Errorf("check %q title %q", it.Check, it.Title)
	}
}

// Older plugin versions wrote a card's lines joined by <br>; they read as
// lines.
func TestBrInACardReadsAsLines(t *testing.T) {
	b := mustParse(t, front+"## Todo\n\n- [ ] first<br>second line<br>third ^abcd1234\n")
	it := b.Lanes[0].Items()[0]
	if it.Title != "first" || it.Description != "second line\nthird" {
		t.Errorf("title %q description %q", it.Title, it.Description)
	}
	it.SetTitle("first, edited")
	again := mustParse(t, string(b.Render())).Lanes[0].Items()[0]
	if again.Title != "first, edited" || again.Description != "second line\nthird" {
		t.Errorf("after an edit: title %q description %q\n%s", again.Title, again.Description, b.Render())
	}
}

// A setext heading ("Todo" over "====") is a lane, as the plugin reads it.
func TestSetextHeadingsAreLanes(t *testing.T) {
	b := mustParse(t, front+"Todo\n====\n\n- [ ] a ^abcd1234\n\n## Done\n\n- [ ] b ^abcd5678\n")
	if len(b.Lanes) != 2 || b.Lanes[0].Title != "Todo" || len(b.Lanes[0].Items()) != 1 {
		t.Fatalf("lanes %+v", b.Lanes)
	}
	if err := b.RenameLane("Todo", "Next"); err != nil {
		t.Fatal(err)
	}
	again := mustParse(t, string(b.Render()))
	if len(again.Lanes) != 2 || again.Lanes[0].Title != "Next" || len(again.Lanes[0].Items()) != 1 {
		t.Errorf("after a rename:\n%s", b.Render())
	}
}
