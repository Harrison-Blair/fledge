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
	cmd := &cobra.Command{Use: "message", Short: "Submit a message, with a sender header, without waiting for agent completion", Args: cobra.NoArgs,
		Long: "Submit a message without waiting for agent completion.\n\nThe delivered text starts with one header line naming the sender and a message ID,\nfor example:\n  ᛉ fledge message from orchestrator (w1:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\nThe reply command appears only when the sender is a named agent."}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (fails if the pane now hosts another terminal)")
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
