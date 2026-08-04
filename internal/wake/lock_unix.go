//go:build !windows

package wake

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

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
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock wake ledger %q: %w", path, err)
	}
	return func() error {
		unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
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
