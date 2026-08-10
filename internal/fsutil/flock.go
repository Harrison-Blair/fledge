package fsutil

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ReleaseFlock drops the advisory lock held on file and closes it, returning
// both failures joined so neither is lost. subject names the locked resource in
// the wrapped errors, e.g. "messaging lock" or "Fledge session startup lock".
func ReleaseFlock(file *os.File, subject string) error {
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		unlockErr = fmt.Errorf("unlock %s: %w", subject, unlockErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close %s: %w", subject, closeErr)
	}
	return errors.Join(unlockErr, closeErr)
}
