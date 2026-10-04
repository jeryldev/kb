package fstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/text/unicode/norm"
)

// lockKey is the name a path is locked under. Callers pass paths in a
// vault whose symlinks Open resolved; folding case and Unicode form makes
// "Notes" and "notes" (one folder on macOS's default disk) one lock. On a
// case-sensitive disk two such files then share a lock, which only makes
// one wait for the other.
func lockKey(abs string) string {
	return strings.ToLower(norm.NFC.String(abs))
}

// lockFile takes an exclusive advisory lock for the file at abs, waiting
// for another kb process that holds it. The lock file is named after a
// hash of the path's lockKey, in dir, which is outside the synced vault.
// Lock files are small and kept, one per file kb has written.
func lockFile(dir, abs string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}
	sum := sha256.Sum256([]byte(lockKey(abs)))
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
