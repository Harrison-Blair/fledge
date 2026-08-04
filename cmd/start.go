package cmd

import (
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/spf13/cobra"
)

func newStartCommand(manager sessionManager, getwd func() (string, error)) *cobra.Command {
	var options lifecycle.StartOptions
	command := &cobra.Command{
		Use:   "start [-- native-args...]",
		Short: "Start or attach to this directory's Fledge session",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && cmd.ArgsLenAtDash() < 0 {
				return fmt.Errorf("native agent arguments must follow --")
			}
			dir, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			options.NativeArgs = append([]string(nil), args...)
			options.HarnessSet = cmd.Flags().Changed("harness")
			options.ModelSet = cmd.Flags().Changed("model")
			options.TimeoutSet = cmd.Flags().Changed("timeout")
			if options.Timeout == 0 {
				options.Timeout = lifecycle.DefaultAgentTimeout
			}
			if err := lifecycle.ValidateAgentTimeout(options.Timeout); err != nil {
				return err
			}
			return manager.Start(cmd.Context(), dir, options)
		},
	}
	command.Flags().StringVarP(&options.Harness, "harness", "k", "", "agent harness (claude, codex, pi, or opencode)")
	command.Flags().StringVarP(&options.Model, "model", "m", "", "model ID (defaults to the harness default)")
	command.Flags().DurationVarP(&options.Timeout, "timeout", "t", lifecycle.DefaultAgentTimeout, "agent startup timeout")
	return command
}
