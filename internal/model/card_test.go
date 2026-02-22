package model

import (
	"strings"
	"testing"
)

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input string
		want  Priority
	}{
		{"low", PriorityLow},
		{"medium", PriorityMedium},
		{"high", PriorityHigh},
		{"urgent", PriorityUrgent},
		{"LOW", PriorityLow},
		{"Medium", PriorityMedium},
		{"HIGH", PriorityHigh},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePriority(tt.input)
			if err != nil {
				t.Fatalf("ParsePriority(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParsePriority(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePriorityInvalid(t *testing.T) {
	invalid := []string{"", "critical", "none", "123"}
	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			_, err := ParsePriority(input)
			if err == nil {
				t.Errorf("ParsePriority(%q) should return error", input)
			}
		})
	}
}

func TestPriorityNext(t *testing.T) {
	tests := []struct {
		from Priority
		want Priority
	}{
		{PriorityUrgent, PriorityHigh},
		{PriorityHigh, PriorityMedium},
		{PriorityMedium, PriorityLow},
		{PriorityLow, PriorityUrgent}, // wraps around
	}

	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			got := tt.from.Next()
			if got != tt.want {
				t.Errorf("%q.Next() = %q, want %q", tt.from, got, tt.want)
			}
		})
	}
}

func TestPriorityPrev(t *testing.T) {
	tests := []struct {
		from Priority
		want Priority
	}{
		{PriorityLow, PriorityMedium},
		{PriorityMedium, PriorityHigh},
		{PriorityHigh, PriorityUrgent},
		{PriorityUrgent, PriorityLow}, // wraps around
	}

	for _, tt := range tests {
		t.Run(string(tt.from), func(t *testing.T) {
			got := tt.from.Prev()
			if got != tt.want {
				t.Errorf("%q.Prev() = %q, want %q", tt.from, got, tt.want)
			}
		})
	}
}

func TestPriorityString(t *testing.T) {
	if PriorityHigh.String() != "high" {
		t.Errorf("PriorityHigh.String() = %q, want %q", PriorityHigh.String(), "high")
	}
}

func TestValidateCardTitle(t *testing.T) {
	if err := ValidateCardTitle("Fix bug"); err != nil {
		t.Errorf("ValidateCardTitle with valid title returned error: %v", err)
	}

	if err := ValidateCardTitle(""); err == nil {
		t.Error("ValidateCardTitle with empty title should return error")
	}

	long := strings.Repeat("a", 201)
	if err := ValidateCardTitle(long); err == nil {
		t.Error("ValidateCardTitle with 201-char title should return error")
	}

	exactly200 := strings.Repeat("a", 200)
	if err := ValidateCardTitle(exactly200); err != nil {
		t.Errorf("ValidateCardTitle with 200-char title returned error: %v", err)
	}
}

func TestCardLabelList(t *testing.T) {
	tests := []struct {
		name   string
		labels string
		want   int
	}{
		{"empty", "", 0},
		{"single", "bug", 1},
		{"multiple", "bug, feature, ui", 3},
		{"with spaces", " bug , feature ", 2},
		{"trailing comma", "bug,", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := &Card{Labels: tt.labels}
			got := card.LabelList()
			if len(got) != tt.want {
				t.Errorf("LabelList() returned %d labels, want %d: %v", len(got), tt.want, got)
			}
		})
	}
}

func TestCardHasLabel(t *testing.T) {
	card := &Card{Labels: "bug, Feature, UI"}

	if !card.HasLabel("bug") {
		t.Error("HasLabel should find 'bug'")
	}
	if !card.HasLabel("BUG") {
		t.Error("HasLabel should be case-insensitive")
	}
	if !card.HasLabel("feature") {
		t.Error("HasLabel should find 'feature' (case-insensitive)")
	}
	if card.HasLabel("backend") {
		t.Error("HasLabel should not find 'backend'")
	}
}
