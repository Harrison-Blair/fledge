package statedir

import "path/filepath"

const contextDirectory = "context"

// Context returns the folder holding the persisted agent context-usage report
// for one Herdr session. It lives beside the watcher and status state under the
// session's ephemeral tmp folder.
func Context(root, session string) string {
	return filepath.Join(TempSession(root, session), contextDirectory)
}
