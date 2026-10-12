// Package run implements fledge supervise run: the supervision daemon of one
// Herdr session.
//
//	run.go     the lock, heartbeat, and main loop
//	daemon.go  the watched set and the wake, ring, and dedupe rules
//	events.go  Herdr event subscriptions with reconnect backoff
package run
