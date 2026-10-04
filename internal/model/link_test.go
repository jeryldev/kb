package model

import (
	"strings"
	"testing"
)

func TestParseWikilinks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []ParsedLink
	}{
		{
			name:  "simple note link",
			input: "Check out [[my-note]] for details",
			want: []ParsedLink{
				{TargetType: "note", TargetRef: "my-note", Display: "my-note"},
			},
		},
		{
			name:  "note link with display text",
			input: "See [[my-note|My Note Title]] here",
			want: []ParsedLink{
				{TargetType: "note", TargetRef: "my-note", Display: "My Note Title"},
			},
		},
		{
			name:  "card link",
			input: "Related to [[card:abc12345]]",
			want: []ParsedLink{
				{TargetType: "card", TargetRef: "abc12345", Display: "abc12345"},
			},
		},
		{
			name:  "board link",
			input: "On [[board:my-project]] board",
			want: []ParsedLink{
				{TargetType: "board", TargetRef: "my-project", Display: "my-project"},
			},
		},
		{
			name:  "multiple links",
			input: "Link [[note-a]] and [[note-b]] together",
			want: []ParsedLink{
				{TargetType: "note", TargetRef: "note-a", Display: "note-a"},
				{TargetType: "note", TargetRef: "note-b", Display: "note-b"},
			},
		},
		{
			name:  "no links",
			input: "Plain text with no links",
			want:  nil,
		},
		{
			name:  "card link with display",
			input: "See [[card:abc12345|Login Bug]]",
			want: []ParsedLink{
				{TargetType: "card", TargetRef: "abc12345", Display: "Login Bug"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseWikilinks(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseWikilinks() returned %d links, want %d", len(got), len(tt.want))
			}
			for i, link := range got {
				if link.TargetType != tt.want[i].TargetType {
					t.Errorf("link[%d].TargetType = %q, want %q", i, link.TargetType, tt.want[i].TargetType)
				}
				if link.TargetRef != tt.want[i].TargetRef {
					t.Errorf("link[%d].TargetRef = %q, want %q", i, link.TargetRef, tt.want[i].TargetRef)
				}
				if link.Display != tt.want[i].Display {
					t.Errorf("link[%d].Display = %q, want %q", i, link.Display, tt.want[i].Display)
				}
			}
		})
	}
}

func TestParseWikilinksDoNotSpanLines(t *testing.T) {
	text := "a stray [[ opener\nmore prose\nand a real [[Target]] link"
	links := ParseWikilinks(text)
	if len(links) != 1 || links[0].TargetRef != "Target" {
		t.Fatalf("links = %+v", links)
	}
	if links[0].Context != "and a real [[Target]] link" {
		t.Errorf("context = %q", links[0].Context)
	}
}

func TestWikilinksInCodeAndImageEmbedsAreNotLinks(t *testing.T) {
	text := "real [[A]]\n`[[B]]` inline\n```\n[[C]]\n```\n~~~\n[[D]]\n~~~\n![[img.png]] ![[E]] ![[doc.pdf]]"
	var got []string
	for _, l := range ParseWikilinks(text) {
		got = append(got, l.TargetRef)
	}
	if strings.Join(got, ",") != "A,E" {
		t.Errorf("links = %v", got)
	}
	out, n := RewriteWikilinks(text, func(target string) (string, bool) { return "X", true })
	if n != 2 || !strings.Contains(out, "`[[B]]`") || !strings.Contains(out, "\n[[C]]\n") || !strings.Contains(out, "![[img.png]]") {
		t.Errorf("rewrite touched code or images (%d):\n%s", n, out)
	}
}

// Fences close as CommonMark closes them: with the same character, at
// least as many of it, and nothing after but spaces. Any other fence-like
// line inside the block is code, so links after the block still count.
func TestFencesCloseOnlyOnAMatchingFence(t *testing.T) {
	cases := map[string][]string{
		"```\n~~~\n```\n\nSee [[Real]]\n":                         {"Real"},
		"~~~\n```\n[[InCode]]\n~~~\n[[After]]\n":                  {"After"},
		"````\n```\n[[InCode]]\n````\n[[After]]\n":                {"After"},
		"```go\n[[InCode]]\n``` not a close\n```\n[[After]]":      {"After"},
		"```\n[[InCode]]\n    ```\n[[StillCode]]\n```\n[[After]]": {"After"},
		"``` info `with` backticks\n[[NotCode]]\n":                {"NotCode"},
		"text ``a ` [[InSpan]] b`` [[Out]]\n":                     {"Out"},
		"```\n[[Unclosed]]\n":                                     nil,
	}
	for text, want := range cases {
		var got []string
		for _, l := range ParseWikilinks(text) {
			got = append(got, l.TargetRef)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%q: links %v, want %v", text, got, want)
		}
	}
}

// In a table, a wikilink's pipe is written \| so the table keeps its
// columns; a backslash before [[ makes it text.
func TestTablePipesAndEscapedLinks(t *testing.T) {
	links := ParseWikilinks(`| [[Note\|alias]] | \[[not a link]] | [[Real]] |`)
	var got []string
	for _, l := range links {
		got = append(got, l.TargetRef+"="+l.Display)
	}
	if strings.Join(got, ",") != "Note=alias,Real=Real" {
		t.Errorf("links = %v", got)
	}
}
