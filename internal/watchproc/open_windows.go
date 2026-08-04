//go:build windows

package watchproc

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var errAlreadyRunning = errors.New("watcher is already running")

type singletonLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func openOwned(path string, flags int, permission os.FileMode) (*os.File, error) {
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, flags, permission)
	if err != nil {
		return nil, err
	}
	return validateOwned(file, path)
}

func acquire(path string) (*singletonLock, error) {
	file, err := openOwned(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open watch lock %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	lock := &singletonLock{file: file}
	handle := windows.Handle(file.Fd())
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, &lock.overlapped); err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errAlreadyRunning
		}
		return nil, fmt.Errorf("lock watcher singleton %q: %w", path, err)
	}
	return lock, nil
}

func (l *singletonLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	handle := windows.Handle(file.Fd())
	unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &l.overlapped)
	return errors.Join(unlockErr, file.Close())
}
