// Package importcmd wires task import.
package importcmd

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	taskimport "github.com/Harrison-Blair/fledge/internal/task/import"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options taskimport.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "import", Short: "Create a parent task and tasks from a proposal file", Args: cobra.NoArgs,
		Long: "Create the tasks a proposal file describes.\n\nA proposal is a TOML file: schema_version = 1, an optional [parent] with a title\nand brief, and one [[tasks]] entry per task with a key, title, brief, and after,\nthe local keys or existing task ids it waits on. fledge task template --proposal\nprints a skeleton. Every brief must follow the brief template.\n\nThe whole file is validated before the store is touched; then, under one store\nlock, the parent (if any) and the tasks are created in dependency order, with\neach after key resolved to the new task's id. --parent places the tasks under an\nexisting task that is not verified or cancelled instead; it conflicts with a\n[parent] in the file. --dry-run validates and prints the tasks in creation order\nwithout creating anything.\n\nBy convention a planner writes its proposal to .fledge/tmp/plans/<task-id>.toml,\nnamed for the planning task it was assigned; any path works."}
	f := cmd.Flags()
	f.StringVar(&options.File, "file", "", "Proposal TOML file, or - for stdin (required)")
	f.StringVar(&options.Parent, "parent", "", "Existing parent task ID")
	f.BoolVar(&options.DryRun, "dry-run", false, "Validate and show the tasks without creating them")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.FileSet = f.Changed("file")
		return libagent.Finish(taskimport.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, taskimport.Render)
	}
	return cmd
}
