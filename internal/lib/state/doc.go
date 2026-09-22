// Package state stores JSON records under a directory, safe for concurrent use
// by several Fledge processes.
// store.go defines the Store API, record paths, and ids; file.go provides the
// store lock and atomic file writes.
package state
