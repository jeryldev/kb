package model

import (
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

func TestExtractMarkdownLinks(t *testing.T) {
	input := "Visit [example](https://example.com) and [docs](https://docs.go.dev)"
	got := ExtractMarkdownLinks(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 links, got %d", len(got))
	}
	if got[0].TargetType != "url" || got[0].TargetRef != "https://example.com" {
		t.Errorf("link[0] = %+v, want url/https://example.com", got[0])
	}
}
