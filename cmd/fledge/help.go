package main

import (
	"fmt"
	"strings"
)

const rootHelp = `        _,
 _.--""'/
)-._.-\)

usage:
  fledge <command> [args]
  fledge <flag>

commands:
  init       create or refresh the .fledge directory
  deinit     remove the .fledge directory
  start      bring up a flock and open its herdr UI
  restart    restart a flock daemon in place
  stop       stop this flock, or every flock outside one
  watch      stream a flock's daemon log
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
  fledge init [--fresh] [dir]

create or refresh the .fledge directory without replacing existing files.
dir defaults to the current directory.

also discovers native Claude Code default and model-family launchers (claude --version)
and the models available through pi and Codex (pi --list-models, codex debug
models), then regenerates .fledge/agents/fledge/catalog.json and synchronizes portable
Markdown definitions into versioned indexes.

--fresh previews and permanently removes an existing .fledge tree, warns about
untracked files not covered by Git ignore rules, and requires interactive
confirmation on terminals before rebuilding from scratch. it refuses while a
flock is running and cannot be combined with --json.

flags:
  --fresh, -X  destructively rebuild the complete .fledge tree
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

a fresh start offers a profile for the managed, profile-agnostic
fledge-orchestrator definition, opens the UI, then visibly launches the
orchestrator with its identity and Markdown role in the integration's native
instruction channel and readiness as the CLI's initial prompt.
after that succeeds, the right-hand CLI pane is split evenly downward:
fledge watch runs above an interactive project-root shell, with equal-width
columns and focus returned to the orchestrator. the watcher is not registered
as a herdr or fledge agent. watcher setup failure keeps the healthy flock,
restores the prior layout when possible, and prints a manual-watch hint.
if no orchestrator can be started, the start is rolled back.
a start whose stdout is not a terminal stops after the daemon: no orchestrator,
no picker, no attach.

flags:
  --flock, -K <name>      flock name (default: the lowest free flockN)
  --session, -N <name>    herdr session name (default: fledge-<dir>-<hash>-<flock>)
  --help, -H              print this help
`,
	"restart": `usage:
  fledge restart [name]

restart a running flock daemon without ending its herdr session. name defaults
to FLEDGE_FLOCK. the replacement daemon uses the current fledge executable and
the same herdr session binding, including an empty binding when the old daemon
was unbound.

flags:
  --help, -H   print this help
`,
	"stop": `usage:
  fledge stop

when FLEDGE_FLOCK is set, show and offer to stop only that flock. otherwise,
list every flock in the workspace and offer to stop them all. each target is
torn down exactly as fledge flock stop would tear it down.

this command is interactive only: it refuses unless both stdin and stdout are
terminals, and there is no flag to bypass the confirmation. use
fledge flock stop <name> to stop a flock from a script.

flags:
  --help, -H   print this help
`,
	"watch": `usage:
  fledge watch [name]

print a flock's complete daemon.log, then follow appended entries until the
daemon stops or the command is interrupted. name defaults to FLEDGE_FLOCK.

flags:
  --help, -H   print this help
`,
	"context": `usage:
  fledge context <command>

commands:
  scan            list the files visible to fledge
  graph           show their structural directory graph
  compose         inject template instructions into analyzer requests
  validate        validate analyzer request and reply JSON
  render-project  validate and publish a completed context run

flags:
  --help, -H   print this help
`,
	"context compose": `usage:
  fledge context compose <command>

commands:
  analyzer-request  inject template instructions into one analyzer request
  worksheet         stamp one analyzer worksheet from its template

flags:
  --help, -H   print this help
`,
	"context compose analyzer-request": `usage:
  fledge context compose analyzer-request [flags] FILE

inject the operator-editable instruction sections from
.fledge/context/templates/analyzer-request.md into FILE's
instructions_before and instructions_after fields, substituting {group_id},
{purpose}, and {worksheet_path} with the request's own values. The template
sections are delimited by <instructions_before> and <instructions_after> XML
tags, each required exactly once. A template that references
{worksheet_path} requires --worksheet. Recomposing overwrites previously
composed instructions. The composed request prints to standard output unless
--in-place rewrites FILE atomically.

flags:
  --in-place, -A          rewrite FILE with the composed request
  --worksheet, -E <path>  worksheet path substituted for {worksheet_path}
  --help, -H              print this help
`,
	"context compose worksheet": `usage:
  fledge context compose worksheet [flags] REQUEST

stamp the operator-editable worksheet template
.fledge/context/templates/analyzer-worksheet.md for REQUEST's group,
substituting {group_id}, {purpose}, and {files} ({files} renders the
request's assigned files as a Markdown checklist). The worksheet is the
analyzer's scratch pad and human-readable deliverable, retained in the run
folder. It prints to standard output unless --output writes it atomically,
creating parent directories.

flags:
  --output, -U <file>  write the worksheet to file instead of standard output
  --help, -H           print this help
`,
	"context validate": `usage:
  fledge context validate <command>

commands:
  analyzer-request  validate one analyzer request
  analyzer-reply    validate a reply against its request

flags:
  --help, -H   print this help
`,
	"context validate analyzer-request": `usage:
  fledge context validate analyzer-request [FILE|-]

strictly validate one analyzer request JSON object. FILE defaults to "-", which
reads standard input. Unknown, missing, duplicate, unsafe, and inconsistent
fields are rejected.

flags:
  --help, -H   print this help
`,
	"context validate analyzer-reply": `usage:
  fledge context validate analyzer-reply --request FILE [FILE|-]

strictly validate one analyzer reply JSON object and correlate its group and
assigned-file references with the request. Internal dependencies may use safe
normalized project-relative paths outside the request. The reply FILE defaults
to standard input.

flags:
  --request, -Q <file>  analyzer request used for correlation
  --help, -H            print this help
`,
	"context render-project": `usage:
  fledge context render-project RUN_DIR

validate a complete forager/analyzer run, atomically replace
.fledge/context/project.md, publish the run provenance separately as
.fledge/context/provenance.json, and remove the consumed run JSON only after
publication succeeds; a run directory retaining worksheets survives. RUN_DIR
must be canonical and strictly below .fledge/context/runs. success prints
JSON with path, sha256, provenance_path, and warnings; post-publication
durability or cleanup problems are warnings, not rollback errors.

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
  --json, -J   emit strict scan.json JSON with derived counts and total size
  --help, -H   print this help
`,
	"context graph": `usage:
  fledge context graph [dir] [flags]

show the workspace, its visible directories and files as a structural tree.
directory sizes and file counts are recursive. the workspace root is found
and .fledgeignore is applied exactly as for context scan; dir defaults to the
whole workspace and an explicit dir limits the graph to that subtree.

flags:
  --json, -J   emit JSON nodes and contains edges instead of text
  --help, -H   print this help
`,
	"flock": `usage:
  fledge flock <command>

commands:
  clear [name]   permanently remove saved flock state
  stop [name]    stop a flock (default: the calling flock)
  list           list every flock in the workspace
  status [name]  show one flock (default: the calling flock)

flags:
  --help, -H   print this help
`,
	"flock clear": `usage:
  fledge flock clear [name]

permanently remove saved state for one stopped flock, or every flock in the
workspace when no name is given. the bare form does not use FLEDGE_FLOCK.
running flocks are skipped; stop them separately with fledge flock stop first.

the command previews each target and its running/down status, then asks once
with a default-No [y/N] confirmation. it requires terminals on both stdin and
stdout, and has no non-interactive bypass. clearing removes the flock's
journal, daemon log, RPC remnants, and roster history.
the corresponding default fledge-managed herdr session is stopped and deleted
first. after confirmation, project-scoped managed herdr sessions with no saved
flock directory are also removed. operator-named --session records and managed
sessions belonging to other projects are untouched.

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
  register <type|agent.md>  register an already-running agent
  spawn [agent]    launch an agent definition or raw profile/model
  ready            authenticate a Fledge-started agent
  stop <name>      stop a spawned agent
  list             list registered agents with liveness
  types            list configured agent definitions
  models           list the configured spawnable models
  msg <command>    send and wait for messages

flags:
  --help, -H   print this help
`,
	"agent register": `usage:
  fledge agent register <type|agent.md> [flags]

register an already-running agent and print its assigned name. An .agent.md
path attaches its configured agent/profile/source metadata. A bare type is an
ad hoc registration; the fledge-* namespace is reserved for managed agents.
Cooperative agents can establish their shell identity with:
  export FLEDGE_AGENT_NAME="$(fledge agent register <type>)"

flags:
  --species, -S <slug>  request a specific species slug
  --pid, -P <pid>       liveness pid (default: session leader)
  --help, -H            print this help
`,
	"agent spawn": `usage:
  fledge agent spawn <agent> [flags]
  fledge agent spawn --profile <name> [flags]
  fledge agent spawn --model <id> [flags]
  fledge agent spawn [flags]

launch an agent and print its assigned name after authenticated readiness.
Fledge installs an agent's exact Markdown role plus its assigned identity and
message guidance through the integration's native instruction option. A raw
profile has identity and messaging guidance but no authored role; it may also
supply the profile for a profile-agnostic agent. Readiness is the CLI's initial
positional prompt; startup does not send lifecycle text through the pane.
Given no selection on a terminal, a numbered agent menu is shown.

Definitions may request fledge.workspace label/tab metadata. Fledge then
creates that unfocused workspace in the flock's existing Herdr session,
renames its initial tab, starts a pane-hosted Claude/Codex agent there, and
removes the initial shell. Pi profiles cannot satisfy dedicated placement.
Any launch, spawn-journal, or readiness failure closes the whole new workspace.

The managed fledge-forager uses its pinned Claude profile in a fledge-context workspace.
After spawn returns, send it an explicit context task, save the message id, and
wait with "agent msg wait --reply-to <id>". It partitions one context scan,
spawns file-scoped fledge-analyzer agents into --workspace/--tab targets,
strictly validates their correlated replies, and publishes the atomic
.fledge/context/project.md artifact with "context render-project".

flags:
  --profile, -L <name>     raw profile, or profile for an agnostic agent
  --model, -M <id>         raw model id to route and launch
  --provider, -D <name>    pi provider override
  --integration, -I <name> run the model under this integration
                           (claude, pi or codex; default: routed by prefix)
  --cwd, -C <dir>          working directory (default: workspace root)
  --workspace, -W <value>  existing Herdr workspace id or exact label
  --tab, -B <value>        existing tab id/label, or label to create
                           (--workspace and --tab must be used together)
  --species, -S <slug>     request a specific species slug
  --timeout, -T <duration> readiness timeout (default: 2m)
  --help, -H               print this help
`,
	"agent ready": `usage:
  fledge agent ready [flags]

authenticate the one-use readiness token injected by fledge agent spawn.
By default, readiness is journaled and then this command waits for the first
mailbox message, printing the same JSON object as agent msg wait. --no-wait
preserves the startup handshake behavior and prints the assigned name.
This command takes no arguments and only works inside a starting agent.

flags:
  --no-wait, -O  authenticate readiness and return without waiting
  --help, -H     print this help
`,
	"agent stop": `usage:
  fledge agent stop <name>

stop a spawned agent. A dedicated agent closes its whole workspace; an
ordinary pane-hosted agent closes only its pane.

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

list resolved launch profiles from the user, managed, and discovered indexes,
grouped by provider. Conflicting declarations are errors.
This command needs no flock context or running daemon.

flags:
  --json, -J   emit JSON instead of text
  --help, -H   print this help
`,
	"agent types": `usage:
  fledge agent types [flags]

list configured Markdown agent definitions. This command needs no flock
context or running daemon.

flags:
  --json, -J   emit JSON instead of text
  --help, -H   print this help
`,
	"agent msg": `usage:
  fledge agent msg <command>

commands:
  send <to> [body]  send a message and print its id
  reply <id> [body] reply to a claimed inbound message
  inbox             claim the oldest available message without waiting
  wait              wait for a message and print it as JSON

flags:
  --help, -H   print this help
`,
	"agent msg send": `usage:
  fledge agent msg send <to> [body] [flags]

send a message and print its id. The sender is FLEDGE_AGENT_NAME. A spawned
recipient receives it through its durable mailbox after running agent ready.
Startup instructions are supplied at launch and are not mailbox messages.
Supply exactly one positional body or --body-file; "-" reads the body unchanged
from standard input.

flags:
  --body-file, -F <file|->  read the body from a file or standard input
  --reply-to, -R <id>       id this message answers
  --help, -H                print this help
`,
	"agent msg reply": `usage:
  fledge agent msg reply <message-id> [body] [flags]

reply to a claimed inbound message and print the new message id. Fledge
validates that message-id was delivered to FLEDGE_AGENT_NAME, derives its
original sender as the recipient, and sets reply_to to that exact message id.
Supply exactly one positional body or --body-file.

flags:
  --body-file, -F <file|->  read the body from a file or standard input
  --help, -H                print this help
`,
	"agent msg inbox": `usage:
  fledge agent msg inbox [flags]

claim the oldest available message and print it as JSON, or print null when the
inbox has no match. Delivery is acknowledged only after the JSON is written
successfully, so an interrupted output remains available to retry. The
recipient is FLEDGE_AGENT_NAME. When both --from and --reply-to are present,
both constraints must match.

flags:
  --from <agent>       claim only a message from this exact agent
  --reply-to, -R <id>  claim only a reply to this id
  --help, -H           print this help
`,
	"agent msg wait": `usage:
  fledge agent msg wait [flags]

wait for a message and print it as JSON. The recipient is FLEDGE_AGENT_NAME.
Delivery is acknowledged only after the JSON is written successfully, so an
interrupted output remains available to retry. When both --from and --reply-to
are present, both constraints must match.
flags:
  --from <agent>           wait only for a message from this exact agent
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
