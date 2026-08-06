package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/spf13/cobra"
)

type coordinationManager interface {
	AgentList(context.Context, string) ([]messaging.Agent, error)
	TaskAssign(context.Context, string, string, string, string) (messaging.Task, error)
	TaskProgress(context.Context, string, string, string) (messaging.Task, error)
	TaskTransition(context.Context, string, string, messaging.TaskStatus, string) (messaging.Task, error)
	TaskList(context.Context, string) ([]messaging.Task, error)
	TaskShow(context.Context, string, string) (messaging.Task, error)
}

func newAgentListCommand(manager coordinationManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List the durable pane-bound agent registry", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			agents, err := manager.AgentList(cmd.Context(), dir)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "NAME\tSTATE\tPANE\tHARNESS\tCAN DELEGATE\tPARENT TASK"); err != nil {
				return err
			}
			for _, agent := range agents {
				state := agent.Status
				if !agent.Active {
					state = "stopped"
				}
				if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%t\t%s\n", agent.Name, state, agent.PaneID, agent.Harness, agent.CanDelegate, agent.ParentTaskID); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	}
}

func newAgentTaskCommand(manager coordinationManager, getwd func() (string, error)) *cobra.Command {
	task := &cobra.Command{Use: "task", Short: "Coordinate durable agent tasks", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	task.AddCommand(newTaskAssignCommand(manager, getwd))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "progress", messaging.TaskActive, true))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "blocked", messaging.TaskBlocked, true))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "needs-decision", messaging.TaskNeedsDecision, true))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "resume", messaging.TaskActive, false))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "complete", messaging.TaskCompleted, false))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "fail", messaging.TaskFailed, true))
	task.AddCommand(newTaskDetailCommand(manager, getwd, "cancel", messaging.TaskCanceled, false))
	task.AddCommand(newTaskListCommand(manager, getwd))
	task.AddCommand(newTaskShowCommand(manager, getwd))
	return task
}

func newTaskAssignCommand(manager coordinationManager, getwd func() (string, error)) *cobra.Command {
	var parent, bodyFile string
	command := &cobra.Command{Use: "assign <agent> [task]", Short: "Assign work to an agent", Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := messageBody(cmd, args[1:], bodyFile)
			if err != nil {
				return err
			}
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			task, err := manager.TaskAssign(cmd.Context(), dir, args[0], parent, body)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Assigned task %s to %s.\n", task.ID, task.Assignee)
			return err
		}}
	command.Flags().StringVar(&parent, "parent-task", "", "parent task for delegated work")
	addBodyFileFlag(command, &bodyFile)
	return command
}

func newTaskDetailCommand(manager coordinationManager, getwd func() (string, error), name string, status messaging.TaskStatus, required bool) *cobra.Command {
	var bodyFile string
	command := &cobra.Command{Use: name + " <task-id> [detail]", Short: taskTransitionShort(name), Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			detail, err := optionalTaskDetail(cmd, args[1:], bodyFile, required)
			if err != nil {
				return err
			}
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			var task messaging.Task
			if name == "progress" {
				task, err = manager.TaskProgress(cmd.Context(), dir, args[0], detail)
			} else {
				task, err = manager.TaskTransition(cmd.Context(), dir, args[0], status, detail)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Task %s is %s.\n", task.ID, task.Status)
			return err
		}}
	addBodyFileFlag(command, &bodyFile)
	return command
}

func optionalTaskDetail(cmd *cobra.Command, args []string, bodyFile string, required bool) (string, error) {
	if len(args) > 0 || bodyFile != "" {
		return messageBody(cmd, args, bodyFile)
	}
	if required {
		return "", fmt.Errorf("supply transition detail as an argument or with --file")
	}
	return "", nil
}

func taskTransitionShort(name string) string {
	return map[string]string{"progress": "Record task progress", "blocked": "Report a blocked task", "needs-decision": "Request a decision",
		"resume": "Resume a blocked task", "complete": "Complete a task", "fail": "Fail a task", "cancel": "Cancel a task and its descendants"}[name]
}

func newTaskListCommand(manager coordinationManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List visible tasks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		dir, err := currentDirectory(getwd)
		if err != nil {
			return err
		}
		tasks, err := manager.TaskList(cmd.Context(), dir)
		if err != nil {
			return err
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "ID\tSTATUS\tASSIGNEE\tASSIGNER\tPARENT\tTASK"); err != nil {
			return err
		}
		for _, task := range tasks {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", task.ID, task.Status, task.Assignee, task.Assigner, task.ParentID, strings.Join(strings.Fields(task.Description), " ")); err != nil {
				return err
			}
		}
		return writer.Flush()
	}}
}

func newTaskShowCommand(manager coordinationManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{Use: "show <task-id>", Short: "Show one visible task", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := currentDirectory(getwd)
		if err != nil {
			return err
		}
		task, err := manager.TaskShow(cmd.Context(), dir, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\nStatus: %s\nAssignee: %s\nAssigner: %s\nParent: %s\nTask:\n%s\nDetail:\n%s\n", task.ID, task.Status, task.Assignee, task.Assigner, task.ParentID, task.Description, task.Detail)
		return err
	}}
}
