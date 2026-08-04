//go:build windows

package wake

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

func (l *Ledger) acquireLock(path string) (func() error, error) {
	file, err := fsutil.OpenRegular(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open wake lock %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure wake lock %q: %w", path, err)
	}
	var overlapped windows.Overlapped
	handle := windows.Handle(file.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock wake ledger %q: %w", path, err)
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
		closeErr := file.Close()
		if unlockErr != nil {
			unlockErr = fmt.Errorf("unlock wake ledger: %w", unlockErr)
		}
		if closeErr != nil {
			closeErr = fmt.Errorf("close wake lock: %w", closeErr)
		}
		return errors.Join(unlockErr, closeErr)
	}, nil
}
