// Package supervise holds what the supervision daemon shares with the
// commands that start, stop, and report on it. The daemon of one Herdr
// session is live exactly while it holds an exclusive flock on the lock file
// in its directory; the lock file also carries its status.
//
//	lock.go      per-session directory, the liveness lock, and status
//	start.go     starting a detached daemon, replacing another version, and stopping it
//	episodes.go  wake dedupe state that survives a daemon restart
package supervise
