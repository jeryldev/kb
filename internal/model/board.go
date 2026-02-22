package model

import (
	"fmt"
	"time"
)

type Board struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func ValidateBoardName(name string) error {
	if name == "" {
		return fmt.Errorf("board name cannot be empty")
	}
	if len(name) > 100 {
		return fmt.Errorf("board name cannot exceed 100 characters")
	}
	return nil
}

var DefaultColumns = []string{"Backlog", "Todo", "In Progress", "Review", "Done"}
