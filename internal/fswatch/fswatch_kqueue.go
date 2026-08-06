//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package fswatch

import (
	"fmt"
	"sync"

	"golang.org/x/sys/unix"
)

const vnodeInterest = unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME |
	unix.NOTE_EXTEND | unix.NOTE_ATTRIB | unix.NOTE_LINK

const (
	directoryIdent = 1
	stopIdent      = 2
)

type kqueueWatcher struct {
	kq        int
	directory int
	stopRead  int
	stopWrite int
	events    chan struct{}
	errs      chan error
	finished  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// open watches dir through kqueue's vnode filter. kqueue reports that a
// directory changed but not which entry changed, so name is accepted and
// ignored: every signal is coalesced and every reader re-reads the state it
// cares about, which makes a spurious wake harmless and a missed one the only
// real failure.
func open(dir, _ string) (Watcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("create kqueue watch for %q: %w", dir, err)
	}
	unix.CloseOnExec(kq)
	directory, err := unix.Open(dir, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Close(kq)
		return nil, fmt.Errorf("open directory %q for watching: %w", dir, err)
	}
	var pipe [2]int
	if err := unix.Pipe(pipe[:]); err != nil {
		unix.Close(directory)
		unix.Close(kq)
		return nil, fmt.Errorf("create watch stop pipe for %q: %w", dir, err)
	}
	unix.CloseOnExec(pipe[0])
	unix.CloseOnExec(pipe[1])
	changes := []unix.Kevent_t{
		{Ident: uint64(directory), Filter: unix.EVFILT_VNODE,
			Flags: unix.EV_ADD | unix.EV_CLEAR, Fflags: vnodeInterest},
		{Ident: uint64(pipe[0]), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD},
	}
	if _, err := unix.Kevent(kq, changes, nil, nil); err != nil {
		unix.Close(pipe[0])
		unix.Close(pipe[1])
		unix.Close(directory)
		unix.Close(kq)
		return nil, fmt.Errorf("watch directory %q: %w", dir, err)
	}
	w := &kqueueWatcher{
		kq: kq, directory: directory, stopRead: pipe[0], stopWrite: pipe[1],
		events: make(chan struct{}, 1), errs: make(chan error, 1), finished: make(chan struct{}),
	}
	go w.read()
	return w, nil
}

func (w *kqueueWatcher) Events() <-chan struct{} { return w.events }
func (w *kqueueWatcher) Errors() <-chan error    { return w.errs }

func (w *kqueueWatcher) Close() error {
	w.closeOnce.Do(func() {
		// The reader is blocked in kevent on the stop pipe; only once it has
		// returned is closing these descriptors free of a recycling race.
		if _, err := unix.Write(w.stopWrite, []byte{0}); err != nil {
			w.closeErr = fmt.Errorf("stop filesystem watch: %w", err)
		}
		<-w.finished
		for _, fd := range []int{w.kq, w.directory, w.stopRead, w.stopWrite} {
			if err := unix.Close(fd); err != nil && w.closeErr == nil {
				w.closeErr = fmt.Errorf("close filesystem watch: %w", err)
			}
		}
	})
	return w.closeErr
}

func (w *kqueueWatcher) read() {
	defer close(w.finished)
	pending := make([]unix.Kevent_t, 8)
	for {
		count, err := unix.Kevent(w.kq, nil, pending, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			w.fail(fmt.Errorf("await filesystem events: %w", err))
			return
		}
		changed := false
		for _, event := range pending[:count] {
			if event.Filter == unix.EVFILT_READ && int(event.Ident) == w.stopRead {
				return
			}
			if event.Flags&unix.EV_ERROR != 0 {
				w.fail(fmt.Errorf("await filesystem events: kevent error %d", event.Data))
				return
			}
			changed = true
		}
		if changed {
			w.signal()
		}
	}
}

func (w *kqueueWatcher) signal() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

func (w *kqueueWatcher) fail(err error) {
	select {
	case w.errs <- err:
	default:
	}
}
