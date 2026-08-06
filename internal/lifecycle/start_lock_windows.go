//go:build windows

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockSessionRecord serializes Start/Stop on a dedicated, never-renamed
// session.lock file. Locking session.json directly would decouple the lock from
// the record the moment rewriteRecord renames a fresh inode over it. The lock
// file is created lazily on first acquisition and is never removed by Fledge.
func lockSessionRecord(_ context.Context, root string) (func() error, error) {
	path := sessionLockPath(root)
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open Fledge session startup lock %q: %w", path, err)
	}
	var overlapped windows.Overlapped
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Fledge session startup lock %q: %w", path, err)
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
		closeErr := file.Close()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("unlock Fledge session startup lock: %w", unlockErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close Fledge session startup lock: %w", closeErr)
		}
		return errors.Join(unlockErr, closeErr)
	}, nil
}
