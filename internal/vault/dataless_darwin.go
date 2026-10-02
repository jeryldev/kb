package vault

import (
	"io/fs"
	"syscall"
)

// sfDataless is macOS's SF_DATALESS file flag: the file's contents have
// been evicted to iCloud and reading it downloads them (or fails offline).
const sfDataless = 0x40000000

func isDataless(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Flags&sfDataless != 0
}
