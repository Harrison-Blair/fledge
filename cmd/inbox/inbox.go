// Package inbox wires the caller's inbox listing and its read subcommand.
package inbox

import (
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/cmd/inbox/read"
	"github.com/Harrison-Blair/fledge/internal/inbox"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func New() *cobra.Command {
	var options inbox.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "inbox", Short: "List your inbox: messages, task assignments, and supervisor wakes", Args: cobra.NoArgs,
		Long: "List the inbox of the caller's registered agent: unread records first, then read\nones with --all, each group oldest first, with the first line of each body.\n\nMessages to a registered agent are stored here and announced in its pane by a\none-line doorbell. Read the bodies with fledge inbox read.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			options.Width, options.Now = width(cmd), time.Now()
			return cli.Finish(inbox.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, inbox.Render)
		}}
	cmd.Flags().BoolVar(&options.All, "all", false, "Include read records")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.AddCommand(read.New())
	return cmd
}

// width is the terminal width of the command's output, or 0 (no limit) when
// it is not a terminal.
func width(cmd *cobra.Command) int {
	f, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return 0
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}
