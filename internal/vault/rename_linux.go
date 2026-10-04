package vault

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames a file, failing with EEXIST if to exists, in one
// step (renameat2 with RENAME_NOREPLACE), or in two on a filesystem or
// kernel without it.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) {
		return checkThenRename(from, to)
	}
	return err
}
