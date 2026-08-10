package fswatch

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

const inotifyMask = unix.IN_MODIFY | unix.IN_CLOSE_WRITE | unix.IN_CREATE |
	unix.IN_MOVED_TO | unix.IN_MOVED_FROM | unix.IN_DELETE |
	unix.IN_DELETE_SELF | unix.IN_MOVE_SELF

type unixWatcher struct {
	inotify   int
	stopRead  int
	stopWrite int
	events    chan struct{}
	errs      chan error
	finished  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// open watches dir, reporting every change to one of its entries. The reader
// goroutine blocks in poll over both the inotify descriptor and a stop pipe,
// never over a bare read: closing a descriptor out from under a blocked read
// neither reliably wakes the reader nor prevents the number being recycled to
// an unrelated file.
func open(dir string) (Watcher, error) {
	inotify, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("create inotify watch for %q: %w", dir, err)
	}
	if _, err := unix.InotifyAddWatch(inotify, dir, inotifyMask); err != nil {
		unix.Close(inotify)
		return nil, fmt.Errorf("watch directory %q: %w", dir, err)
	}
	var pipe [2]int
	if err := unix.Pipe2(pipe[:], unix.O_CLOEXEC); err != nil {
		unix.Close(inotify)
		return nil, fmt.Errorf("create watch stop pipe for %q: %w", dir, err)
	}
	w := &unixWatcher{
		inotify: inotify, stopRead: pipe[0], stopWrite: pipe[1],
		events: make(chan struct{}, 1), errs: make(chan error, 1), finished: make(chan struct{}),
	}
	go w.read()
	return w, nil
}

func (w *unixWatcher) Events() <-chan struct{} { return w.events }
func (w *unixWatcher) Errors() <-chan error    { return w.errs }

func (w *unixWatcher) Close() error {
	w.closeOnce.Do(func() {
		// A byte on the stop pipe is what the reader is polling for; only once it
		// has returned is closing the inotify descriptor free of a recycling race.
		if _, err := unix.Write(w.stopWrite, []byte{0}); err != nil {
			w.closeErr = fmt.Errorf("stop filesystem watch: %w", err)
		}
		<-w.finished
		for _, fd := range []int{w.inotify, w.stopRead, w.stopWrite} {
			if err := unix.Close(fd); err != nil && w.closeErr == nil {
				w.closeErr = fmt.Errorf("close filesystem watch: %w", err)
			}
		}
	})
	return w.closeErr
}

func (w *unixWatcher) read() {
	defer close(w.finished)
	buffer := make([]byte, 64*1024)
	for {
		fds := []unix.PollFd{
			{Fd: int32(w.inotify), Events: unix.POLLIN},
			{Fd: int32(w.stopRead), Events: unix.POLLIN},
		}
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == unix.EINTR {
				continue
			}
			w.fail(fmt.Errorf("await filesystem events: %w", err))
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}
		if _, err := unix.Read(w.inotify, buffer); err != nil {
			if err == unix.EINTR || err == unix.EAGAIN {
				continue
			}
			w.fail(fmt.Errorf("read filesystem events: %w", err))
			return
		}
		w.signal()
	}
}

func (w *unixWatcher) signal() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

func (w *unixWatcher) fail(err error) {
	select {
	case w.errs <- err:
	default:
	}
}
