package model

import (
	"strings"
	"testing"
)

func TestValidateBoardName(t *testing.T) {
	if err := ValidateBoardName("my-project"); err != nil {
		t.Errorf("valid name returned error: %v", err)
	}

	if err := ValidateBoardName(""); err == nil {
		t.Error("empty name should return error")
	}

	long := strings.Repeat("a", 101)
	if err := ValidateBoardName(long); err == nil {
		t.Error("101-char name should return error")
	}

	exactly100 := strings.Repeat("a", 100)
	if err := ValidateBoardName(exactly100); err != nil {
		t.Errorf("100-char name returned error: %v", err)
	}
}
