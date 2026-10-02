//go:build !darwin

package vault

import "io/fs"

func isDataless(fs.FileInfo) bool { return false }
