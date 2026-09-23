// Package message wires acknowledged prompt submission.
package message

import (
	"time"

	"github.com/Harrison-Blair/fledge/internal/agent/message"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options message.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "message", Short: "Submit a message, with a sender header, without waiting for agent completion", Args: cobra.NoArgs,
		Long: "Submit a message without waiting for agent completion.\n\nThe delivered text starts with one header line naming the sender and a message ID,\nfor example:\n  ᛉ fledge message from orchestrator (w1:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\nThe reply command appears only when the sender is a named agent.\n\nWith --confirm, Herdr waits up to --timeout for the agent to show activity after\nsubmission. A stalled confirmation or wait timeout is never retried: the message\nwas, or may have been, submitted, so do not resend it. An agent already working\nis messaged, but this prompt's start is reported as not confirmed."}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.StringVar(&options.Body, "body", "", "Message text")
	f.StringVar(&options.File, "file", "", "UTF-8 message file, or - for stdin")
	f.BoolVar(&options.Confirm, "confirm", false, "Wait for observed agent activity after submission")
	f.DurationVar(&options.Timeout, "timeout", 10*time.Second, "Confirmation timeout (requires --confirm)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		options.TimeoutSet = f.Changed("timeout")
		var timeout time.Duration
		if options.Confirm {
			timeout = options.Timeout
		}
		return libagent.Finish(message.Run(cmd.Context(), libagent.FromEnvironment(timeout), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, message.Render)
	}
	return cmd
}
