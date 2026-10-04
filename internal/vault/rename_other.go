//go:build !darwin && !linux

package vault

func renameNoReplace(from, to string) error { return checkThenRename(from, to) }
