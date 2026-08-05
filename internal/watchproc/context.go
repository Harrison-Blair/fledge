package watchproc

import (
	"context"
	"os"

	"github.com/Harrison-Blair/fledge/internal/agentcontext"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

// contextRefresher returns the watcher's best-effort context-usage refresh hook.
// On each lifecycle transition the engine hands it a fresh snapshot; it rebuilds
// the report and persists it beside the session's other ephemeral state. Any
// failure is logged and swallowed — a stale usage cache must never disturb
// supervision.
func contextRefresher(root, session string, log func(string)) func(context.Context, herdr.Snapshot) {
	return func(ctx context.Context, snapshot herdr.Snapshot) {
		home, err := os.UserHomeDir()
		if err != nil {
			log("context refresh skipped: resolve home directory: " + err.Error())
			return
		}
		deps := agentcontext.ProductionDeps(ctx, home)
		report := agentcontext.Build(agentcontext.LiveAgents(snapshot), deps)
		if err := agentcontext.Persist(statedir.Context(root, session), report); err != nil {
			log("context refresh failed: " + err.Error())
		}
	}
}
