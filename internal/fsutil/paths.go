package fsutil

import (
	"path/filepath"
	"regexp"
)

const (
	stateDirectory   = ".fledge"
	logsDirectory    = "logs"
	tmpDirectory     = "tmp"
	contextDirectory = "context"
)

var (
	legacySessionDirPattern = regexp.MustCompile(`^fledge-[0-9a-f]{32}$`)
	sessionDirPattern       = regexp.MustCompile(`^fledge-[a-z0-9]+(?:-[a-z0-9]+)*-[0-9a-f]{8}$`)
)

// Root returns the Fledge state directory belonging to a project root.
func Root(root string) string { return filepath.Join(root, stateDirectory) }

// Logs returns the directory holding every session's log folder.
func Logs(root string) string { return filepath.Join(Root(root), logsDirectory) }

// Session returns the log folder belonging to one Herdr session.
func Session(root, session string) string { return filepath.Join(Logs(root), session) }

// Temp returns the directory holding ephemeral state for active sessions.
func Temp(root string) string { return filepath.Join(Root(root), tmpDirectory) }

// TempSession returns the ephemeral state folder belonging to one Herdr session.
func TempSession(root, session string) string { return filepath.Join(Temp(root), session) }

// Context returns the folder holding the persisted agent context-usage report
// for one Herdr session. It lives beside the watcher and status state under the
// session's ephemeral tmp folder.
func Context(root, session string) string {
	return filepath.Join(TempSession(root, session), contextDirectory)
}

// ValidSessionDirName reports whether name is a Fledge session name that is
// safe to use as a single log folder name. Both the current
// fledge-<slug>-<8 hex> and the legacy fledge-<32 hex> grammars are accepted;
// every other name, including blank names, path separators, "." and "..", is
// rejected.
func ValidSessionDirName(name string) bool {
	return sessionDirPattern.MatchString(name) || legacySessionDirPattern.MatchString(name)
}
