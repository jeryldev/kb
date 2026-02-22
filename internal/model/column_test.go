package model

import (
	"strings"
	"testing"
)

func TestValidateColumnName(t *testing.T) {
	if err := ValidateColumnName("Todo"); err != nil {
		t.Errorf("valid name returned error: %v", err)
	}

	if err := ValidateColumnName(""); err == nil {
		t.Error("empty name should return error")
	}

	long := strings.Repeat("a", 51)
	if err := ValidateColumnName(long); err == nil {
		t.Error("51-char name should return error")
	}

	exactly50 := strings.Repeat("a", 50)
	if err := ValidateColumnName(exactly50); err != nil {
		t.Errorf("50-char name returned error: %v", err)
	}
}
