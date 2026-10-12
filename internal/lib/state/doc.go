// Package state stores JSON records under a directory, safe for concurrent use
// by several Fledge processes. The directory's parent must already exist.
// store.go defines the Store API, record paths, ids, and marker listing;
// tx.go provides the locked transaction handle, record archiving, and marker
// creation; file.go provides the store lock and atomic file writes.
package state
