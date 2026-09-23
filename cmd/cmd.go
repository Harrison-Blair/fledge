package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/spf13/cobra"
)

// Execute runs the command tree with the process arguments and streams.
func Execute() error { return execute(os.Args[1:], nil, os.Stdout, os.Stderr) }

// ExecuteWithArgs runs a fresh command tree with supplied arguments and output.
func ExecuteWithArgs(args []string, out io.Writer) error { return execute(args, nil, out, out) }

func execute(args []string, in io.Reader, out, errOut io.Writer) error {
	root := NewRootCmd()
	root.SetArgs(args)
	if in != nil {
		root.SetIn(in)
	}
	root.SetOut(out)
	root.SetErr(errOut)
	cmd, err := root.ExecuteC()
	if err == nil {
		return nil
	}
	if cli.IsRendered(err) {
		return err
	}
	for _, group := range []string{"agent", "worktree", "task"} {
		if cmd != nil && strings.HasPrefix(cmd.CommandPath(), "fledge "+group+" ") {
			operation := group + "." + cmd.Name()
			return agent.Finish(agent.InvalidOutcome(operation, err), out, jsonRequested(cmd, args), nil)
		}
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
