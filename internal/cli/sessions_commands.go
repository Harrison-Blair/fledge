package cli

import (
	"fmt"
	"io"

	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/spf13/cobra"
)

type pruneOptions struct {
	all    bool
	yes    bool
	dryRun bool
}

func newSessions(env *environment) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "Manage saved Herdr sessions",
	}
	cmd.AddCommand(newSessionsPrune(env))
	return cmd
}

func newSessionsPrune(env *environment) *cobra.Command {
	var opts pruneOptions
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete stopped Herdr sessions",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSessionsPrune(cmd, env, opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.all, "all", "a", false, "include every stopped non-default named session")
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "delete without prompting")
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "n", false, "list candidates without deleting")
	return cmd
}

func runSessionsPrune(cmd *cobra.Command, env *environment, opts pruneOptions) error {
	if opts.yes && opts.dryRun {
		return usage("--yes and --dry-run cannot be used together")
	}
	tty := env.stdinTTY != nil && env.stdinTTY()
	if (env.json || !tty) && !opts.yes && !opts.dryRun {
		return usage("--yes or --dry-run is required when prompting is unavailable")
	}
	binary := herdr.Binary{Path: env.herdrBin}
	candidates, err := fledge.PruneCandidates(cmd.Context(), binary, opts.all)
	if err != nil {
		return err
	}
	result := fledge.SessionPruneResult{
		Candidates: candidates,
		Deleted:    []string{},
		DryRun:     opts.dryRun,
	}
	if env.json {
		return printJSONPruneResult(cmd, env, binary, result)
	}
	return runInteractivePrune(cmd, env, binary, result, opts)
}

func printJSONPruneResult(
	cmd *cobra.Command,
	env *environment,
	binary herdr.Binary,
	result fledge.SessionPruneResult,
) error {
	if result.DryRun {
		return env.print(result, func(io.Writer) {})
	}
	var err error
	result, err = fledge.PruneSessions(cmd.Context(), binary, result.Candidates)
	if err != nil {
		return err
	}
	return env.print(result, func(io.Writer) {})
}

func runInteractivePrune(
	cmd *cobra.Command,
	env *environment,
	binary herdr.Binary,
	result fledge.SessionPruneResult,
	opts pruneOptions,
) error {
	printPruneCandidates(env.out, result.Candidates)
	if opts.dryRun {
		fmt.Fprintln(env.out, "Dry run; no sessions deleted.")
		return nil
	}
	if len(result.Candidates) == 0 {
		return nil
	}
	if !opts.yes {
		confirmed, err := confirm(env, "Delete these sessions? [y/N] ")
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(env.out, "Cancelled; no sessions deleted.")
			return nil
		}
	}
	result, err := fledge.PruneSessions(cmd.Context(), binary, result.Candidates)
	for _, name := range result.Deleted {
		fmt.Fprintf(env.out, "Deleted Herdr session %s\n", name)
	}
	return err
}

func printPruneCandidates(w io.Writer, candidates []string) {
	if len(candidates) == 0 {
		fmt.Fprintln(w, "No stopped Herdr sessions are eligible for pruning.")
		return
	}
	fmt.Fprintln(w, "Stopped Herdr sessions eligible for deletion:")
	for _, session := range candidates {
		fmt.Fprintf(w, "  %s\n", session)
	}
}
