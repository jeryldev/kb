package model

import (
	"fmt"
	"strings"
	"time"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

var Priorities = []Priority{PriorityUrgent, PriorityHigh, PriorityMedium, PriorityLow}

func ParsePriority(s string) (Priority, error) {
	switch strings.ToLower(s) {
	case "low":
		return PriorityLow, nil
	case "medium":
		return PriorityMedium, nil
	case "high":
		return PriorityHigh, nil
	case "urgent":
		return PriorityUrgent, nil
	default:
		return "", fmt.Errorf("invalid priority %q: must be low, medium, high, or urgent", s)
	}
}

func (p Priority) String() string {
	return string(p)
}

func (p Priority) Next() Priority {
	for i, pri := range Priorities {
		if pri == p && i < len(Priorities)-1 {
			return Priorities[i+1]
		}
	}
	return Priorities[0]
}

func (p Priority) Prev() Priority {
	for i, pri := range Priorities {
		if pri == p && i > 0 {
			return Priorities[i-1]
		}
	}
	return Priorities[len(Priorities)-1]
}

type Card struct {
	ID          string
	ColumnID    string
	Title       string
	Description string
	Priority    Priority
	Position    int
	Labels      string
	ExternalID  string
	ArchivedAt  *time.Time
	DeletedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (c *Card) LabelList() []string {
	if c.Labels == "" {
		return nil
	}
	parts := strings.Split(c.Labels, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func (c *Card) HasLabel(label string) bool {
	for _, l := range c.LabelList() {
		if strings.EqualFold(l, label) {
			return true
		}
	}
	return false
}

func ValidateCardTitle(title string) error {
	if title == "" {
		return fmt.Errorf("card title cannot be empty")
	}
	if len(title) > 200 {
		return fmt.Errorf("card title cannot exceed 200 characters")
	}
	return nil
}
