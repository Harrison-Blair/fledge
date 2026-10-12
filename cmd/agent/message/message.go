// Package message wires acknowledged prompt submission.
package message

import (
	"time"

	"github.com/Harrison-Blair/fledge/internal/agent/message"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/selector"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options message.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "message", Short: "Store a message in a registered agent's inbox and ring its doorbell, or type it to an unregistered agent", Args: cobra.NoArgs,
		Long: "Deliver a message without waiting for agent completion.\n\nA registered recipient (one with a Fledge record) gets the body stored as an inbox\nrecord and only a one-line doorbell typed into its pane, for example:\n  ᛉ fledge inbox · message from orchestrator (w1:p1) · id 0a1b2c3d · read: fledge inbox read --id 0a1b2c3d\nThe recipient reads the body with fledge inbox read. A body is never typed into a\nregistered recipient's pane; a doorbell may be rung again by the supervisor. If the\nrecipient is blocked or not ready, the message is stored but not rung (deferred), the\noutcome is partial, and the supervisor rings it later.\n\nAn unregistered recipient gets the body typed directly after one header line naming\nthe sender and a message ID:\n  ᛉ fledge message from orchestrator (w1:p1) · id m-0a1b2c · reply: fledge agent message --name orchestrator\n\n--reply-to marks an inbox record addressed to you as answered by this message.\n\nWith --confirm, Herdr waits up to --timeout for the agent to show activity after\nsubmission. A stalled confirmation or wait timeout is never retried by this command:\nthe text was, or may have been, submitted, so do not resend it. An agent already working\nis messaged, but this prompt's start is reported as not confirmed.\n\nRepeat --name, --pane, or --id, or use filter flags instead, to message several\nagents, one after another: each registered recipient gets its own inbox record, and\nthe result has one row per target. --confirm and --timeout apply to each target.\nFilter flags AND together; repeating one ORs its values; matches never include\nthe caller. Any failed delivery makes the outcome partial with exit status 1."}
	f := cmd.Flags()
	f.StringArrayVar(&options.Names, "name", nil, "Live agent name (repeatable)")
	f.StringArrayVar(&options.Panes, "pane", nil, "Hosting pane ID (repeatable)")
	f.StringArrayVar(&options.IDs, "id", nil, "Fledge agent record ID (repeatable; follows its terminal to a new pane; fails if the terminal is gone)")
	selector.BindFlags(f, &options.Filter)
	f.StringVar(&options.Body, "body", "", "Message text")
	f.StringVar(&options.File, "file", "", "UTF-8 message file, or - for stdin")
	f.StringVar(&options.ReplyTo, "reply-to", "", "Inbox record ID this message answers (you must be its recipient)")
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
		return cli.Finish(message.Run(cmd.Context(), libagent.FromEnvironment(timeout), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, message.Render)
	}
	return cmd
}
