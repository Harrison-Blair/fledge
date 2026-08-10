package lifecycle

import "syscall"

// watcherProcessAttributes puts the watcher in its own session, so the first
// Ctrl-C in the attached TUI reaches only the foreground harness and leaves
// the daemon running.
func watcherProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
