package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// tempPrefix names in-progress writes. Readers ignore these files, so one left
// behind by a crashed writer is harmless.
const tempPrefix = ".tmp-"

// mkdir and syncDir are replaceable so tests can observe directory creation
// and syncing.
var (
	mkdir   = os.Mkdir
	syncDir = fsyncDir
)

// mkdirAll creates path and any missing ancestors with mode 0700, syncing the
// parent of each directory it creates so the new entry survives a crash. A
// directory created concurrently by another process still gets its parent
// synced.
func mkdirAll(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return &fs.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
		}
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent != path {
		if err := mkdirAll(parent); err != nil {
			return err
		}
	}
	if err := mkdir(path, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return syncDir(parent)
}

// createFile creates an empty file at path when it is missing and syncs its
// directory so the new entry survives a crash.
func createFile(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// lock takes an exclusive flock on path and returns its release function. Each
// call opens its own file description, so concurrent callers in one process
// serialize as well as separate processes do.
func lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("state: open lock: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("state: lock %s: %w", path, err)
	}
	return func() { f.Close() }, nil
}

// writeReplace atomically replaces path with data.
func writeReplace(path string, data []byte) error {
	return writeTemp(path, data, os.Rename)
}

// writeExclusive atomically creates path with data, failing with fs.ErrExist
// when path already exists. Linking a complete temp file means readers never
// observe a partially written record.
func writeExclusive(path string, data []byte) error {
	return writeTemp(path, data, os.Link)
}

// writeTemp writes data to a synced temp file beside path, publishes it with
// publish, and syncs the directory.
func writeTemp(path string, data []byte, publish func(oldpath, newpath string) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	if err := publish(tmp.Name(), path); err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	return syncDir(dir)
}

func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("state: sync %s: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("state: sync %s: %w", dir, err)
	}
	return nil
}
