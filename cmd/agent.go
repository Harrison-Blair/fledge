package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/spf13/cobra"
)

func newAgentCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	agent := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents in this project's session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	agent.AddCommand(newAgentSpawnCommand(manager, getwd))
	agent.AddCommand(newAgentListCommand(manager, getwd))
	agent.AddCommand(newAgentTaskCommand(manager, getwd))
	agent.AddCommand(newAgentStopCommand(manager, getwd))
	agent.AddCommand(newAgentMessageCommand(manager, getwd))
	agent.AddCommand(newAgentModelsCommand(nil, nil))
	return agent
}

func newAgentMessageCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	message := &cobra.Command{
		Use:   "message",
		Short: "Exchange audited messages in this project's live session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	message.AddCommand(newMessageSendCommand(manager, getwd))
	message.AddCommand(newMessageReplyCommand(manager, getwd))
	message.AddCommand(newMessageInboxCommand(manager, getwd))
	return message
}

func newMessageSendCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	var bodyFile string
	command := &cobra.Command{
		Use:   "send <recipient> [text]",
		Short: "Send a message to a live agent",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := messageBody(cmd, args[1:], bodyFile)
			if err != nil {
				return err
			}
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			message, err := manager.SendMessage(cmd.Context(), dir, args[0], body)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Sent message %s to %s.\n", message.ID, message.Recipient)
			return err
		},
	}
	addBodyFileFlag(command, &bodyFile)
	return command
}

func newMessageReplyCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	var bodyFile string
	command := &cobra.Command{
		Use:   "reply <message-id> [text]",
		Short: "Send a correlated reply",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := messageBody(cmd, args[1:], bodyFile)
			if err != nil {
				return err
			}
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			reply, err := manager.ReplyMessage(cmd.Context(), dir, args[0], body)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Replied to message %s with %s.\n", args[0], reply.ID)
			return err
		},
	}
	addBodyFileFlag(command, &bodyFile)
	return command
}

func addBodyFileFlag(command *cobra.Command, bodyFile *string) {
	command.Flags().StringVarP(bodyFile, "file", "F", "", "read the message body from a file (- for stdin)")
}

// messageBody resolves a message body from exactly one of the optional text
// argument or --file, so bodies that shell quoting cannot carry still reach
// the manager intact.
func messageBody(cmd *cobra.Command, textArgs []string, bodyFile string) (string, error) {
	var body string
	switch {
	case len(textArgs) == 1 && bodyFile != "":
		return "", errors.New("supply the message body as an argument or with --file, not both")
	case len(textArgs) == 1:
		body = textArgs[0]
	case bodyFile != "":
		contents, err := readMessageBodyFile(cmd, bodyFile)
		if err != nil {
			return "", err
		}
		body = contents
	default:
		return "", errors.New("supply the message body as an argument or with --file")
	}
	if err := messaging.ValidateBody(body); err != nil {
		return "", err
	}
	return body, nil
}

func readMessageBodyFile(cmd *cobra.Command, bodyFile string) (string, error) {
	if bodyFile == "-" {
		contents, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", fmt.Errorf("read message body from stdin: %w", err)
		}
		return string(contents), nil
	}
	contents, err := os.ReadFile(bodyFile)
	if err != nil {
		return "", fmt.Errorf("read message body: %w", err)
	}
	return string(contents), nil
}

func newMessageInboxCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "inbox [identity]",
		Short: "Show an identity's active-session transcript",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			identity := ""
			if len(args) == 1 {
				identity = args[0]
			}
			messages, identity, err := manager.MessageInbox(cmd.Context(), dir, identity)
			if err != nil {
				return err
			}
			return writeInbox(cmd, messages, identity)
		},
	}
}

func writeInbox(cmd *cobra.Command, messages []messaging.Message, identity string) error {
	if len(messages) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Inbox is empty.")
		return err
	}
	for _, message := range messages {
		direction, peer := "received from", message.Sender
		if message.Sender == identity {
			direction, peer = "sent to", message.Recipient
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s %s\n", message.ID, message.Status, direction, peer); err != nil {
			return err
		}
		if message.ReplyTo != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  reply-to: %s\n", message.ReplyTo); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", strings.ReplaceAll(message.Body, "\n", "\n  ")); err != nil {
			return err
		}
		if message.Failure != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  failure: %s\n", strings.ReplaceAll(message.Failure, "\n", "\n  ")); err != nil {
				return err
			}
		}
	}
	return nil
}

func newAgentStopCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop an agent and close its pane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			return manager.StopAgent(cmd.Context(), dir, args[0])
		},
	}
}

func newAgentSpawnCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	var options lifecycle.SpawnOptions
	command := &cobra.Command{
		Use:   "spawn [-- native-args...]",
		Short: "Spawn an agent in a dedicated tab",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
				return fmt.Errorf("native agent arguments must follow --")
			}
			dir, err := currentDirectory(getwd)
			if err != nil {
				return err
			}
			options.NativeArgs = append([]string(nil), args...)
			options.ModelSet = cmd.Flags().Changed("model")
			return manager.Spawn(cmd.Context(), dir, options)
		},
	}
	command.Flags().StringVarP(&options.Name, "name", "n", "", "unique agent and tab name")
	command.Flags().StringVarP(&options.Harness, "harness", "k", "", "agent harness (claude, codex, pi, or opencode)")
	command.Flags().StringVarP(&options.Model, "model", "m", "", "model ID (defaults to the harness default)")
	command.Flags().StringVarP(&options.Cwd, "cwd", "C", "", "agent working directory within this Fledge project (defaults to its root)")
	command.Flags().DurationVarP(&options.Timeout, "timeout", "t", lifecycle.DefaultAgentTimeout, "agent startup timeout")
	command.Flags().StringVar(&options.Task, "task", "", "atomically assign an initial task")
	command.Flags().BoolVar(&options.CanDelegate, "can-delegate", false, "allow this agent to delegate child tasks")
	command.Flags().StringVar(&options.ParentTask, "parent-task", "", "parent task authorizing delegated work")
	return command
}
