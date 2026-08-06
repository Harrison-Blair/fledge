//go:build windows

package fswatch

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const notifyFilter = windows.FILE_NOTIFY_CHANGE_FILE_NAME |
	windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES |
	windows.FILE_NOTIFY_CHANGE_SIZE |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE |
	windows.FILE_NOTIFY_CHANGE_CREATION

// fileNotifyInformation mirrors the Win32 FILE_NOTIFY_INFORMATION header. The
// variable-length UTF-16 name follows it in the same buffer.
type fileNotifyInformation struct {
	NextEntryOffset uint32
	Action          uint32
	FileNameLength  uint32
}

const notifyHeaderSize = int(unsafe.Sizeof(fileNotifyInformation{}))

type windowsWatcher struct {
	directory windows.Handle
	overlap   windows.Overlapped
	stop      windows.Handle
	events    chan struct{}
	errs      chan error
	finished  chan struct{}
	closeOnce sync.Once
	closeErr  error
}

// open watches dir through overlapped ReadDirectoryChangesW, reporting either
// every entry change (name == "") or only changes to the named entry. A second
// event object carries the stop request, so the reader waits on both without
// ever timing a poll.
func open(dir, name string) (Watcher, error) {
	path, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return nil, fmt.Errorf("watch directory %q: %w", dir, err)
	}
	directory, err := windows.CreateFile(path, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open directory %q for watching: %w", dir, err)
	}
	// Manual-reset, initially unsignalled: the read completion and the stop
	// request must both stay latched until the reader has observed them.
	readEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(directory)
		return nil, fmt.Errorf("create watch completion event for %q: %w", dir, err)
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		windows.CloseHandle(readEvent)
		windows.CloseHandle(directory)
		return nil, fmt.Errorf("create watch stop event for %q: %w", dir, err)
	}
	w := &windowsWatcher{
		directory: directory, stop: stop,
		events: make(chan struct{}, 1), errs: make(chan error, 1), finished: make(chan struct{}),
	}
	w.overlap.HEvent = readEvent
	go w.read(name)
	return w, nil
}

func (w *windowsWatcher) Events() <-chan struct{} { return w.events }
func (w *windowsWatcher) Errors() <-chan error    { return w.errs }

func (w *windowsWatcher) Close() error {
	w.closeOnce.Do(func() {
		if err := windows.SetEvent(w.stop); err != nil {
			w.closeErr = fmt.Errorf("stop filesystem watch: %w", err)
		}
		<-w.finished
		for _, handle := range []windows.Handle{w.directory, w.overlap.HEvent, w.stop} {
			if err := windows.CloseHandle(handle); err != nil && w.closeErr == nil {
				w.closeErr = fmt.Errorf("close filesystem watch: %w", err)
			}
		}
	})
	return w.closeErr
}

func (w *windowsWatcher) read(name string) {
	defer close(w.finished)
	// ReadDirectoryChangesW requires a DWORD-aligned buffer, which a uint32
	// backing array guarantees.
	aligned := make([]uint32, 16*1024/4)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&aligned[0])), len(aligned)*4)
	for {
		var returned uint32
		err := windows.ReadDirectoryChanges(w.directory, &buffer[0], uint32(len(buffer)),
			false, notifyFilter, &returned, &w.overlap, 0)
		if err != nil && !errors.Is(err, windows.ERROR_IO_PENDING) {
			w.fail(fmt.Errorf("request filesystem events: %w", err))
			return
		}
		waited, err := windows.WaitForMultipleObjects(
			[]windows.Handle{w.overlap.HEvent, w.stop}, false, windows.INFINITE)
		if err != nil {
			w.fail(fmt.Errorf("await filesystem events: %w", err))
			return
		}
		if waited == windows.WAIT_OBJECT_0+1 {
			// CancelIoEx then a blocking GetOverlappedResult: the kernel must be
			// done with the buffer before this goroutine lets it go.
			_ = windows.CancelIoEx(w.directory, &w.overlap)
			_ = windows.GetOverlappedResult(w.directory, &w.overlap, &returned, true)
			return
		}
		if waited != windows.WAIT_OBJECT_0 {
			w.fail(fmt.Errorf("await filesystem events: unexpected wait result %d", waited))
			return
		}
		if err := windows.GetOverlappedResult(w.directory, &w.overlap, &returned, false); err != nil {
			w.fail(fmt.Errorf("read filesystem events: %w", err))
			return
		}
		if err := windows.ResetEvent(w.overlap.HEvent); err != nil {
			w.fail(fmt.Errorf("rearm filesystem watch: %w", err))
			return
		}
		// A zero-length result means the kernel overflowed the buffer and dropped
		// the records. Something changed; the reader re-reads state regardless.
		if returned == 0 || matched(buffer[:returned], name) {
			w.signal()
		}
	}
}

// matched reports whether the batch holds a change the caller asked about. A
// malformed or truncated record is treated as a match: losing a wake is worse
// than delivering a spurious one.
func matched(batch []byte, name string) bool {
	if name == "" {
		return true
	}
	for offset := 0; offset+notifyHeaderSize <= len(batch); {
		header := (*fileNotifyInformation)(unsafe.Pointer(&batch[offset]))
		start := offset + notifyHeaderSize
		end := start + int(header.FileNameLength)
		if end > len(batch) || header.FileNameLength%2 != 0 {
			return true
		}
		entry := unsafe.Slice((*uint16)(unsafe.Pointer(&batch[start])), int(header.FileNameLength)/2)
		if windows.UTF16ToString(entry) == name {
			return true
		}
		if header.NextEntryOffset == 0 {
			return false
		}
		next := offset + int(header.NextEntryOffset)
		if next <= offset {
			return true
		}
		offset = next
	}
	return false
}

func (w *windowsWatcher) signal() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

func (w *windowsWatcher) fail(err error) {
	select {
	case w.errs <- err:
	default:
	}
}
