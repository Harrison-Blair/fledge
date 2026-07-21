package main

import (
	"fmt"
	"strings"
)

const rootHelp = `fledge - agent development harness

usage:
  fledge <command> [args]
  fledge <flag>

commands:
  init       create or refresh the .fledge directory
  deinit     remove the .fledge directory
  start      bring up a flock and open its herdr UI
  stop       stop every flock in the workspace
  context    inspect what fledge sees in a workspace
  flock      bring flocks up and down, inspect them
  agent      register, launch and inspect agents

flags:
  --version, -V   print the fledge version
  --help, -H      print this help
`

var helpPages = map[string]string{
	"": rootHelp,
	"init": `usage:
  fledge init [dir]

create or refresh the .fledge directory without replacing existing files.
dir defaults to the current directory.

also discovers the models the installed integrations can launch (pi
--list-models, codex debug models) and regenerates .fledge/catalog.json
from them. agents.json is the operator's file and wins on name collisions;
claude models have no list command and stay hand-written there.

flags:
  --json, -J   emit a JSON summary instead of text
  --help, -H   print this help
`,
	"deinit": `usage:
  fledge deinit [dir]

list the contents of the .fledge directory, then remove it after an
interactive [y/N] confirmation. dir defaults to the current directory.
refuses while any flock daemon is running.

flags:
  --help, -H   print this help
`,
	"start": `usage:
  fledge start [flags]

bring up a flock, its herdr session and its daemon, then open the herdr UI.
runs anywhere inside the workspace; everything roots at the directory holding
.fledge. starting a flock that is already running reattaches to its session.

a fresh start also spawns the fledge-orchestrator agent and lands you in an
orchestrator | shell pane split. that agent runs under its exact config name,
with no species suffix. with no such config it offers the agent
picker instead, and whatever you pick runs as the orchestrator: under the same
reserved name, listed under the config it came from. if no orchestrator can be
started at all — nothing to pick,
or the pick is cancelled — the start is rolled back rather than attached.
a start whose stdout is not a terminal stops after the daemon: no orchestrator,
no picker, no attach.

flags:
  --flock, -K <name>      flock name (default: the lowest free flockN)
  --session, -N <name>    herdr session name (default: fledge-<dir>-<hash>-<flock>)
  --help, -H              print this help
`,
	"stop": `usage:
  fledge stop

list every flock in the workspace, then stop them all after an interactive
[y/N] confirmation. runs anywhere inside the workspace. each flock is torn
down exactly as fledge flock stop would tear it down.

this command is interactive only: it refuses unless both stdin and stdout are
terminals, and there is no flag to bypass the confirmation. use
fledge flock stop <name> to stop a flock from a script.

flags:
  --help, -H   print this help
`,
	"context": `usage:
  fledge context <command>

commands:
  scan   list the files visible to fledge

flags:
  --help, -H   print this help
`,
	"context scan": `usage:
  fledge context scan [dir] [flags]

list every file in the workspace not excluded by .fledge/.fledgeignore.
the workspace root is found git-style: the nearest ancestor of dir
containing .fledge/. dir defaults to the current directory; a dir below
the root limits the listing to that subtree.

flags:
  --json, -J   emit JSON instead of grouped text
  --help, -H   print this help
`,
	"flock": `usage:
  fledge flock <command>

commands:
  stop [name]    stop a flock (default: the calling flock)
  list           list every flock in the workspace
  status [name]  show one flock (default: the calling flock)

flags:
  --help, -H   print this help
`,
	"flock stop": `usage:
  fledge flock stop [name]

stop a flock by ending its herdr session. name defaults to FLEDGE_FLOCK.
a fledge-managed session (fledge- prefix) is also deleted from herdr's
session list; a session named with --session is only stopped.

flags:
  --help, -H   print this help
`,
	"flock list": `usage:
  fledge flock list

list every flock in the workspace. This command needs no flock context.

flags:
  --help, -H   print this help
`,
	"flock status": `usage:
  fledge flock status [name]

show one flock in detail. name defaults to FLEDGE_FLOCK.

flags:
  --help, -H   print this help
`,
	"agent": `usage:
  fledge agent <command>

commands:
  register <type>  register an already-running agent
  spawn [config]   launch an agent
  stop <name>      stop a spawned agent
  list             list registered agents with liveness
  models           list the configured spawnable models
  msg <command>    send and wait for messages

flags:
  --help, -H   print this help
`,
	"agent register": `usage:
  fledge agent register <type> [flags]

register an already-running agent and print its assigned name.
types are lowercase letters and digits; fledge-orchestrator is the one
reserved name allowed a hyphen, and it registers under that exact name
with no species suffix.

flags:
  --species, -S <slug>  request a specific species slug
  --pid, -P <pid>       liveness pid (default: session leader)
  --help, -H            print this help
`,
	"agent spawn": `usage:
  fledge agent spawn <config> [flags]
  fledge agent spawn --model <id> [flags]
  fledge agent spawn [flags]

launch an agent and print its assigned name. Supply exactly one config name
or model id. Given neither on a terminal, it prints a numbered menu of the
configured agents and spawns the one picked.

flags:
  --model, -M <id>         model id to launch instead of a config name
  --provider, -D <name>    pi provider override
  --integration, -I <name> run the model under this integration
                           (claude, pi or codex; default: routed by prefix)
  --cwd, -C <dir>          working directory (default: workspace root)
  --species, -S <slug>     request a specific species slug
  --help, -H               print this help
`,
	"agent stop": `usage:
  fledge agent stop <name>

stop a spawned agent.

flags:
  --help, -H   print this help
`,
	"agent list": `usage:
  fledge agent list [flags]

list registered agents with their liveness and launch details.

flags:
  --json, -J   emit JSON instead of text
  --help, -H   print this help
`,
	"agent models": `usage:
  fledge agent models [flags]

list spawnable models from .fledge/catalog.json and .fledge/agents.json
(agents.json wins on name collisions), grouped by provider.
This command needs no flock context or running daemon.

flags:
  --json, -J   emit JSON instead of text
  --help, -H   print this help
`,
	"agent msg": `usage:
  fledge agent msg <command>

commands:
  send <to> <body>  send a message and print its id
  wait              wait for a message and print it as JSON

flags:
  --help, -H   print this help
`,
	"agent msg send": `usage:
  fledge agent msg send <to> <body> --from <name> [flags]

send a message and print its id.

flags:
  --from, -F <name>    sender name (required)
  --reply-to, -R <id>  id this message answers
  --help, -H           print this help
`,
	"agent msg wait": `usage:
  fledge agent msg wait --as <name> [flags]

wait for a message and print it as JSON.

flags:
  --as, -A <name>          name to wait as (required)
  --reply-to, -R <id>      wait only for a reply to this id
  --timeout, -T <duration> give up after this duration (default: never)
  --help, -H               print this help
`,
}

