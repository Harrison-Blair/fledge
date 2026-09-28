// Package template wires the task brief and proposal skeletons.
package template

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/template"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options template.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "template", Short: "Print the task brief or proposal skeleton", Args: cobra.NoArgs,
		Long: "Print the task brief skeleton: the six required headings, each with a hint to\nreplace. Fill it in for task create rather than writing the headings from\nmemory. --proposal prints a task import proposal skeleton instead. Never\ncontacts Herdr or reads task state."}
	cmd.Flags().BoolVar(&options.Proposal, "proposal", false, "Print the proposal skeleton instead of the brief skeleton")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(template.Run(options), cmd.OutOrStdout(), asJSON, template.Render)
	}
	return cmd
}
