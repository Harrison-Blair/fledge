// Package profiles wires the read-only agent profile listing.
package profiles

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/agent/profiles"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "profiles [NAME]", Short: "List agent profiles, or show one profile's brief",
		Long: "List agent profiles, or show one profile's source and the brief a spawn sends: its\nMarkdown role instructions followed by the shared Fledge protocol. Spawn appends the\ndestination repository's project memory index at launch; the brief shown here omits it.\n\nBuilt-in profiles (orchestrator, planner, researcher, implementer, debugger, integrator,\nreviewer, verifier) ship with the binary. A file .fledge/profiles/NAME.md at the invoking\ncheckout's Git top level replaces a same-name built-in entirely or adds a custom profile;\nits text must be nonblank UTF-8 without NUL. Profiles choose no harness, model, or native\narguments. Legacy TOML profiles are rejected with migration guidance.\nListing reads files only; it needs no Herdr session and writes nothing.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var options profiles.Options
			if len(args) == 1 {
				options.Name = args[0]
			}
			cwd, _ := os.Getwd()
			return cli.Finish(profiles.Run(cmd.Context(), cwd, options), cmd.OutOrStdout(), asJSON, profiles.Render)
		}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
