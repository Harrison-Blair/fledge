// Package send wires raw terminal input without a sender header.
package send

import (
	"github.com/Harrison-Blair/fledge/internal/agent/send"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options send.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "send", Short: "Type raw text and press keys in a live agent's terminal, without a sender header", Args: cobra.NoArgs,
		Long: "Type raw text and press keys in a live agent's terminal, in any agent state.\n\nText is delivered first, as a paste, so it does not submit: a newline inside\nit is typed literally. Keys are pressed after it, in the order given; add\n--key enter to submit. Unknown key names reject the whole send before any\ninput is written. Nothing is prefixed, so the recipient has no reply channel;\nuse agent message for attributed messages. The result reports the agent's\nstatus observed before sending.\n\nExamples:\n  fledge agent send --name worker --text \"/model claude-haiku-4-5\" --key enter\n  (Claude Code's /model also saves the model as the default for new Claude sessions.)\n  fledge agent send --name worker --key down --key enter",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return libagent.Finish(send.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, send.Render)
		}}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Live agent name")
	f.StringVar(&options.Pane, "pane", "", "Hosting pane ID")
	f.StringVar(&options.ID, "id", "", "Fledge agent record ID (follows its terminal to a new pane; fails if the terminal is gone)")
	f.StringVar(&options.Text, "text", "", "Literal text to type as a paste; does not press enter")
	f.StringArrayVar(&options.Keys, "key", nil, "Logical key to press after the text, such as enter, esc, down, or ctrl+c (repeatable, in order)")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
