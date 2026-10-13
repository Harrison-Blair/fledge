// Package size wires the task review-size report.
package size

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/size"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options size.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "size", Short: "Show how large a task's branch is for review", Args: cobra.NoArgs,
		Long: "Show how large a task's branch is for review, as task land measures it.\n\n" +
			"The diff is <target>...<head>: head is the task's verified commit, else the\n" +
			"head of its branch; target is its base branch, else the integration branch.\n" +
			"The measures are hand-written source lines, guard-eligible files, code\n" +
			"domains, and total changed lines.\n\n" +
			"Delivery modes local and none compare with the task limits and\n" +
			"refs/heads/<target>; pr and auto compare with the pull request target and\n" +
			"ceiling and refs/remotes/origin/<target>. When that ref is missing, the other\n" +
			"one is used and the output says so.\n\n" +
			"The report is advisory: an over-limit size never makes the command fail. It\n" +
			"writes nothing and does not need Herdr.",
	}
	f := cmd.Flags()
	f.StringVar(&options.ID, "id", "", "Task ID")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return cli.Finish(size.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), asJSON, size.Render)
	}
	return cmd
}
