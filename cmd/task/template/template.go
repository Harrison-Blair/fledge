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
		Long: "Print the task brief skeleton: six headings, each with a hint to replace.\nThe template is optional; task create and task import accept any nonblank brief,\nincluding the unfilled skeleton. --proposal prints a task import proposal\nskeleton instead. Never contacts Herdr or reads task state."}
	cmd.Flags().BoolVar(&options.Proposal, "proposal", false, "Print the proposal skeleton instead of the brief skeleton")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return libagent.Finish(template.Run(options), cmd.OutOrStdout(), asJSON, template.Render)
	}
	return cmd
}
