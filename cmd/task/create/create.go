// Package create wires task creation.
package create

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/task/create"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options create.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "create", Short: "Record a new task with a title and brief", Args: cobra.NoArgs,
		Long: "Record a new task in the created state.\n\nThe brief is the text later delivered to the owner by task assign. The creator is\nthe caller's agent record, or null when the caller is unregistered.\n\n--parent makes the new task a subtask of an existing task that is not verified,\nlanded, or cancelled. The parent is fixed at creation and is for grouping and progress\nonly; it does not wait for its subtasks. --after names an existing prerequisite\ntask (repeatable); see task depend.\n\n--delivery-mode sets how fledge task land delivers the task: local merges its\nbranch into the target branch, pr opens and merges a pull request, and none\nrefuses to land it. Without it the mode is .fledge/config.toml delivery_mode,\nelse none. --base-branch names the branch the task lands into instead of the\nrepository integration branch.\n\nThe brief is any nonblank UTF-8 text without NUL, stored exactly as given.\nfledge task template prints an optional six-section skeleton to start from."}
	f := cmd.Flags()
	f.StringVar(&options.Title, "title", "", "Single-line task title")
	f.StringVar(&options.Body, "body", "", "Brief text")
	f.StringVar(&options.File, "file", "", "UTF-8 brief file, or - for stdin")
	f.StringVar(&options.Parent, "parent", "", "Parent task ID")
	f.StringArrayVar(&options.After, "after", nil, "Prerequisite task ID (repeatable)")
	f.StringVar(&options.DeliveryMode, "delivery-mode", "", "How task land delivers the task: local, pr, or none")
	f.StringVar(&options.BaseBranch, "base-branch", "", "Branch the task lands into")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		options.DeliveryModeSet = f.Changed("delivery-mode")
		options.BaseBranchSet = f.Changed("base-branch")
		return cli.Finish(create.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, create.Render)
	}
	return cmd
}
