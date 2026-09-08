//go:build linux

package lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type projectLock struct {
	fd   int
	once sync.Once
	err  error
}

// Acquire takes the exclusive project lock on fledgeDir, waiting until the
// holder releases it or ctx ends. The returned release is idempotent.
func Acquire(ctx context.Context, fledgeDir string) (func() error, error) {
	fd, err := unix.Open(fledgeDir, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open project lock directory: %w", err)
	}

	delay := 10 * time.Millisecond
	for {
		err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			lock := &projectLock{fd: fd}
			return lock.release, nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			closeErr := unix.Close(fd)
			return nil, errors.Join(fmt.Errorf("lock project directory: %w", err), closeErr)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			closeErr := unix.Close(fd)
			return nil, errors.Join(ctx.Err(), closeErr)
		case <-timer.C:
		}
		if delay < 100*time.Millisecond {
			delay *= 2
			if delay > 100*time.Millisecond {
				delay = 100 * time.Millisecond
			}
		}
	}
}

func (l *projectLock) release() error {
	l.once.Do(func() {
		l.err = errors.Join(unix.Flock(l.fd, unix.LOCK_UN), unix.Close(l.fd))
	})
	return l.err
}
