// Package list wires agent enumeration.
package list

import (
	"github.com/Harrison-Blair/fledge/internal/agent/list"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options list.Options
	cmd := &cobra.Command{Use: "list", Short: "List all live Herdr agents", Args: cobra.NoArgs,
		Long: "List all live Herdr agents, or only those the filter flags select. Different\nflags AND together; repeating one flag ORs its values. --profile, --task,\n--worktree, --registered, --mine, and --parent match only agents with a live\nrecord in this repository.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return libagent.Finish(list.Run(cmd.Context(), libagent.FromEnvironment(0), options), cmd.OutOrStdout(), options.JSON, list.Render)
		}}
	f := cmd.Flags()
	f.BoolVar(&options.Mine, "mine", false, "Only agents whose parent is the caller's own record")
	f.StringVar(&options.Parent, "parent", "", "Only agents whose parent is this Fledge agent record ID")
	f.StringArrayVar(&options.States, "state", nil, "Only agents in this state: idle, working, blocked, done, or unknown (repeatable)")
	f.StringArrayVar(&options.Harnesses, "harness", nil, "Only agents running this harness (repeatable)")
	f.StringArrayVar(&options.Profiles, "profile", nil, "Only agents spawned with this profile (repeatable)")
	f.StringArrayVar(&options.Tasks, "task", nil, "Only the owner of this task ID (repeatable)")
	f.StringArrayVar(&options.Worktrees, "worktree", nil, "Only agents whose recorded worktree is this path (repeatable)")
	f.BoolVar(&options.Registered, "registered", false, "Only agents with a live record in this repository")
	f.BoolVar(&options.IDs, "ids", false, "Print one record ID per registered match instead of a table")
	f.BoolVar(&options.JSON, "json", false, "Emit a structured outcome")
	return cmd
}
