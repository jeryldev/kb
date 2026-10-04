package vault

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames a file, failing with EEXIST if to exists, in one
// step (renamex_np with RENAME_EXCL), or in two on a filesystem without it.
func renameNoReplace(from, to string) error {
	err := unix.RenamexNp(from, to, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return checkThenRename(from, to)
	}
	return err
}
