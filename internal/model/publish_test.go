package model

import "testing"

func TestParseEngine(t *testing.T) {
	tests := []struct {
		input string
		want  Engine
		err   bool
	}{
		{"jekyll", EngineJekyll, false},
		{"Jekyll", EngineJekyll, false},
		{"JEKYLL", EngineJekyll, false},
		{"hugo", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := ParseEngine(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseEngine(%q) error = %v, want err=%v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseEngine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
