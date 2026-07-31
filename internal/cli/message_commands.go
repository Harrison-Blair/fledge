package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/spf13/cobra"
)

func newAgentMessage(env *environment) *cobra.Command {
	cmd := &cobra.Command{Use: "message", Short: "Send and audit durable project-scoped messages"}
	cmd.AddCommand(
		newMessageSend(env),
		newMessageReply(env),
		newMessageAck(env),
		newMessageInbox(env),
		newMessageHistory(env),
		newMessageShow(env),
		newMessageRuns(env),
		newMessageRetry(env),
		newMessageCancel(env),
		newMessageDeliver(env),
	)
	return cmd
}

func newMessageDeliver(env *environment) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:    "deliver <agent> <activation-id>",
		Hidden: true,
		Args:   exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeout <= 0 || timeout > 5*time.Minute {
				return usage("--timeout must be greater than zero and no more than 5m")
			}
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			return service.DeliverActivation(cmd.Context(), args[0], args[1], timeout)
		},
	}
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "bounded readiness wait")
	return cmd
}

func newMessageSend(env *environment) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "send <recipient> [text]",
		Short: "Durably send a message",
		Args:  messageSourceArgs("message send", &file),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readMessageInput(env, args[1:], file)
			if err != nil {
				return err
			}
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.SendMessage(cmd.Context(), args[0], body)
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageResult(env, result, "Created")
		},
	}
	cmd.Flags().StringVarP(&file, "file", "F", "", "read message from a file, or - for stdin")
	return cmd
}

func newMessageReply(env *environment) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "reply <message-id> [text]",
		Short: "Reply and atomically acknowledge the received message",
		Args:  messageSourceArgs("message reply", &file),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, err := readMessageInput(env, args[1:], file)
			if err != nil {
				return err
			}
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.ReplyMessage(cmd.Context(), args[0], body)
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageResult(env, result, "Replied with")
		},
	}
	cmd.Flags().StringVarP(&file, "file", "F", "", "read reply from a file, or - for stdin")
	return cmd
}

func messageSourceArgs(label string, file *string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 || len(args) > 2 {
			return usage(label + " requires an ID/name and exactly one message source")
		}
		hasFile := cmd.Flags().Changed("file")
		if hasFile && *file == "" {
			return usage("--file requires a path or -")
		}
		if (len(args) == 2) == hasFile {
			return usage("provide exactly one message source: positional text or --file path|-")
		}
		return nil
	}
}

func readMessageInput(env *environment, bodyArgs []string, file string) (string, error) {
	if len(bodyArgs) == 1 {
		return bodyArgs[0], nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(io.LimitReader(env.in, fledge.MaxMessageBodyBytes+1))
	} else {
		var input *os.File
		input, err = os.Open(file)
		if err == nil {
			defer input.Close()
			data, err = io.ReadAll(io.LimitReader(input, fledge.MaxMessageBodyBytes+1))
		}
	}
	if err != nil {
		return "", fledge.Wrap("message_read_failed", fmt.Sprintf("read message: %v", err), err)
	}
	return string(data), nil
}

func newMessageAck(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "ack <message-id>",
		Short: "Acknowledge receipt of a message",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.AckMessage(cmd.Context(), args[0])
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageResult(env, result, "Acknowledged")
		},
	}
}

func newMessageInbox(env *environment) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "inbox [identity]",
		Short: "Show unresolved inbound messages in the active run",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return usage("message inbox accepts at most one identity")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 0 {
				return usage("--limit must not be negative")
			}
			identity := ""
			if len(args) == 1 {
				identity = args[0]
			}
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.MessageInbox(cmd.Context(), identity, limit)
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageCollection(env, result)
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "maximum messages; 0 is unlimited")
	return cmd
}

func newMessageHistory(env *environment) *cobra.Command {
	var with string
	var runIDs []string
	var allRuns bool
	var status string
	var limit int
	cmd := &cobra.Command{
		Use:   "history <agent>",
		Short: "Show a chronological message transcript",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if allRuns && len(runIDs) > 0 {
				return usage("--run and --all-runs are mutually exclusive")
			}
			if limit < 0 {
				return usage("--limit must not be negative")
			}
			if status != "" && !validMessageStatus(status) {
				return usage(fmt.Sprintf("invalid message status %q", status))
			}
			service, err := env.auditService()
			if err != nil {
				return err
			}
			result, err := service.MessageHistory(fledge.MessageHistoryOptions{
				Agent: args[0], With: with, RunIDs: runIDs, AllRuns: allRuns,
				Status: status, Limit: limit,
			})
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageCollection(env, result)
		},
	}
	cmd.Flags().StringVarP(&with, "with", "w", "", "limit transcript to one counterpart")
	cmd.Flags().StringSliceVarP(&runIDs, "run", "r", nil, "run ID to include; repeat for comparison")
	cmd.Flags().BoolVarP(&allRuns, "all-runs", "a", false, "include every archived and active run")
	cmd.Flags().StringVarP(&status, "status", "s", "", "filter by current message status")
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "maximum newest messages; 0 is unlimited")
	return cmd
}

