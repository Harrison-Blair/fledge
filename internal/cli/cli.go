// Package cli implements the fledge command-line interface: argument
// dispatch, output formatting (human and --json), and process exit codes.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/bootstrap"
)

// Exit codes shared by all commands.
const (
	ExitOK    = 0 // success
	ExitFail  = 1 // domain failure: check findings, lock held, illegal transition, cycle
	ExitUsage = 2 // usage error
	ExitEnv   = 3 // environment error: not a git repo, no .fledge/ where required
)

type command struct {
	run   func(args []string) int
	usage string
}

// commands is populated by each command file's init().
var commands = map[string]command{}

func register(name string, run func(args []string) int, usage string) {
	commands[name] = command{run, usage}
}

// Run dispatches to a subcommand and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return ExitUsage
	}
	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "fledge: unknown command %q\n\n", name)
		printUsage(os.Stderr)
		return ExitUsage
	}
	warnStampMismatch(name)
	return cmd.run(args[1:])
}

// warnStampMismatch prints a one-line stderr advisory when the scaffold stamp's
// fledgeVersion differs from the running binary. Skipped for init and version
// (which have no need for the warning), and for any error reading the stamp
// (missing stamp, unreadable file) — the warning must never break a command.
func warnStampMismatch(name string) {
	if name == "init" || name == "version" {
		return
	}
	root := findFledgeRoot()
	if root == "" {
		return
	}
	stamp, err := bootstrap.LoadStamp(root)
	if err != nil || stamp == nil {
		return
	}
	if stamp.FledgeVersion != binaryVersion {
		fmt.Fprintf(os.Stderr,
			"fledge: scaffold was written by fledge %s, binary is %s — run 'fledge init --refresh'\n",
			stamp.FledgeVersion, binaryVersion)
	}
}

// findFledgeRoot walks up from the current working directory looking for
// .fledge/scaffold.json. Returns the directory that contains .fledge/, or ""
// when no stamp is found. Uses no git subprocess.
func findFledgeRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".fledge", "scaffold.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, "usage: fledge <command> [args]")
	fmt.Fprintln(w, "\ncommands:")
	for _, name := range commandOrder {
		if cmd, ok := commands[name]; ok {
			fmt.Fprintf(w, "  %s\n", cmd.usage)
		}
	}
}

// commandOrder controls usage listing; keep in sync with registrations.
var commandOrder = []string{
	"init", "agents", "scan", "new", "nest", "preen", "ready", "vee", "colony",
	"unfledged", "status", "set", "criteria", "brood", "abandon", "broods", "version",
}

// emitJSON writes v as indented JSON to stdout.
func emitJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "fledge: %v\n", err)
		return ExitFail
	}
	return ExitOK
}

// fail prints a domain error to stderr and returns ExitFail.
func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "fledge: "+format+"\n", a...)
	return ExitFail
}

// usageErr prints a usage error to stderr and returns ExitUsage.
func usageErr(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "fledge: "+format+"\n", a...)
	return ExitUsage
}

// envErr prints an environment error to stderr and returns ExitEnv.
func envErr(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "fledge: "+format+"\n", a...)
	return ExitEnv
}
