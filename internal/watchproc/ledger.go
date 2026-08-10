package watchproc

import "github.com/Harrison-Blair/fledge/internal/fswatch"

// watchLedger reports appends to the session ledger through the host's native
// change notification API, so a durable coordination event reaches the
// dispatcher without anything on either side keeping time.
func watchLedger(path string) (FileWatcher, error) { return fswatch.File(path) }
