package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/tui"
	"github.com/spf13/cobra"
)

type sessionManager interface {
	coordinationManager
	Init(string) (string, error)
	Start(context.Context, string, lifecycle.StartOptions) error
	Spawn(context.Context, string, lifecycle.SpawnOptions) error
	StopAgent(context.Context, string, string) error
	SendMessage(context.Context, string, string, string) (messaging.Message, error)
	ReplyMessage(context.Context, string, string, string) (messaging.Message, error)
	MessageInbox(context.Context, string, string) ([]messaging.Message, string, error)
	Watch(context.Context, string, lifecycle.WatchOptions) error
	Stop(context.Context, string) error
	SetOutput(io.Writer)
}

// commandOutput routes the manager's plain output through the root command's
// writer, so SetOut captures everything Fledge prints rather than only what
// the commands themselves write.
type commandOutput struct{ command *cobra.Command }

func (o commandOutput) Write(contents []byte) (int, error) {
	return o.command.OutOrStdout().Write(contents)
}

// currentDirectory resolves the invocation directory every command needs.
func currentDirectory(getwd func() (string, error)) (string, error) {
	dir, err := getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return dir, nil
}

// runInDir builds a RunE that resolves the invocation directory before
// delegating to fn, collapsing the getwd preamble every command shares.
func runInDir(getwd func() (string, error), fn func(cmd *cobra.Command, args []string, dir string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dir, err := currentDirectory(getwd)
		if err != nil {
			return err
		}
		return fn(cmd, args, dir)
	}
}

// Execute runs the root command with the version main embedded from VERSION.
func Execute(version string) error {
	client := herdr.NewClient("herdr", os.Stdin, os.Stdout, os.Stderr)
	confirmer := tui.NewConfirmer(os.Stdin, os.Stdout)
	manager := lifecycle.NewManager(client, confirmer, os.Stdin, os.Stdout)

	return newRootCommand(manager, os.Getwd, version).Execute()
}

func newRootCommand(manager sessionManager, getwd func() (string, error), version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "fledge",
		Short:         "Manage project-local Herdr sessions",
		Args:          cobra.NoArgs,
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	// Print the bare version so --version and the version subcommand agree.
	root.SetVersionTemplate("{{.Version}}\n")
	manager.SetOutput(commandOutput{command: root})

	root.AddCommand(newStartCommand(manager, getwd))
	root.AddCommand(newStopCommand(manager, getwd))
	root.AddCommand(newInitCommand(manager, getwd))
	root.AddCommand(newAgentCommand(manager, getwd))
	root.AddCommand(newWatchCommand(manager, getwd))
	root.AddCommand(newVersionCommand(version))

	return root
}
