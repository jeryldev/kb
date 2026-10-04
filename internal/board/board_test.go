package board

import (
	"strings"
	"testing"
)

// A board as the Obsidian Kanban plugin writes it (see its
// src/parsers/formats/list.ts boardToMd): frontmatter, "## lane" headings
// with an optional "(N)" item limit, task items, a **Complete** marker, an
// archive after "***", and the plugin's settings block.
const pluginBoard = `---

kanban-plugin: board
workspace: School

---

## Backlog

- [ ] Add auth ^c1efacf3
- [ ] Draft outline @{2026-10-05} #writing



## In Progress (2)

- [ ] Fix login bug #ui #bug [priority:: high] [ext:: linear:DEV-42] ^889962bb
    Steps are in [[Login Notes]].
    Second line of the description.
- [/] Half done thing ^aa00aa00



## Done

**Complete**
- [x] Shipped it ^5b20d812



***

## Archive

- [ ] Old card ^3c8e4fa8

%% kanban:settings
` + "```" + `
{"kanban-plugin":"board","lane-width":272}
` + "```" + `
%%`

func TestParseReadsLanesItemsAndArchive(t *testing.T) {
	b, err := Parse([]byte(pluginBoard))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Front.Workspace(); got != "School" {
		t.Errorf("workspace = %q", got)
	}
	var names []string
	for _, l := range b.Lanes {
		names = append(names, l.Title)
	}
	if strings.Join(names, "|") != "Backlog|In Progress|Done" {
		t.Fatalf("lanes = %v", names)
	}
	if b.Lanes[1].MaxItems != 2 || b.Lanes[0].MaxItems != 0 {
		t.Errorf("max items = %d, %d", b.Lanes[1].MaxItems, b.Lanes[0].MaxItems)
	}
	if !b.Lanes[2].Complete {
		t.Error("Done should carry the **Complete** marker")
	}

	fix := b.Lanes[1].Items()[0]
	if fix.ID != "889962bb" || fix.Title != "Fix login bug" {
		t.Errorf("item = %q %q", fix.ID, fix.Title)
	}
	if strings.Join(fix.Tags, ",") != "ui,bug" || fix.Field("priority") != "high" || fix.Field("ext") != "linear:DEV-42" {
		t.Errorf("tags=%v priority=%q ext=%q", fix.Tags, fix.Field("priority"), fix.Field("ext"))
	}
	if fix.Description != "Steps are in [[Login Notes]].\nSecond line of the description." {
		t.Errorf("description = %q", fix.Description)
	}
	if draft := b.Lanes[0].Items()[1]; draft.Title != "Draft outline" || draft.Tags[0] != "writing" {
		t.Errorf("draft = %+v", draft)
	}
	if half := b.Lanes[1].Items()[1]; half.Check != '/' {
		t.Errorf("check = %q", half.Check)
	}
	if b.Archive == nil || len(b.Archive.Items()) != 1 || b.Archive.Items()[0].ID != "3c8e4fa8" {
		t.Errorf("archive = %+v", b.Archive)
	}
}

func TestAnUnchangedBoardRendersByteForByte(t *testing.T) {
	b, err := Parse([]byte(pluginBoard))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b.Render()); got != pluginBoard {
		t.Errorf("round trip changed the file:\n--- got ---\n%s\n--- want ---\n%s", got, pluginBoard)
	}
}

func TestEditingACardKeepsWhatKbDoesNotOwn(t *testing.T) {
	b, _ := Parse([]byte(pluginBoard))
	draft := b.Lanes[0].Items()[1]
	draft.SetTitle("Draft the outline")
	draft.SetField("priority", "urgent")
	out := string(b.Render())
	// The plugin's date and the tag survive, an id is assigned, and nothing
	// else in the file changes.
	want := "- [ ] Draft the outline @{2026-10-05} #writing [priority:: urgent] ^" + draft.ID
	if !strings.Contains(out, want+"\n") {
		t.Errorf("edited line missing %q in:\n%s", want, out)
	}
	if len(draft.ID) != 8 {
		t.Errorf("assigned id = %q", draft.ID)
	}
	if strings.Replace(out, want, "- [ ] Draft outline @{2026-10-05} #writing", 1) != pluginBoard {
		t.Error("more than the edited line changed")
	}
}

