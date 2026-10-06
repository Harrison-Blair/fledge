// Package usage wires per-agent usage reporting.
package usage

import (
	"github.com/Harrison-Blair/fledge/internal/agent/usage"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/harnessenv"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options usage.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "usage", Short: "Report token and cost usage per agent from harness session stores", Args: cobra.NoArgs,
		Long: "Report each selected agent's whole-session usage: models, turns, input,\noutput, and cache tokens, harness-recorded cost, and elapsed time since\nregistration. Tokens are measured from the harness's own session store; a\ncost is only what the harness recorded, shown as an estimate. An agent whose\nusage cannot be read has basis unavailable and the reason.\n\nEach agent's harness and native session ref come from the live agent, else\nfrom its Fledge record, so --id also reports an agent that has ended. A live\nref is recorded on the agent's record. Harness-internal sub-agents count in\nthe agent's totals and appear as subagents in --json.\n\nRepeat --name, --pane, or --id, or use filter flags instead. Filter flags AND\ntogether; repeating one ORs its values; matches never include the caller.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return libagent.Finish(usage.Run(cmd.Context(), libagent.FromEnvironment(0), harnessenv.Local(), options), cmd.OutOrStdout(), asJSON, usage.Render)
		}}
	f := cmd.Flags()
	f.StringArrayVar(&options.Names, "name", nil, "Live agent name (repeatable)")
	f.StringArrayVar(&options.Panes, "pane", nil, "Hosting pane ID (repeatable)")
	f.StringArrayVar(&options.IDs, "id", nil, "Fledge agent record ID (repeatable; follows its terminal to a new pane; reports from the record if the agent has ended)")
	selector.BindFlags(f, &options.Filter)
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
