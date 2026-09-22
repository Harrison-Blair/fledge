// Package create wires task creation.
package create

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/create"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options create.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "create", Short: "Record a new task with a title and brief", Args: cobra.NoArgs,
		Long: "Record a new task in the created state.\n\nThe brief is the text later delivered to the owner by task assign. The creator is\nthe caller's agent record, or null when the caller is unregistered."}
	f := cmd.Flags()
	f.StringVar(&options.Title, "title", "", "Single-line task title")
	f.StringVar(&options.Body, "body", "", "Brief text")
	f.StringVar(&options.File, "file", "", "UTF-8 brief file, or - for stdin")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		return libagent.Finish(create.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, create.Render)
	}
	return cmd
}
