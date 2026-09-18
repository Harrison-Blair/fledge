package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/spf13/cobra"
)

// Execute runs the command tree with the process arguments and streams.
func Execute() error { return execute(os.Args[1:], os.Stdout, os.Stderr) }

// ExecuteWithArgs runs a fresh command tree with supplied arguments and output.
func ExecuteWithArgs(args []string, out io.Writer) error { return execute(args, out, out) }
func execute(args []string, out, errOut io.Writer) error {
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(errOut)
	cmd, err := root.ExecuteC()
	if err == nil {
		return nil
	}
	var outputFailure *agent.OutputError
	if errors.As(err, &outputFailure) {
		return err
	}
	var rendered *agent.ResultError
	if errors.As(err, &rendered) {
		return err
	}
	if cmd != nil && strings.HasPrefix(cmd.CommandPath(), "fledge agent ") {
		operation := "agent." + cmd.Name()
		return agent.Finish(agent.InvalidOutcome(operation, err), out, jsonRequested(cmd, args))
	}
	fmt.Fprintln(errOut, "Error:", err)
	return err
}

// jsonRequested also handles a JSON flag after a syntax error, while skipping
// values of known flags and every native token following the separator.
func jsonRequested(cmd *cobra.Command, args []string) bool {
	selected := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		key, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		flag := cmd.Flags().Lookup(key)
		if flag == nil {
			flag = cmd.InheritedFlags().Lookup(key)
		}
		if flag == nil {
			continue
		}
		if key == "json" {
			if !hasValue {
				selected = true
			} else {
				selected, _ = strconv.ParseBool(value)
			}
		}
		if !hasValue && flag.NoOptDefVal == "" {
			i++
		}
	}
	return selected
}

// ExitCode preserves typed operation status while retaining existing failures.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var status interface{ ExitCode() int }
	if errors.As(err, &status) {
		return status.ExitCode()
	}
	return 1
}
