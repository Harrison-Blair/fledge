//go:build windows

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func lockSessionRecord(_ context.Context, root string) (func() error, error) {
	path := recordPath(root)
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open Fledge session lock %q: %w", path, err)
	}
	var overlapped windows.Overlapped
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock Fledge session record %q: %w", path, err)
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
		closeErr := file.Close()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("unlock Fledge session record: %w", unlockErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close Fledge session lock: %w", closeErr)
		}
		return errors.Join(unlockErr, closeErr)
	}, nil
}
