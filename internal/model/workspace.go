package model

import (
	"fmt"
	"strings"
	"time"
)

type WorkspaceKind string

const (
	KindProject  WorkspaceKind = "project"
	KindArea     WorkspaceKind = "area"
	KindResource WorkspaceKind = "resource"
	KindArchive  WorkspaceKind = "archive"
)

var WorkspaceKinds = []WorkspaceKind{KindProject, KindArea, KindResource, KindArchive}

func ParseWorkspaceKind(s string) (WorkspaceKind, error) {
	switch strings.ToLower(s) {
	case "project":
		return KindProject, nil
	case "area":
		return KindArea, nil
	case "resource":
		return KindResource, nil
	case "archive":
		return KindArchive, nil
	default:
		return "", fmt.Errorf("invalid workspace kind %q: must be project, area, resource, or archive", s)
	}
}

func (k WorkspaceKind) String() string {
	return string(k)
}

func (k WorkspaceKind) Label() string {
	switch k {
	case KindProject:
		return "[P]"
	case KindArea:
		return "[A]"
	case KindResource:
		return "[R]"
	case KindArchive:
		return "[Ar]"
	default:
		return "[?]"
	}
}

type Workspace struct {
	ID          string
	Name        string
	Kind        WorkspaceKind
	Description string
	Path        string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func ValidateWorkspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	if len(name) > 100 {
		return fmt.Errorf("workspace name cannot exceed 100 characters")
	}
	return nil
}
