// Package profiles wires the read-only agent profile listing.
package profiles

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/agent/profiles"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "profiles [NAME]", Short: "List agent profiles, or show one resolved profile",
		Long: "List agent profiles, or show one resolved profile with its source, base, launch settings, and role.\n\nBuilt-in profiles (orchestrator, implementer, planner, reviewer, verifier) ship with the\nbinary. Files in .fledge/profiles/<name>.toml at the invoking checkout's Git top level\noverride a same-name built-in or add custom profiles. Listing reads files only; it needs\nno Herdr session and writes nothing.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var options profiles.Options
			if len(args) == 1 {
				options.Name = args[0]
			}
			cwd, _ := os.Getwd()
			return libagent.Finish(profiles.Run(cmd.Context(), cwd, options), cmd.OutOrStdout(), asJSON, profiles.Render)
		}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
