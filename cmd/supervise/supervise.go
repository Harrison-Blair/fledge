// Package supervise wires the supervision daemon's run, status, and stop.
package supervise

import (
	"github.com/Harrison-Blair/fledge/cmd/supervise/run"
	"github.com/Harrison-Blair/fledge/cmd/supervise/status"
	"github.com/Harrison-Blair/fledge/cmd/supervise/stop"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "supervise", Short: "Run, inspect, or stop the session's supervision daemon", Args: cobra.NoArgs,
		Long: "The supervision daemon watches a Herdr session's workers and writes wakes to their\nparents' inboxes: blocked, exited, quiet (idle while owning an assigned task), and\nunacknowledged (an inbox message still unread). agent spawn, task assign, and a\nmessage stored but not rung start it automatically; it exits after 60 seconds with\nnothing to watch.",
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	cmd.AddCommand(run.New(), status.New(), stop.New())
	return cmd
}
