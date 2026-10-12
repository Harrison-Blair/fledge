// Package read wires reading inbox records.
package read

import (
	"github.com/Harrison-Blair/fledge/internal/inbox/read"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options read.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "read", Short: "Print inbox records with a reply command and mark them read", Args: cobra.NoArgs,
		Long: "Print the caller's inbox records: a header, the body, and a ready-made reply\ncommand, then mark them read. Without --id, read every unread record, oldest\nfirst. Reading a record again keeps its first read time. Only the recipient\nmay read a record.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cli.Finish(read.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, read.Render)
		}}
	cmd.Flags().StringArrayVar(&options.IDs, "id", nil, "Inbox message ID (repeatable)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	return cmd
}
