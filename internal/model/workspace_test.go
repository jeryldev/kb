package model

import (
	"testing"
)

func TestParseWorkspaceKind(t *testing.T) {
	tests := []struct {
		input string
		want  WorkspaceKind
		err   bool
	}{
		{"project", KindProject, false},
		{"area", KindArea, false},
		{"resource", KindResource, false},
		{"archive", KindArchive, false},
		{"PROJECT", KindProject, false},
		{"Area", KindArea, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := ParseWorkspaceKind(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("ParseWorkspaceKind(%q) error = %v, want err=%v", tt.input, err, tt.err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseWorkspaceKind(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWorkspaceKindString(t *testing.T) {
	if KindProject.String() != "project" {
		t.Errorf("KindProject.String() = %q", KindProject.String())
	}
}

func TestWorkspaceKindLabel(t *testing.T) {
	tests := []struct {
		kind WorkspaceKind
		want string
	}{
		{KindProject, "[P]"},
		{KindArea, "[A]"},
		{KindResource, "[R]"},
		{KindArchive, "[Ar]"},
	}
	for _, tt := range tests {
		if got := tt.kind.Label(); got != tt.want {
			t.Errorf("%s.Label() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	if err := ValidateWorkspaceName(""); err == nil {
		t.Error("expected error for empty name")
	}
	if err := ValidateWorkspaceName("Valid Name"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateWorkspaceName(string(long)); err == nil {
		t.Error("expected error for name > 100 chars")
	}
}
