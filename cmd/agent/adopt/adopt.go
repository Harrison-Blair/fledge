// Package adopt wires registering an already-running agent.
package adopt

import (
	"github.com/Harrison-Blair/fledge/internal/agent/adopt"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options adopt.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "adopt", Short: "Register the caller's own running agent with a durable Fledge ID", Args: cobra.NoArgs,
		Long: "Register the caller's own running agent with a durable Fledge ID.\n\nRun adopt inside the agent, in its Herdr pane: adopt registers only the\nagent in the caller's pane (HERDR_PANE_ID) and has no flag to select another\npane. To register a different agent, run adopt in that agent's own pane, or\nstart the agent with fledge agent spawn.\n\nAdopt verifies through Herdr and Linux /proc that this command runs inside the\nagent: one of the pane's foreground processes must be this command or one of\nits ancestors. It verifies again just before its first change. A tool runner\nthat is not a descendant of the agent's process, such as a shared Codex daemon,\nfails with caller_identity_unverified and changes nothing; start Codex with\ncodex --no-daemon. Other platforms are not supported.\n\nAn unnamed agent needs --name, which adopt sets through Herdr and gives its pane\nlabel, and its tab label when the pane is alone there; a named agent keeps its\nname, and a different --name is refused (use agent rename). A named agent whose\nterminal already has a live record is refused with its existing ID; an unnamed\none is named and keeps that record and ID, which stores the new name and\ncurrent pane. A new record has no parent. The record lives in .fledge/state\nunder the repository's primary checkout.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cli.Finish(adopt.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, adopt.Render)
		}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Name to give an unnamed agent, or its current name")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
