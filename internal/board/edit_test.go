package board

import (
	"os"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Board {
	t.Helper()
	b, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func plugin(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/plugin/" + name + ".md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// changedLines lists the line numbers that differ between two renderings
// with the same number of lines.
func changedLines(t *testing.T, before, after string) []int {
	t.Helper()
	a, b := strings.Split(before, "\n"), strings.Split(after, "\n")
	if len(a) != len(b) {
		t.Fatalf("line count changed: %d -> %d\n%s", len(a), len(b), after)
	}
	var out []int
	for i := range a {
		if a[i] != b[i] {
			out = append(out, i)
		}
	}
	return out
}

// Editing a card changes that card's first line and nothing else in the
// file, for every card in every corpus board.
func TestEditsTouchOnlyTheEditedCard(t *testing.T) {
	for _, name := range corpus(t) {
		src := plugin(t, name)
		probe := mustParse(t, src)
		for li := range probe.Lanes {
			for ci := range probe.Lanes[li].Items() {
				b := mustParse(t, src)
				it := b.Lanes[li].Items()[ci]
				it.SetTitle(it.Title + " edited")
				changed := changedLines(t, src, string(b.Render()))
				if len(changed) != 1 {
					t.Errorf("%s lane %d card %d: lines %v changed", name, li, ci, changed)
				}
			}
		}
	}
}

func TestTitlesKeepMidSentenceTagsAndTrailingTokens(t *testing.T) {
	b := mustParse(t, plugin(t, "tags"))
	fix := b.Lanes[0].Items()[0] // "Fix #ui login on the mobile app"
	if fix.Title != "Fix #ui login on the mobile app" || strings.Join(fix.Tags, ",") != "ui" {
		t.Fatalf("title=%q tags=%v", fix.Title, fix.Tags)
	}
	fix.SetTitle("Fix #ui login on the web app")
	if got := lineOf(t, b, "Fix #ui"); got != "- [ ] Fix #ui login on the web app ^"+fix.ID {
		t.Errorf("line = %q", got)
	}

	draft := mustParse(t, plugin(t, "basic")).Lanes[0].Items()[1] // "Draft outline @{2026-10-05} #writing"
	if draft.Title != "Draft outline" {
		t.Errorf("title = %q", draft.Title)
	}
}

func lineOf(t *testing.T, b *Board, contains string) string {
	t.Helper()
	for _, l := range strings.Split(string(b.Render()), "\n") {
		if strings.Contains(l, contains) {
			return l
		}
	}
	t.Fatalf("no line contains %q", contains)
	return ""
}

func TestAHashTypedInATitleIsNotATag(t *testing.T) {
	b := New([]string{"Todo"})
	it, _ := b.Add("Todo", "Fix #3 bug", "")
	again := mustParse(t, string(b.Render()))
	got := again.Lanes[0].Items()[0]
	if got.Title != "Fix #3 bug" || len(got.Tags) != 0 {
		t.Errorf("title=%q tags=%v (line %q)", got.Title, got.Tags, lineOf(t, b, it.ID))
	}
}

func TestPriorityIsANestedTag(t *testing.T) {
	b := New([]string{"Todo"})
	it, _ := b.Add("Todo", "Ship", "")
	it.SetPriority("high")
	it.SetLabels([]string{"some label", "ui"})
	line := lineOf(t, b, "Ship")
	if line != "- [ ] Ship #some-label #ui #priority/high ^"+it.ID {
		t.Errorf("line = %q", line)
	}
	reread := mustParse(t, string(b.Render()))
	again := reread.Lanes[0].Items()[0]
	if again.Priority() != "high" || strings.Join(again.Labels(), ",") != "some-label,ui" {
		t.Errorf("priority=%q labels=%v", again.Priority(), again.Labels())
	}
	again.SetPriority("medium")
	if strings.Contains(lineOf(t, reread, "Ship"), "priority") {
		t.Error("medium should remove the priority tag")
	}
}

// Priorities written by other tools are read too.
func TestPriorityFromTasksAndFields(t *testing.T) {
	src := "---\nkanban-plugin: board\n---\n\n## Todo\n\n" +
		"- [ ] a 🔺\n- [ ] b ⏫\n- [ ] c 🔽\n- [ ] d [priority:: high]\n- [ ] e #priority/urgent\n- [ ] f\n"
	var got []string
	for _, it := range mustParse(t, src).Lanes[0].Items() {
		got = append(got, it.Priority())
	}
	if strings.Join(got, ",") != "urgent,high,low,high,urgent,medium" {
		t.Errorf("priorities = %v", got)
	}
}

func TestNewCardsFollowTheLanesBulletAndIndent(t *testing.T) {
	// The source board, as written by hand: the plugin itself rewrites *
	// bullets as - when it saves.
	src, _ := os.ReadFile("testdata/src/bullets-star.md")
	star := mustParse(t, string(src))
	it, _ := star.Add("Starred", "third", "")
	if got := lineOf(t, star, "third"); got != "* [ ] third ^"+it.ID {
		t.Errorf("star lane line = %q", got)
	}
	tabs := mustParse(t, plugin(t, "tabs.usetab"))
	it, _ = tabs.Add("Todo", "another", "line one\nline two")
	out := string(tabs.Render())
	if !strings.Contains(out, "- [ ] another ^"+it.ID+"\n\tline one\n\tline two\n") {
		t.Errorf("tab lane:\n%s", out)
	}
}

func TestTheSettingsBlockStaysLastWithABlankLineBeforeIt(t *testing.T) {
	src := plugin(t, "empty-lanes")
	b := mustParse(t, src)
	it, _ := b.Add("Empty last", "appended", "")
	if err := b.ArchiveItem(it.ID); err != nil {
		t.Fatal(err)
	}
	out := string(b.Render())
	i := strings.Index(out, "%% kanban:settings")
	if i < 2 || out[i-2:i] != "\n\n" || !strings.HasSuffix(out, "%%") {
		t.Errorf("settings block not last after a blank line:\n%s", out)
	}
	again := mustParse(t, out)
	if again.Archive == nil || len(again.Archive.Items()) != 1 {
		t.Errorf("archive not re-read:\n%s", out)
	}
}

func TestANewArchiveUsesTheBoardsLanguage(t *testing.T) {
	b := mustParse(t, "---\nkanban-plugin: board\n---\n\n## Offen\n\n- [ ] Karte ^k0000001\n\n\n## Erledigt\n\n**Fertiggestellt**\n\n\n")
	if err := b.ArchiveItem("k0000001"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b.Render()), "***\n\n## Archiv\n") {
		t.Errorf("archive heading:\n%s", b.Render())
	}
}

func TestLaneNamesEndingInANumberInParensAreRefused(t *testing.T) {
	for _, name := range []string{"Q3 (2024)", "Done (3)", "", "  "} {
		if err := ValidateLaneName(name); err == nil {
			t.Errorf("%q should be refused", name)
		}
	}
	if err := ValidateLaneName("Q3 2024"); err != nil {
		t.Error(err)
	}
}
