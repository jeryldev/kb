package model

import (
	"fmt"
	"strings"
	"time"
)

type Engine string

const (
	EngineJekyll Engine = "jekyll"
)

func ParseEngine(s string) (Engine, error) {
	switch strings.ToLower(s) {
	case "jekyll":
		return EngineJekyll, nil
	default:
		return "", fmt.Errorf("unsupported engine %q: must be jekyll", s)
	}
}

type PublishTarget struct {
	ID          string
	WorkspaceID *string
	Name        string
	Engine      Engine
	BasePath    string
	PostsDir    string
	CreatedAt   time.Time
}

type PublishLog struct {
	ID          string
	NoteID      string
	TargetID    string
	FilePath    string
	FrontMatter string
	PublishedAt time.Time
}
