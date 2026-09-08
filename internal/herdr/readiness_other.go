//go:build !linux

package herdr

// observePaneReady fails closed: without /proc and PTY ioctls this platform
// cannot tell whether a pane's shell is at an interactive prompt.
func observePaneReady(ProcessInfo) (bool, error) {
	return false, errReadinessUnsupported
}
