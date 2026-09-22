// Package state stores JSON records under a directory, safe for concurrent use
// by several Fledge processes. The directory's parent must already exist.
// store.go defines the Store API, record paths, and ids; tx.go provides the
// locked transaction handle and record archiving; file.go provides the store
// lock and atomic file writes.
package state
