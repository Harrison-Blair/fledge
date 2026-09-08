// Package lock serializes the Fledge operations that mutate one project's
// session records.
//
// Files:
//   - lock_linux.go  Acquire backed by a flock on the project's .fledge directory
//   - lock_other.go  Acquire stub reporting that locking is unsupported
package lock