func newMessageShow(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "show <message-id>",
		Short: "Show one message and its delivery audit",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			service, err := env.auditService()
			if err != nil {
				return err
			}
			message, err := service.ShowMessage(args[0])
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(map[string]any{"message": message}, func(w io.Writer, theme *ui.Theme) {
				printMessageBlock(w, message, theme)
			})
		},
	}
}

func newMessageRuns(env *environment) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List message audit runs",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if limit < 0 {
				return usage("--limit must not be negative")
			}
			service, err := env.auditService()
			if err != nil {
				return err
			}
			result, err := service.MessageRuns(limit)
			if err != nil {
				return fledge.Translate(err)
			}
			return env.print(result, func(w io.Writer, theme *ui.Theme) {
				if len(result.Runs) == 0 {
					fmt.Fprintln(w, "No message runs")
					return
				}
				fmt.Fprintln(w, theme.Accent("RUN\tSTARTED\tSTATE\tMESSAGES"))
				for _, run := range result.Runs {
					state := "closed"
					if run.Active {
						state = "active"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", run.ID, run.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
						theme.Status(state), len(run.Messages))
				}
			})
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "maximum runs; 0 is unlimited")
	return cmd
}

func newMessageRetry(env *environment) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "retry <message-id>",
		Short: "Retry delivery of an unresolved message",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.RetryMessage(cmd.Context(), args[0], force)
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageResult(env, result, "Retried")
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "reinject a message that is awaiting acknowledgement")
	return cmd
}

func newMessageCancel(env *environment) *cobra.Command {
	var reason string
	cmd := &cobra.Command{
		Use:   "cancel <message-id>",
		Short: "Prevent future delivery or replay",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service, err := env.messagingService()
			if err != nil {
				return err
			}
			result, err := service.CancelMessage(cmd.Context(), args[0], reason)
			if err != nil {
				return fledge.Translate(err)
			}
			return printMessageResult(env, result, "Cancelled")
		},
	}
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "record a cancellation reason")
	return cmd
}

func printMessageResult(env *environment, result fledge.MessageResult, action string) error {
	if result.DeliveryError != "" && !env.json {
		fmt.Fprintf(env.errOut, "%s message is durable but delivery was not confirmed: %s\n",
			env.stderrTheme().Warning("Warning:"), result.DeliveryError)
	}
	return env.print(result, func(w io.Writer, theme *ui.Theme) {
		fmt.Fprintf(w, "%s message %s (%s)\n",
			theme.Accent(action), result.Message.ID, theme.Status(result.Message.Status))
	})
}

func printMessageCollection(env *environment, collection messaging.Collection) error {
	return env.print(collection, func(w io.Writer, theme *ui.Theme) {
		if len(collection.Messages) == 0 {
			fmt.Fprintln(w, "No messages")
			return
		}
		for index, message := range collection.Messages {
			if index > 0 {
				fmt.Fprintln(w, strings.Repeat("-", 72))
			}
			printMessageBlock(w, message, theme)
		}
	})
}

func printMessageBlock(w io.Writer, message *messaging.Message, themes ...*ui.Theme) {
	theme := firstTheme(themes)
	fmt.Fprintf(w, "%s  %s → %s  [%s]\n", message.CreatedAt.Format("2006-01-02 15:04:05Z07:00"),
		message.Sender, message.Recipient, theme.Status(message.Status))
	fmt.Fprintf(w, "%s %s  %s %s\n", theme.Accent("ID:"), message.ID, theme.Accent("Run:"), message.RunID)
	if message.ReplyTo != "" {
		fmt.Fprintf(w, "%s %s\n", theme.Accent("Reply to:"), message.ReplyTo)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, message.Body)
	if len(message.DeliveryAttempts) > 0 {
		fmt.Fprintln(w, "\n"+theme.Accent("Delivery:"))
		for _, attempt := range message.DeliveryAttempts {
			fmt.Fprintf(w, "  %s %s", attempt.Timestamp.Format("2006-01-02T15:04:05Z07:00"), theme.Status(attempt.Outcome))
			if attempt.Error != "" {
				fmt.Fprintf(w, ": %s", attempt.Error)
			}
			fmt.Fprintln(w)
		}
	}
}

func validMessageStatus(status string) bool {
	switch status {
	case messaging.StatusQueued, messaging.StatusAwaitingAck, messaging.StatusAcknowledged,
		messaging.StatusFailed, messaging.StatusUncertain, messaging.StatusCancelled:
		return true
	default:
		return false
	}
}
