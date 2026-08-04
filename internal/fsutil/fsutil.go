// Package fsutil holds the symlink-safe file primitives shared by every
// package that owns files beneath .fledge.
//
// The threat these guard against is a symlink planted at a path fledge is about
// to write: without O_NOFOLLOW the open follows it and fledge truncates or
// appends to a file the attacker chose. Unix refuses such an open outright;
// Windows has no equivalent flag, so OpenRegular brackets the open with an
// Lstat before and a same-file check after, and Windows callers must keep
// os.O_TRUNC out of their flags and truncate through the returned handle
// instead.
package fsutil

import (
	"errors"
	"fmt"
	"os"
)

// RejectSymlink reports an error when path names a symlink. A path that does
// not exist is not an error: callers use this before creating a file.
func RejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %q must not be a symlink", path)
	}
	return nil
}

// OpenRegular opens path without following symlinks and hands back the handle
// only once it is proven to be a regular file that path still names. Callers
// wrap the error with their own subject; the messages here name only the path.
func OpenRegular(path string, flags int, permission os.FileMode) (*os.File, error) {
	if err := RejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := OpenNoFollow(path, flags, permission)
	if err != nil {
		return nil, err
	}
	if err := validateOpened(file, path); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// validateOpened checks that file is a regular file and is still the file that
// path names, so a replacement that landed during the open is rejected.
func validateOpened(file *os.File, path string) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) {
		return fmt.Errorf("path %q changed while opening or is a symlink", path)
	}
	return nil
}

// SyncDirectory flushes a directory so a create, rename or unlink inside it
// survives a crash.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory %q: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory %q: %w", path, err)
	}
	return nil
}
