//go:build windows

package lifecycle

import "syscall"

// watcherProcessAttributes best-effort detaches the watcher from the console's
// Ctrl-C handling; Windows has no session equivalent.
func watcherProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
