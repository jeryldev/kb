package model

import (
	"testing"
)

func TestValidateNoteTitle(t *testing.T) {
	if err := ValidateNoteTitle(""); err == nil {
		t.Error("expected error for empty title")
	}
	if err := ValidateNoteTitle("Valid title"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateNoteTitle(string(long)); err == nil {
		t.Error("expected error for title > 200 chars")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Great Idea!", "my-great-idea"},
		{"Hello   World", "hello-world"},
		{"  Leading Trailing  ", "leading-trailing"},
		{"UPPERCASE", "uppercase"},
		{"already-slugged", "already-slugged"},
		{"Special @#$ Characters", "special-characters"},
		{"Multiple---Hyphens", "multiple-hyphens"},
		{"123 Numbers", "123-numbers"},
	}
	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidateNoteSlug(t *testing.T) {
	if err := ValidateNoteSlug(""); err == nil {
		t.Error("expected error for empty slug")
	}
	if err := ValidateNoteSlug("valid-slug"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateNoteSlug("Invalid Slug"); err == nil {
		t.Error("expected error for slug with spaces")
	}
	if err := ValidateNoteSlug("UPPER"); err == nil {
		t.Error("expected error for slug with uppercase")
	}
}

func TestNoteTagList(t *testing.T) {
	n := &Note{Tags: "go,pkm,tools"}
	tags := n.TagList()
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
	if tags[0] != "go" || tags[1] != "pkm" || tags[2] != "tools" {
		t.Errorf("unexpected tags: %v", tags)
	}
}

func TestNoteTagListEmpty(t *testing.T) {
	n := &Note{Tags: ""}
	tags := n.TagList()
	if tags != nil {
		t.Errorf("expected nil for empty tags, got %v", tags)
	}
}

func TestNoteHasTag(t *testing.T) {
	n := &Note{Tags: "go,pkm"}
	if !n.HasTag("go") {
		t.Error("expected HasTag('go') to be true")
	}
	if !n.HasTag("GO") {
		t.Error("expected HasTag('GO') to be true (case-insensitive)")
	}
	if n.HasTag("rust") {
		t.Error("expected HasTag('rust') to be false")
	}
}
