package fstore

import (
	"crypto/sha256"
	"encoding/binary"
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

// lockStripes is how many lock files there are. Paths share them by hash,
// so the lock folder never grows; two files sharing one only makes a
// writer of one wait for a writer of the other.
const lockStripes = 256

// lockName is the lock file that guards abs.
func lockName(abs string) string {
	sum := sha256.Sum256([]byte(lockKey(abs)))
	return fmt.Sprintf("kb-%03d.lock", binary.BigEndian.Uint32(sum[:4])%lockStripes)
}

// lockPath takes the exclusive lock that guards the file at abs, waiting
// for another kb process that holds it. A lock this store already holds
// (a path sharing its lock file, locked inside another) is held again
// rather than waited on, so one write can lock two paths. A Store is used
// by one goroutine, so its count needs no mutex.
//
// Locks taken inside another are taken in an order every kb follows, so
// two kb processes never each hold the lock the other waits for: a rename
// locks its two names in lockName order, and a trash name (trashLock) is
// only ever the innermost lock.
func (s *Store) lockPath(abs string) (func(), error) {
	return s.lockNamed(lockName(abs))
}

// trashLock guards a name in the trash. Trash names have lock files of
// their own: taken inside a note's lock, they must never be one a note
// uses.
func (s *Store) trashLock(abs string) (func(), error) {
	return s.lockNamed("trash-" + lockName(abs))
}

func (s *Store) lockNamed(name string) (func(), error) {
	if s.held == nil {
		s.held = map[string]*heldLock{}
	}
	if h := s.held[name]; h != nil {
		h.count++
		return s.unlocker(name), nil
	}
	unlock, err := lockFile(s.opts.LockDir, name)
	if err != nil {
		return nil, fmt.Errorf("locking %s: %w", name, err)
	}
	s.held[name] = &heldLock{count: 1, unlock: unlock}
	return s.unlocker(name), nil
}

type heldLock struct {
	count  int
	unlock func()
}

func (s *Store) unlocker(name string) func() {
	return func() {
		h := s.held[name]
		if h.count--; h.count == 0 {
			h.unlock()
			delete(s.held, name)
		}
	}
}

// lockFile takes an exclusive advisory lock on the lock file name in dir,
// which is outside the synced vault.
func lockFile(dir, name string) (func(), error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// kb 0.4.2 and before kept one lock file per path. kb leaves those: a
// lock file can only be removed safely when no kb has it open, which kb
// cannot tell (flock does not change a file's time). They are empty, and
// the folder can be emptied whenever no kb is running.
