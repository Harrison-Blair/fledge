package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/tui"
	"github.com/spf13/cobra"
)

type sessionManager interface {
	Init(string) (string, error)
	Start(context.Context, string, lifecycle.StartOptions) error
	Spawn(context.Context, string, lifecycle.SpawnOptions) error
	StopAgent(context.Context, string, string) error
	SendMessage(context.Context, string, string, string) (messaging.Message, error)
	ReplyMessage(context.Context, string, string, string) (messaging.Message, error)
	MessageInbox(context.Context, string, string) ([]messaging.Message, error)
	Watch(context.Context, string, lifecycle.WatchOptions) error
	Stop(context.Context, string) error
}

// currentDirectory resolves the invocation directory every command needs.
func currentDirectory(getwd func() (string, error)) (string, error) {
	dir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return dir, nil
}

// Execute runs the root command.
func Execute() error {
	client := herdr.NewClient("herdr", os.Stdin, os.Stdout, os.Stderr)
	confirmer := tui.NewConfirmer(os.Stdin, os.Stdout)
	manager := lifecycle.NewManager(client, confirmer, os.Stdin, os.Stdout)

	return newRootCommand(manager, os.Getwd).Execute()
}

func newRootCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	root := &cobra.Command{
		Use:           "fledge",
		Short:         "Manage project-local Herdr sessions",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newStartCommand(manager, getwd))
	root.AddCommand(newStopCommand(manager, getwd))
	root.AddCommand(newInitCommand(manager, getwd))
	root.AddCommand(newAgentCommand(manager, getwd))
	root.AddCommand(newWatchCommand(manager, getwd))

	return root
}