func TestMovingArchivingAndDeletingCards(t *testing.T) {
	b, _ := Parse([]byte(pluginBoard))
	if err := b.Move("c1efacf3", "Done", 0); err != nil {
		t.Fatal(err)
	}
	if err := b.ArchiveItem("889962bb"); err != nil {
		t.Fatal(err)
	}
	if err := b.Delete("aa00aa00"); err != nil {
		t.Fatal(err)
	}
	again, err := Parse(b.Render())
	if err != nil {
		t.Fatal(err)
	}
	ids := func(l *Lane) (out []string) {
		for _, it := range l.Items() {
			out = append(out, it.ID)
		}
		return
	}
	if got := strings.Join(ids(again.Lanes[2]), ","); got != "c1efacf3,5b20d812" {
		t.Errorf("Done = %s", got)
	}
	// Moved into a **Complete** lane, the card is checked, as the plugin does.
	if again.Lanes[2].Items()[0].Check != 'x' {
		t.Error("card moved into Done should be checked")
	}
	if got := strings.Join(ids(again.Lanes[1]), ","); got != "" {
		t.Errorf("In Progress = %s", got)
	}
	if got := strings.Join(ids(again.Archive), ","); got != "3c8e4fa8,889962bb" {
		t.Errorf("Archive = %s", got)
	}
	if !strings.Contains(string(b.Render()), "Steps are in [[Login Notes]].") {
		t.Error("the archived card lost its description")
	}
}

func TestNewBoardAndCardsAreValidPluginMarkdown(t *testing.T) {
	b := New([]string{"Todo", "Doing", "Done"})
	b.Lanes[1].MaxItems = 3
	it, err := b.Add("Todo", "Write tests", "first line\nsecond line")
	if err != nil {
		t.Fatal(err)
	}
	it.SetLabels([]string{"dev"})
	out := string(b.Render())
	for _, want := range []string{
		"---\nkanban-plugin: board\n---\n",
		"## Todo\n\n- [ ] Write tests #dev ^" + it.ID + "\n    first line\n    second line\n",
		"## Doing (3)\n",
		"%% kanban:settings\n```\n{\"kanban-plugin\":\"board\"}\n```\n%%",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	again, err := Parse([]byte(out))
	if err != nil || len(again.Lanes) != 3 || again.Lanes[0].Items()[0].Description != "first line\nsecond line" {
		t.Errorf("re-parse: %v %+v", err, again)
	}
}

func TestCardsWithoutIDsGetStableDerivedIDs(t *testing.T) {
	src := "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] Plain card from the plugin\n- [ ] Plain card from the plugin\n"
	a, _ := Parse([]byte(src))
	b, _ := Parse([]byte(src))
	first, second := a.Lanes[0].Items()[0].ID, a.Lanes[0].Items()[1].ID
	if first == "" || first == second {
		t.Fatalf("ids = %q %q", first, second)
	}
	if b.Lanes[0].Items()[0].ID != first {
		t.Error("derived ids differ between reads")
	}
	// Reading never writes them; the file only gains ids when kb edits the board.
	if string(a.Render()) != src {
		t.Error("an untouched board gained ids")
	}
	a.Lanes[0].Items()[0].SetTitle("Edited")
	if !strings.Contains(string(a.Render()), "- [ ] Edited ^"+first) {
		t.Errorf("edit should pin the derived id:\n%s", a.Render())
	}
}

func TestDuplicateBlockIDsAreMadeUnique(t *testing.T) {
	src := "---\nkanban-plugin: board\n---\n\n## Todo\n\n- [ ] A ^dup00001\n- [ ] B ^dup00001\n"
	b, _ := Parse([]byte(src))
	items := b.Lanes[0].Items()
	if items[0].ID == items[1].ID {
		t.Errorf("both cards have id %q", items[0].ID)
	}
}

func TestIsBoard(t *testing.T) {
	if !IsBoard([]byte(pluginBoard)) {
		t.Error("plugin board not recognised")
	}
	if IsBoard([]byte("---\ntitle: note\n---\n## Not a board\n- [ ] task")) {
		t.Error("a note with tasks is not a board")
	}
}

// A board whose line endings are mixed (LF first, CRLF later, as some sync
// tools leave them) keeps its frontmatter and reads its lanes.
func TestMixedLineEndingsKeepTheFrontmatter(t *testing.T) {
	src := "---\nkanban-plugin: board\r\n---\r\n\r\n## Todo\r\n\r\n- [ ] one ^abcd1234\r\n"
	b, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(b.frontLines) != 3 || len(b.Lanes) != 1 || b.Lanes[0].Title != "Todo" || len(b.Lanes[0].Items()) != 1 {
		t.Fatalf("front %q, lanes %+v", b.frontLines, b.Lanes)
	}
	if got := string(b.Render()); got != src {
		t.Errorf("a no-op round trip changed it:\n%q", got)
	}
}
