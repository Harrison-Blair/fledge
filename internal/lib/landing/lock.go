package landing

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// LockPath is the land lock relative to the primary checkout.
var LockPath = filepath.Join(".fledge", "state", "land.lock")

// Lock takes the exclusive land lock of the repository whose primary checkout
// is root, waiting for any other holder, and returns its release function.
// The state directory must already exist.
func Lock(root string) (func(), error) {
	path := filepath.Join(root, LockPath)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open land lock: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("take land lock %s: %w", path, err)
	}
	return func() { f.Close() }, nil
}
