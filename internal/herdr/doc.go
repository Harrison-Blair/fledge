// Package herdr provides the small Herder CLI surface used by Fledge.
//
// Files:
//   - herdr.go            Client construction, session listing, and
//     Launch/Stop of a Herder session
//   - api.go              workspace, tab, pane, and agent types, and the
//     Client methods that invoke the Herder socket API
//   - render.go           bounded rendering of argument vectors and captured
//     output for error text
//   - readiness.go        the pane readiness gate StartAgent waits on, its
//     timing, and ReadinessError
//   - readiness_linux.go  Linux observation of a pane's shell through /proc
//     and PTY ioctls
//   - readiness_other.go  fail-closed observation for other platforms
package herdr
