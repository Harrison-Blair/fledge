package fswatch

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestReadReportsFatalReadError drives the reader's fatal read branch: a
// directory descriptor polls ready for reading but the read(2) syscall rejects
// it with EISDIR, which is neither EINTR nor EAGAIN, so read() must surface a
// single wrapped error on Errors() and then fall silent.
func TestReadReportsFatalReadError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// A directory descriptor is always poll-readable, yet read(2) on it fails
	// with EISDIR — a deterministic stand-in for a descriptor going bad under
	// the poll without the recycling race a live close would introduce.
	dirfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var pipe [2]int
	if err := unix.Pipe2(pipe[:], unix.O_CLOEXEC); err != nil {
		unix.Close(dirfd)
		t.Fatal(err)
	}
	defer func() {
		for _, fd := range []int{dirfd, pipe[0], pipe[1]} {
			unix.Close(fd)
		}
	}()

	w := &unixWatcher{
		inotify: dirfd, stopRead: pipe[0], stopWrite: pipe[1],
		events: make(chan struct{}, 1), errs: make(chan error, 1), finished: make(chan struct{}),
	}
	go w.read()

	select {
	case err := <-w.Errors():
		if err == nil {
			t.Fatal("Errors() delivered a nil error")
		}
	case <-w.Events():
		t.Fatal("read() signalled an event instead of failing")
	case <-time.After(5 * time.Second):
		t.Fatal("read() reported no error for a failing descriptor")
	}

	// The reader returns after failing; confirm it has exited and stays silent.
	select {
	case <-w.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("read() did not exit after reporting its error")
	}
	select {
	case <-w.Events():
		t.Fatal("failed reader still signalled events")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestFailDropsWhenErrorsChannelFull covers fail()'s non-blocking default arm:
// a pending terminal error must not be overwritten and a second fail() must not
// block the reader.
func TestFailDropsWhenErrorsChannelFull(t *testing.T) {
	t.Parallel()

	w := &unixWatcher{errs: make(chan error, 1)}
	first := errors.New("first terminal error")
	w.errs <- first

	done := make(chan struct{})
	go func() {
		w.fail(errors.New("second terminal error"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fail() blocked on a full Errors() channel")
	}

	select {
	case got := <-w.errs:
		if got != first {
			t.Fatalf("Errors() = %v, want the first error preserved", got)
		}
	default:
		t.Fatal("Errors() channel unexpectedly empty")
	}
	select {
	case got := <-w.errs:
		t.Fatalf("fail() enqueued a second error %v instead of dropping it", got)
	default:
	}
}
