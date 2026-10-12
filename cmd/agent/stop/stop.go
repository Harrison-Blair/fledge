// Package stop wires live agent teardown.
package stop

import (
	"github.com/Harrison-Blair/fledge/internal/agent/stop"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options stop.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "stop", Short: "Stop live agents by closing their panes", Args: cobra.NoArgs,
		Long: "Stop live agents by closing their panes and ending their Fledge records.\n\nAn agent that is working, blocked, or unknown is refused unless --force is\ngiven; a working agent is first given --grace to finish its turn.\n\nRepeat --name, --pane, or --id, or use filter flags instead, to stop several\nagents: each is looked up and stopped in turn, and the result has one row per\ntarget. --force and --grace apply to each target, so the worst-case wait is\n--grace times the number of targets. Filter flags AND together; repeating one\nORs its values; matches never include the caller. Any refused or failed\ntarget makes the outcome partial with exit status 1.\n\n--dry-run lists each target with its state and whether it would be stopped,\nrefused, or could not be looked up, without changing anything.", RunE: func(cmd *cobra.Command, _ []string) error {
			options.GraceSet = cmd.Flags().Changed("grace")
			return cli.Finish(stop.Run(cmd.Context(), client(options), options), cmd.OutOrStdout(), asJSON, stop.Render)
		}}
	f := cmd.Flags()
	f.StringArrayVar(&options.Names, "name", nil, "Live agent name (repeatable)")
	f.StringArrayVar(&options.Panes, "pane", nil, "Hosting pane ID (repeatable)")
	f.StringArrayVar(&options.IDs, "id", nil, "Fledge agent record ID (repeatable; follows its terminal to a new pane; fails if the terminal is gone)")
	selector.BindFlags(f, &options.Filter)
	f.BoolVar(&options.DryRun, "dry-run", false, "List each target and what stopping it would do, without changing anything")
	f.BoolVar(&options.Force, "force", false, "Stop even when the agent is working, blocked, or unknown, without waiting")
	f.DurationVar(&options.Grace, "grace", stop.DefaultGrace, "How long a working agent is given to finish its turn before stop refuses (0s through 60s; 0 refuses at once)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}

// client sizes the transport limit to outlast the settle wait.
func client(o stop.Options) libagent.Client {
	return libagent.FromEnvironment(o.EffectiveGrace())
}
