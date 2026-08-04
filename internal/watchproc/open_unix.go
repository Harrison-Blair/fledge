//go:build !windows

package watchproc

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var errAlreadyRunning = errors.New("watcher is already running")

type singletonLock struct{ file *os.File }

func openOwned(path string, flags int, permission os.FileMode) (*os.File, error) {
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	descriptor, err := unix.Open(path, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(permission.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
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
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errAlreadyRunning
		}
		return nil, fmt.Errorf("lock watcher singleton %q: %w", path, err)
	}
	return &singletonLock{file: file}, nil
}

func (l *singletonLock) release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	return errors.Join(unlockErr, file.Close())
}
