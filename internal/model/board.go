package model

import "fmt"

type Board struct {
	ID          string
	Name        string
	Description string
	WorkspaceID string
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
