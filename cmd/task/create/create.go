// Package create wires task creation.
package create

import (
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/task/create"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options create.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "create", Short: "Record a new task with a title and brief", Args: cobra.NoArgs,
		Long: "Record a new task in the created state.\n\nThe brief is the text later delivered to the owner by task assign. The creator is\nthe caller's agent record, or null when the caller is unregistered.\n\n--parent makes the new task a subtask of an existing task that is not verified\nor cancelled. The parent is fixed at creation and is for grouping and progress\nonly; it does not wait for its subtasks. --after names an existing prerequisite\ntask (repeatable); see task depend.\n\nThe brief must follow the brief template: the six sections Objective, Acceptance\ncriteria, Scope, Known facts, Deliverables, and Constraints as \"## \" headings, in\nthat order, each with content. fledge task template prints it. --freeform skips\nthe check; a brief off the template fails with task_brief_incomplete."}
	f := cmd.Flags()
	f.StringVar(&options.Title, "title", "", "Single-line task title")
	f.StringVar(&options.Body, "body", "", "Brief text")
	f.StringVar(&options.File, "file", "", "UTF-8 brief file, or - for stdin")
	f.StringVar(&options.Parent, "parent", "", "Parent task ID")
	f.StringArrayVar(&options.After, "after", nil, "Prerequisite task ID (repeatable)")
	f.BoolVar(&options.Freeform, "freeform", false, "Skip the brief template check")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		return libagent.Finish(create.Run(cmd.Context(), libagent.FromEnvironment(0), options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, create.Render)
	}
	return cmd
}
