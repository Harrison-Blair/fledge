package statedir

import "path/filepath"

const (
	watchDirectory  = "watch"
	statusDirectory = "status"
	statusExtension = ".status"
)

// WatchSession returns the watcher's ephemeral state folder belonging to one
// Herdr session.
func WatchSession(root, session string) string {
	return filepath.Join(TempSession(root, session), watchDirectory)
}

// StatusDir returns the folder holding every worker's status file for one
// Herdr session.
func StatusDir(root, session string) string {
	return filepath.Join(TempSession(root, session), statusDirectory)
}

// StatusFile returns the status file one agent appends its progress to.
func StatusFile(root, session, agent string) string {
	return filepath.Join(StatusDir(root, session), agent+statusExtension)
}