// usageError marks a command-line syntax error and carries the nearest useful
// help page. Runtime failures deliberately remain ordinary errors.
type usageError struct {
	message  string
	helpPath string
}

func (e *usageError) Error() string {
	return fmt.Sprintf("%s\n\n%s", e.message, helpPages[e.helpPath])
}

func usageErrorf(helpPath, format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...), helpPath: helpPath}
}

func usageErrorFor(helpPath string, err error) error {
	return &usageError{message: err.Error(), helpPath: helpPath}
}

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-H"
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if isHelpFlag(arg) {
			return true
		}
	}
	return false
}

func printHelp(helpPath string) error {
	text, ok := helpPages[helpPath]
	if !ok {
		return usageErrorf("", "unknown help topic %q", helpPath)
	}
	fmt.Print(text)
	return nil
}

// runHelp resolves an explicit help path. If the path is invalid, the error
// carries the deepest valid page rather than falling all the way back to root.
func runHelp(base string, args []string) error {
	parts := strings.Fields(base)
	parts = append(parts, args...)
	requested := strings.Join(parts, " ")
	if _, ok := helpPages[requested]; ok {
		return printHelp(requested)
	}

	nearest := ""
	for i := len(parts) - 1; i > 0; i-- {
		candidate := strings.Join(parts[:i], " ")
		if _, ok := helpPages[candidate]; ok {
			nearest = candidate
			break
		}
	}
	return usageErrorf(nearest, "unknown help topic %q", requested)
}
