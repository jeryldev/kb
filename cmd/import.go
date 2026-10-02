package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// legacyDBPath is where kb 0.3 kept its SQLite database.
func legacyDBPath() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "kb", "kb.db"), nil
}

// checkImport stops kb while a 0.3 database waits to be imported, so that
// nothing is written to the vault that the import would then collide with.
func checkImport() error {
	path, err := legacyDBPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("found kb 0.3's database at %s; move its boards into the vault with: kb import (see what it would do with: kb import --dry-run)", path)
}
