// Package statedir resolves the paths of Fledge's project-local state
// directories, including the per-session log folders shared by the messaging
// audit log and the debug log.
package statedir

import (
	"path/filepath"
	"regexp"
)

const (
	stateDirectory = ".fledge"
	logsDirectory  = "logs"
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

// ValidSessionDirName reports whether name is a Fledge session name that is
// safe to use as a single log folder name. Both the current
// fledge-<slug>-<8 hex> and the legacy fledge-<32 hex> grammars are accepted;
// every other name, including blank names, path separators, "." and "..", is
// rejected.
func ValidSessionDirName(name string) bool {
	return sessionDirPattern.MatchString(name) || legacySessionDirPattern.MatchString(name)
}
