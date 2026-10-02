package fstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockFile takes an exclusive advisory lock for the file at abs, waiting
// for another kb process that holds it. The lock file is named after a
// hash of the path, in dir, which is outside the synced vault.
func lockFile(dir, abs string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	f, err := os.OpenFile(filepath.Join(dir, hex.EncodeToString(sum[:12])+".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("locking %s: %w", abs, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
