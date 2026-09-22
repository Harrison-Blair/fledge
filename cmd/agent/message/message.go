// Package message wires acknowledged prompt submission.
package message

import (
	"github.com/Harrison-Blair/fledge/internal/agent/message"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options message.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "message", Short: "Submit a message without waiting for agent completion", Args: cobra.NoArgs}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.Body, "body", "", "Message text")
	f.StringVar(&options.File, "file", "", "UTF-8 message file, or - for stdin")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		return libagent.Finish(message.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, message.Render)
	}
	return cmd
}
