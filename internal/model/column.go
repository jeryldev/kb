package model

import "fmt"

type Column struct {
	ID       string
	BoardID  string
	Name     string
	Position int
	WIPLimit *int
}

func ValidateColumnName(name string) error {
	if name == "" {
		return fmt.Errorf("column name cannot be empty")
	}
	if len(name) > 50 {
		return fmt.Errorf("column name cannot exceed 50 characters")
	}
	return nil
}
