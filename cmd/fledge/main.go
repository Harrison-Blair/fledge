// Command fledge is the fledge CLI.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/catalog"
	"github.com/Harrison-Blair/fledge/internal/client"
	"github.com/Harrison-Blair/fledge/internal/contextdoc"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/scan"
	"github.com/Harrison-Blair/fledge/internal/version"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fledge:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return printHelp("")
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "deinit":
		return runDeinit(args[1:])
	case "start":
		return runStart(args[1:])
	case "restart":
		return runRestart(args[1:])
	case "stop":
		return runStop(args[1:])
	case "watch":
		return runWatch(args[1:])
	case "context":
		return runContext(args[1:])
	case "flock":
		return runFlock(args[1:])
	case "agent":
		return runAgent(args[1:])
	case "daemon":
		return runDaemon(args[1:])
	case "-V", "--version", "version":
		fmt.Printf("fledge %s\n", version.Get())
		return nil
	case "-H", "--help":
		return printHelp("")
	case "help":
		return runHelp("", args[1:])
	default:
		return usageErrorf("", "unknown command %q", args[0])
	}
}

func runContext(args []string) error {
	if len(args) == 0 {
		return printHelp("context")
	}
	if isHelpFlag(args[0]) {
		return printHelp("context")
	}
	if args[0] == "help" {
		return runHelp("context", args[1:])
	}
	switch args[0] {
	case "scan":
		return runContextScan(args[1:])
	case "graph":
		return runContextGraph(args[1:])
	case "compose":
		return runContextCompose(args[1:])
	case "validate":
		return runContextValidate(args[1:])
	case "render-project":
		return runContextRenderProject(args[1:])
	default:
		return usageErrorf("context", "unknown context subcommand %q", args[0])
	}
}

func runContextValidate(args []string) error {
	if len(args) == 0 {
		return printHelp("context validate")
	}
	if isHelpFlag(args[0]) {
		return printHelp("context validate")
	}
	if args[0] == "help" {
		return runHelp("context validate", args[1:])
	}
	switch args[0] {
	case "analyzer-request":
		return runContextValidateAnalyzerRequest(args[1:])
	case "analyzer-reply":
		return runContextValidateAnalyzerReply(args[1:])
	default:
		return usageErrorf("context validate", "unknown context validate subcommand %q", args[0])
	}
}

func runContextValidateAnalyzerRequest(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context validate analyzer-request")
	}
	if err := rejectFlags("context validate analyzer-request", args); err != nil {
		return usageErrorFor("context validate analyzer-request", err)
	}
	if len(args) > 1 {
		return usageErrorf("context validate analyzer-request",
			"context validate analyzer-request: unexpected argument %q", args[1])
	}
	name := "-"
	if len(args) == 1 {
		name = args[0]
	}
	input, cleanup, err := inputPath(name)
	if err != nil {
		return fmt.Errorf("context validate analyzer-request: %w", err)
	}
	defer cleanup()
	return contextdoc.ValidateAnalyzerRequestFile(input)
}

func runContextValidateAnalyzerReply(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context validate analyzer-reply")
	}
	requestPath, args, err := takeFlag(args, "--request", "-Q")
	if err != nil {
		return usageErrorFor("context validate analyzer-reply", err)
	}
	if err := rejectFlags("context validate analyzer-reply", args); err != nil {
		return usageErrorFor("context validate analyzer-reply", err)
	}
	if requestPath == "" {
		return usageErrorf("context validate analyzer-reply",
			"context validate analyzer-reply: --request is required")
	}
	if requestPath == "-" {
		return usageErrorf("context validate analyzer-reply",
			"context validate analyzer-reply: --request requires a file path")
	}
	if len(args) > 1 {
		return usageErrorf("context validate analyzer-reply",
			"context validate analyzer-reply: unexpected argument %q", args[1])
	}
	replyName := "-"
	if len(args) == 1 {
		replyName = args[0]
	}
	replyPath, cleanup, err := inputPath(replyName)
	if err != nil {
		return fmt.Errorf("context validate analyzer-reply: %w", err)
	}
	defer cleanup()
	// The contextdoc contract takes reply first and its correlating request
	// second. Keep that order explicit at this CLI boundary.
	return contextdoc.ValidateAnalyzerReplyFile(replyPath, requestPath)
}

func runContextRenderProject(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context render-project")
	}
	if err := rejectFlags("context render-project", args); err != nil {
		return usageErrorFor("context render-project", err)
	}
	if len(args) != 1 {
		return usageErrorf("context render-project",
			"context render-project: want exactly one run directory")
	}
	result, err := contextdoc.RenderProject(args[0])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// inputPath adapts stdin to validators whose strict public contract is a file
// path. Named inputs are returned untouched; "-" is copied to a private
// temporary file that is removed before the command returns.
func inputPath(name string) (string, func(), error) {
	if name != "-" {
		return name, func() {}, nil
	}
	file, err := os.CreateTemp("", "fledge-context-*.json")
	if err != nil {
		return "", func() {}, err
	}
	tempPath := file.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if _, err := io.Copy(file, os.Stdin); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("read stdin: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tempPath, cleanup, nil
}

func runContextScan(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context scan")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	if err := rejectFlags("context scan", args); err != nil {
		return usageErrorFor("context scan", err)
	}
	if len(args) > 1 {
		return usageErrorf("context scan", "context scan: unexpected argument %q", args[1])
	}

	start := "."
	if len(args) == 1 {
		start = args[0]
	}

	context, err := scanContext(start, len(args) == 1)
	if err != nil {
		return err
	}

	if asJSON {
		totalSize := int64(0)
		for _, file := range context.Files {
			if file.Size < 0 || file.Size > math.MaxInt64-totalSize {
				return fmt.Errorf("context scan: file sizes overflow total_size")
			}
			totalSize += file.Size
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			SchemaVersion int         `json:"schema_version"`
			Root          string      `json:"root"`
			FileCount     int         `json:"file_count"`
			TotalSize     int64       `json:"total_size"`
			Files         []scan.File `json:"files"`
		}{contextdoc.SchemaVersion, context.Root, len(context.Files), totalSize, context.Files})
	}

	printGrouped(context.Files)
	return nil
}

// printGrouped lists files under a heading per directory, root files first,
// with sizes aligned within each group.
func printGrouped(files []scan.File) {
	byDir := make(map[string][]scan.File)
	for _, f := range files {
		dir := path.Dir(f.Path)
		byDir[dir] = append(byDir[dir], f)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for i, dir := range dirs {
		if i > 0 {
			fmt.Println()
		}
		// Root files stand alone; only subdirectories get a heading to sit under.
		indent := ""
		if dir != "." {
			fmt.Println(dir + "/")
			indent = "  "
		}

		widest := 0
		for _, f := range byDir[dir] {
			if n := len(path.Base(f.Path)); n > widest {
				widest = n
			}
		}
		for _, f := range byDir[dir] {
			fmt.Printf("%s%-*s  %6s\n", indent, widest, path.Base(f.Path), humanSize(f.Size))
		}
	}
}

// humanSize renders a byte count in the shortest unit that keeps it under
// 1024, carrying one decimal only while the number is small enough to need it.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}

	value := float64(n)
	var suffix byte
	for _, s := range []byte("KMGTPE") {
		value /= unit
		suffix = s
		if value < unit {
			break
		}
	}
	if value < 10 {
		return fmt.Sprintf("%.1f%c", value, suffix)
	}
	return fmt.Sprintf("%.0f%c", value, suffix)
}

// takeFlag pulls a flag and its value out of args, returning the value and the
// remaining arguments. A flag with no value following it is an error. A
// following token that is itself flag-shaped (begins with "-") counts as a
// missing value, so it survives into the later rejectFlags sweep instead of
// being silently swallowed. The exact token "-" is the conventional stdin
// value accepted by file-input flags.
func takeFlag(args []string, long, short string) (value string, rest []string, err error) {
	for i, arg := range args {
		if arg != long && arg != short {
			rest = append(rest, arg)
			continue
		}
		if i+1 >= len(args) || (args[i+1] != "-" && strings.HasPrefix(args[i+1], "-")) {
			return "", nil, fmt.Errorf("%s: missing value", long)
		}
		return args[i+1], append(rest, args[i+2:]...), nil
	}
	return "", rest, nil
}

// rejectFlags reports the first leftover argument that looks like a flag.
func rejectFlags(cmd string, args []string) error {
	for _, arg := range args {
		if len(arg) > 1 && arg[0] == '-' {
			return fmt.Errorf("%s: unknown flag %q", cmd, arg)
		}
	}
	return nil
}

func runFlock(args []string) error {
	if len(args) == 0 {
		return printHelp("flock")
	}
	if isHelpFlag(args[0]) {
		return printHelp("flock")
	}
	if args[0] == "help" {
		return runHelp("flock", args[1:])
	}
	switch args[0] {
	case "clear":
		return runFlockClear(args[1:])
	case "stop":
		return runFlockStop(args[1:])
	case "list":
		return runFlockList(args[1:])
	case "status":
		return runFlockStatus(args[1:])
	default:
		return usageErrorf("flock", "unknown flock subcommand %q", args[0])
	}
}

// runFlockClear permanently forgets saved state for one flock, or for every
// flock when no name is given. Unlike other optionally named flock commands,
// the bare form is deliberately workspace-wide and never consults
// FLEDGE_FLOCK.
func runFlockClear(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("flock clear")
	}
	if err := rejectFlags("flock clear", args); err != nil {
		return usageErrorFor("flock clear", err)
	}
	if len(args) > 1 {
		return usageErrorf("flock clear", "flock clear: unexpected argument %q", args[1])
	}
	if len(args) == 1 {
		if err := flock.Validate(args[0]); err != nil {
			return usageErrorFor("flock clear", err)
		}
	}
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return errors.New("fledge flock clear needs a terminal on stdin and stdout")
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	orphanSessions, err := clearFlockOrphans(root)
	if err != nil {
		return err
	}
	names, err := flock.List(root)
	if err != nil {
		return err
	}
	if len(args) == 1 {
		requested := args[0]
		matched := names[:0]
		for _, name := range names {
			if name == requested {
				matched = append(matched, name)
			}
		}
		names = matched
		if len(names) == 0 {
			fmt.Printf("flock %s: no saved state\n", requested)
			if len(orphanSessions) == 0 {
				return nil
			}
		}
	}
	if len(names) == 0 && len(orphanSessions) == 0 {
		fmt.Println("no saved flock state")
		return nil
	}

	if len(names) > 0 {
		printClearOverview(root, names, clearFlockRunning)
	}
	for _, session := range orphanSessions {
		fmt.Printf("%s  orphan managed herdr session\n", session)
	}
	fmt.Print("\npermanently clear the flock state and managed herdr sessions above? [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Println("aborted; nothing cleared")
		return nil
	}

	var clearErrors []error
	if len(names) > 0 {
		if err := clearFlocks(root, names, clearFlockRunning, clearFlockSession, clearFlockRemoveAll); err != nil {
			clearErrors = append(clearErrors, err)
		}
	}
	if len(orphanSessions) > 0 {
		if err := clearOrphanSessions(orphanSessions, clearOrphanSession); err != nil {
			clearErrors = append(clearErrors, err)
		}
	}
	return errors.Join(clearErrors...)
}

// These seams make the liveness race and partial filesystem failure testable
// without depending on timing or host permissions.
var (
	clearFlockRunning   = client.Running
	clearFlockSession   = clearManagedSession
	clearFlockRemoveAll = os.RemoveAll
	clearFlockOrphans   = managedOrphanSessions
	clearOrphanSession  = herdr.Remove
)

func clearManagedSession(root, name string) error {
	return herdr.Remove(flock.SessionName(root, name))
}

func managedOrphanSessions(root string) ([]string, error) {
	sessions, err := herdr.List()
	if err != nil {
		return nil, err
	}
	names, err := flock.List(root)
	if err != nil {
		return nil, err
	}
	linked := make(map[string]struct{}, len(names))
	for _, name := range names {
		linked[flock.SessionName(root, name)] = struct{}{}
	}
	prefix := flock.SessionPrefix(root)
	var orphans []string
	for _, session := range sessions {
		if !strings.HasPrefix(session.Name, prefix) {
			continue
		}
		if _, ok := linked[session.Name]; !ok {
			orphans = append(orphans, session.Name)
		}
	}
	sort.Strings(orphans)
	return orphans, nil
}

func clearOrphanSessions(names []string, remove func(name string) error) error {
	failed := 0
	for _, name := range names {
		if err := remove(name); err != nil {
			fmt.Printf("herdr session %s: failed to clear: %v\n", name, err)
			failed++
			continue
		}
		fmt.Printf("herdr session %s: cleared\n", name)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d orphan managed herdr sessions were not cleared", failed, len(names))
	}
	return nil
}

func printClearOverview(root string, names []string, running func(root, name string) bool) {
	widest := 0
	for _, name := range names {
		if len(name) > widest {
			widest = len(name)
		}
	}
	for _, name := range names {
		status := "down"
		if running(root, name) {
			status = "running"
		}
		fmt.Printf("%-*s  %s\n", widest, name, status)
	}
}

// clearFlocks rechecks each daemon immediately before touching that flock's
// directory. The deterministic managed Herdr session is removed first so a
// stale pane identity cannot survive cleared state. A running flock or cleanup
// failure does not strand later targets, but either makes the command fail.
func clearFlocks(
	root string,
	names []string,
	running func(root, name string) bool,
	removeSession func(root, name string) error,
	removeAll func(string) error,
) error {
	notCleared := 0
	for _, name := range names {
		if running(root, name) {
			fmt.Printf("flock %s: skipped (daemon running; run fledge flock stop %s first)\n", name, name)
			notCleared++
			continue
		}
		if err := removeSession(root, name); err != nil {
			fmt.Printf("flock %s: failed to clear managed herdr session: %v\n", name, err)
			notCleared++
			continue
		}
		if err := removeAll(flock.Dir(root, name)); err != nil {
			fmt.Printf("flock %s: failed to clear: %v\n", name, err)
			notCleared++
			continue
		}
		fmt.Printf("flock %s: cleared\n", name)
	}
	if notCleared > 0 {
		return fmt.Errorf("%d of %d flocks were not cleared", notCleared, len(names))
	}
	return nil
}

// runStart brings up a flock: its own herdr session and its own daemon.
// Teardown is flock stop's business; the flock's daemon follows its session
// out.
func runStart(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("start")
	}
	name, args, err := takeFlag(args, "--flock", "-K")
	if err != nil {
		return usageErrorFor("start", err)
	}
	session, args, err := takeFlag(args, "--session", "-N")
	if err != nil {
		return usageErrorFor("start", err)
	}
	if err := rejectFlags("start", args); err != nil {
		return usageErrorFor("start", err)
	}
	if len(args) != 0 {
		return usageErrorf("start", "start: unexpected argument %q", args[0])
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	if err := agentcfg.Synchronize(root); err != nil {
		return err
	}

	if name == "" {
		if name, err = flock.Mint(root); err != nil {
			return err
		}
	} else if err := flock.Validate(name); err != nil {
		return usageErrorFor("start", err)
	}
	if err := daemon.CheckSocketPath(root, name); err != nil {
		return err
	}
	if client.Running(root, name) {
		// The flock is already up; starting it again just returns to it.
		resp, err := client.Do(root, name, protocol.Request{Op: protocol.OpStatus})
		if err != nil {
			return err
		}
		if resp.Session == "" {
			return fmt.Errorf("flock %s is already running without a herdr session; stop it by hand", name)
		}
		if session != "" && session != resp.Session {
			return fmt.Errorf("flock %s is already running in herdr session %s", name, resp.Session)
		}
		fmt.Printf("flock:  %s\n", name)
		fmt.Printf("herdr:  %s (%s)\n", resp.Session, resp.SessionSocket)
		fmt.Println("daemon: up")
		if !stdoutIsTerminal() {
			return nil
		}
		return attachHerdr(resp.Session, root, nil)
	}

	// A flock's session is named after its workspace and itself unless the
	// operator points it at another one, so two flocks never share a session
	// by accident — not even the same-named flock of another workspace. The
	// fledge- prefix is what marks it as managed in herdr's session list.
	managedSession := session == ""
	if managedSession {
		session = flock.SessionName(root, name)
	}

	var s herdr.Session
	var started bool
	if managedSession {
		// The daemon is known to be down, so any surviving default session is
		// stale. It may still contain an orchestrator pane whose Herdr identity
		// a new daemon cannot adopt; recreate the managed session before launch.
		s, err = herdr.Recreate(session, []string{flock.Env + "=" + name}, root)
		started = err == nil
	} else {
		// Explicit sessions belong to the operator and retain the established
		// reuse behavior, including their existing environment and panes.
		s, started, err = herdr.Ensure(session, []string{flock.Env + "=" + name}, root)
	}
	if err != nil {
		return err
	}

	// started is true only when this start created (or recreated) the session,
	// so guardedBringUp tears it down on failure while a reused operator session
	// is left running. Every pre-daemon failure below routes through that guard
	// so a start that exits 1 never strands a herdr server.
	var steps []func() error
	if started {
		steps = append(steps, func() error {
			// A fresh session has no workspace until a client attaches, and the
			// one herdr then makes lands in $HOME, not here. Creating it now,
			// rooted at the workspace, is what makes attaching open the project.
			created, err := herdrwire.WorkspaceCreate(s.SocketPath, root, workspaceLabel, true)
			if err != nil {
				return fmt.Errorf("create workspace in session %s: %w", s.Name, err)
			}
			// Labelling is workspace metadata, not part of the interactive
			// orchestrator flow, so it happens here for scripted starts too. The
			// tab herdr opens with the workspace already exists and its id came
			// back above, so this needs no lookup.
			if err := herdrwire.TabRename(s.SocketPath, created.TabID, tabLabel); err != nil {
				return fmt.Errorf("label tab in session %s: %w", s.Name, err)
			}
			return nil
		})
	}
	steps = append(steps, func() error { return spawnDaemon(root, name, s.Name) })
	if err := guardedBringUp(s.Name, started, stopHerdrSession, steps...); err != nil {
		return err
	}

	fmt.Printf("flock:  %s\n", name)
	fmt.Printf("herdr:  %s (%s)\n", s.Name, s.SocketPath)
	fmt.Println("daemon: up")
	if !started {
		// Panes inherit the session server's environment, so a session that
		// was already up cannot be told about this flock after the fact.
		fmt.Printf("\nsession %s was already running, so its panes do not carry %s.\n", s.Name, flock.Env)
		fmt.Printf("export it in each pane before running fledge:\n\n    export %s=%s\n", flock.Env, name)
	}
	// A scripted start stops at server-only bring-up: no orchestrator, no
	// menu, nothing to attach to.
	if !stdoutIsTerminal() {
		return nil
	}

	// The pane the orchestrator will split off, captured before the spawn
	// because that is when the shell pane is still the focused one.
	shellPane, err := herdrwire.PaneCurrent(s.SocketPath)
	if err != nil {
		return abortStart(root, name, err)
	}

	req, err := managedOrchestratorRequest(root)
	if err != nil {
		return abortStart(root, name, err)
	}
	req.AnchorPane = shellPane

	// Herdr must own the terminal before the authenticated launch begins. That
	// lets the operator watch the orchestrator pane appear, receive bootstrap,
	// and transition to running instead of staring at a blocked `fledge start`
	// while all of that happens in an invisible server-side session.
	spawned := make(chan error, 1)
	attachErr := attachHerdr(s.Name, root, func() {
		startAfterAttach(func() {
			resp, spawnErr := client.Do(root, name, req)
			// Report the outcome the instant it is known — before the non-critical
			// watcher path — so a hang or panic while installing the watcher can
			// never block awaitSpawn. The channel is buffered, so this never waits.
			spawned <- spawnErr
			if spawnErr != nil {
				// The UI is already visible, so ending the session is how an
				// asynchronous launch failure preserves start's rollback rule.
				_ = stopFlock(root, name)
				return
			}
			self, executableErr := os.Executable()
			if executableErr != nil {
				warnWatcherFailure(s.SocketPath, shellPane, name,
					fmt.Errorf("locate fledge executable: %w", executableErr))
				_ = herdrwire.PaneFocus(s.SocketPath, resp.PaneID)
				return
			}
			// Monitoring is intentionally non-critical. The orchestrator is
			// already authenticated and healthy, so layout failure preserves
			// the flock and leaves a manual recovery hint in the CLI pane.
			_ = installWatcherPane(s.SocketPath, root, name, self, shellPane, resp.PaneID)
		})
	})
	return awaitSpawn(root, name, attachErr, spawned, abortStart)
}

// awaitSpawn resolves an interactive start's outcome once its Herdr attach has
// returned. attachHerdr fires its attached callback — which launches the spawn
// goroutine — before it calls cmd.Wait, so attachErr carries a narrow invariant:
//
//	nil     ⇒ the attach handed off; the goroutine is running and is guaranteed
//	          to report exactly one outcome on spawned.
//	non-nil ⇒ ambiguous: a wait error can arrive after a clean handoff (goroutine
//	          launched, will report) or before one (LookPath/Start failed, so the
//	          goroutine never launched and nothing will ever land on spawned).
//
// A buffered spawn outcome takes precedence over an attach error: a goroutine
// that already reported a failure has also already rolled the flock back, so
// that failure is the real cause and abort must not tear the flock down twice.
func awaitSpawn(root, name string, attachErr error, spawned <-chan error, abort func(root, name string, cause error) error) error {
	if attachErr == nil {
		// Handoff is guaranteed, so block for the one outcome the goroutine owes
		// rather than drop a spawn failure that lands just after attach returns.
		// The wait is bounded by the daemon's readiness timeout, not the CLI, so
		// announce it — otherwise the terminal reads as hung.
		fmt.Println("waiting for the orchestrator to launch...")
		if spawnErr := <-spawned; spawnErr != nil {
			return fmt.Errorf("%w; flock %s rolled back", spawnErr, name)
		}
		return nil
	}
	// Ambiguous attach error: read an already-reported outcome without blocking,
	// since the goroutine may never have launched and a blocking read would then
	// deadlock. A reported spawn failure wins; otherwise the attach error stands.
	select {
	case spawnErr := <-spawned:
		if spawnErr != nil {
			return fmt.Errorf("%w; flock %s rolled back", spawnErr, name)
		}
	default:
	}
	return abort(root, name, attachErr)
}

func managedOrchestratorRequest(root string) (protocol.Request, error) {
	defs, profiles, err := agentcfg.LoadDefinitions(root)
	if err != nil {
		return protocol.Request{}, err
	}
	d, ok := defs[agentcfg.ReservedOrchestrator]
	if !ok {
		return protocol.Request{}, fmt.Errorf("managed %q definition is missing", agentcfg.ReservedOrchestrator)
	}
	profile := d.Profile
	if profile == "" {
		if !stdinIsTerminal() {
			return protocol.Request{}, errors.New("start needs a terminal to choose the orchestrator profile")
		}
		if len(profiles) == 0 {
			return protocol.Request{}, errors.New("no profiles available for the fledge-orchestrator")
		}
		profile, err = pickOrchestratorConfig(profiles, os.Stdin, os.Stdout)
		if err != nil {
			return protocol.Request{}, err
		}
	}
	return protocol.Request{
		Op: protocol.OpSpawn, Agent: d.Name, Profile: profile, Split: "right",
	}, nil
}

// Labels for the herdr workspace and tab a fresh start opens, so the session
// reads as fledge's rather than as an unnamed shell.
const (
	workspaceLabel = "fledge-orchestrator"
	tabLabel       = "orchestrator"
)

// guardedBringUp runs a start's post-session bring-up steps in order. If any
// step fails and this start created the session (created), the session is torn
// down through teardown so a half-finished start never strands a herdr server
// the operator would have to find and stop by hand. A reused operator session
// (created == false) is left running.
func guardedBringUp(session string, created bool, teardown func(string) error, steps ...func() error) error {
	for _, step := range steps {
		if err := step(); err != nil {
			if created {
				if tdErr := teardown(session); tdErr != nil {
					return fmt.Errorf("%w (tearing herdr session %s down failed too: %v)", err, session, tdErr)
				}
			}
			return err
		}
	}
	return nil
}

// stopHerdrSession ends a herdr session server directly, without the daemon.
// It is the pre-daemon counterpart to stopFlock: a start that created a session
// but never got a daemon up rolls it back through here. A variable so tests can
// observe the rollback without a real herdr.
var stopHerdrSession = func(session string) error {
	return herdr.Stop(session)
}

// abortStart rolls a half-finished start back. An interactive start exists to
// land the operator in a running orchestrator, so a flock that cannot get one
// is torn down rather than left up for them to find and clean out by hand.
func abortStart(root, name string, cause error) error {
	if err := stopFlock(root, name); err != nil {
		return fmt.Errorf("%w (rolling the start back failed too: %v)", cause, err)
	}
	return fmt.Errorf("%w; flock %s rolled back", cause, name)
}

// takeBoolFlag pulls a valueless flag out of args, reporting whether it was
// present.
func takeBoolFlag(args []string, long, short string) (found bool, rest []string) {
	for _, arg := range args {
		if arg == long || arg == short {
			found = true
			continue
		}
		rest = append(rest, arg)
	}
	return found, rest
}

// stdoutIsTerminal is a variable for the same reason stdinIsTerminal is: tests
// capture stdout through a pipe, which is never a char device, so the
// interactive path is unreachable without stubbing it.
var stdoutIsTerminal = func() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// stdinIsTerminal is a variable so tests can take the interactive deinit path
// while feeding the prompt from a pipe.
var stdinIsTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// workspaceRoot discovers the workspace the command runs in, git-style: the
// nearest ancestor of the cwd containing .fledge/. Commands address the same
// flock from anywhere inside its workspace.
func workspaceRoot() (string, error) {
	return workspace.FindRoot(".")
}

// attachHerdr starts the herdr UI attached to session and waits for it. Once
// the UI process owns the terminal, attached is called. Keeping this parent
// alive is intentional: interactive start uses that callback to begin the
// authenticated orchestrator launch while the operator watches it happen.
//
// The attach runs from the workspace root: herdr re-roots a session server at
// the attaching client's cwd (verified on 0.7.4), so attaching from wherever
// the operator happened to stand would move where the session opens its panes.
// It is a variable so tests can exercise the interactive start path without
// the process replacing itself.
var attachHerdr = func(session, root string, attached func()) error {
	herdrPath, err := exec.LookPath("herdr")
	if err != nil {
		return fmt.Errorf("start: herdr not on PATH to attach: %w", err)
	}
	cmd := exec.Command(herdrPath, "session", "attach", session)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if attached != nil {
		attached()
	}
	waitErr := cmd.Wait()
	if waitErr == nil {
		return nil
	}
	// Herdr exits its attached client with status 1 when the session server
	// shuts down. Once the session is verifiably gone that is the normal end
	// of the UI lifecycle, not a failed Fledge start.
	deadline := time.Now().Add(2 * time.Second)
	for {
		s, found, findErr := herdr.Find(session)
		if findErr == nil && (!found || !herdr.Up(s.SocketPath)) {
			return nil
		}
		if time.Now().After(deadline) {
			return waitErr
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// startAfterAttach keeps the orchestrator request alive alongside the Herdr
// client without delaying its first frame. Tests replace it with synchronous
// execution so launch ordering and placement remain deterministic.
var startAfterAttach = func(start func()) {
	go start()
}

// spawnDaemon re-execs fledge as `daemon run` in its own session, scoped to a
// flock and bound to a herdr session, then waits for status to report that
// exact session. A bound daemon can listen before it records its herdr binding.
// The daemon writes its own logs, so the child's stdio only needs to catch a
// startup crash.
func spawnDaemon(root, flockName, session string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(flock.Dir(root, flockName), 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(flock.Dir(root, flockName), protocol.LogName)
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(self, "daemon", "run")
	cmd.Dir = root
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(),
		flock.Env+"="+flockName,
		herdr.SessionEnv+"="+session)
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}

	return waitSpawnDaemonReady(root, flockName, session, logPath)
}

var (
	spawnDaemonStatus = queryDaemonStatus
	spawnDaemonSleep  = time.Sleep
)

func waitSpawnDaemonReady(root, flockName, session, logPath string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := spawnDaemonStatus(root, flockName)
		if err == nil && resp.Session == session {
			return nil
		}
		spawnDaemonSleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not come up; see %s", logPath)
}

var daemonStatusForCLI = queryDaemonStatus

func queryDaemonStatus(root, name string) (protocol.Response, error) {
	return client.Do(root, name, protocol.Request{Op: protocol.OpStatus})
}

var (
	restartDaemonStatus   = queryDaemonStatus
	restartDaemonShutdown = shutdownDaemon
	restartDaemonRunning  = client.Running
	restartSpawnDaemon    = spawnDaemon
	restartSleep          = time.Sleep
	restartWaitTimeout    = 10 * time.Second
)

func shutdownDaemon(root, name string) error {
	_, err := client.Do(root, name, protocol.Request{Op: protocol.OpShutdown})
	return err
}

func runRestart(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("restart")
	}
	if err := rejectFlags("restart", args); err != nil {
		return usageErrorFor("restart", err)
	}
	if len(args) > 1 {
		return usageErrorf("restart", "restart: unexpected argument %q", args[1])
	}
	name, err := flockArg("restart", args)
	if err != nil {
		if len(args) == 1 {
			return usageErrorFor("restart", err)
		}
		return err
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	if !restartDaemonRunning(root, name) {
		return client.ErrNotRunning
	}
	oldStatus, err := restartDaemonStatus(root, name)
	if err != nil {
		return err
	}
	if err := restartDaemonShutdown(root, name); err != nil {
		if shutdownUnsupported(err) && restartDaemonRunning(root, name) {
			return fmt.Errorf("restart requires daemon shutdown support; daemon is still running. Use `fledge flock stop %s`, then `fledge start --flock %s`", name, name)
		}
		return err
	}
	if err := waitRestartDaemonDown(root, name); err != nil {
		return err
	}

	if err := restartSpawnDaemon(root, name, oldStatus.Session); err != nil {
		return replacementFailure(name, root, oldStatus.Session, err)
	}
	newStatus, err := restartDaemonStatus(root, name)
	if err != nil {
		return replacementFailure(name, root, oldStatus.Session, err)
	}
	if err := verifyRestartStatus(oldStatus, newStatus); err != nil {
		return replacementFailure(name, root, oldStatus.Session, err)
	}

	fmt.Printf("flock:   %s\n", name)
	fmt.Printf("session: %s\n", displaySession(newStatus.Session))
	fmt.Printf("old:     pid %d version %s\n", oldStatus.DaemonPID, displayVersion(oldStatus.DaemonVersion))
	fmt.Printf("new:     pid %d version %s\n", newStatus.DaemonPID, displayVersion(newStatus.DaemonVersion))
	return nil
}

func shutdownUnsupported(err error) bool {
	return strings.Contains(err.Error(), `unknown op "shutdown"`)
}

func waitRestartDaemonDown(root, name string) error {
	deadline := time.Now().Add(restartWaitTimeout)
	for time.Now().Before(deadline) {
		if !restartDaemonRunning(root, name) {
			return nil
		}
		restartSleep(100 * time.Millisecond)
	}
	return fmt.Errorf("flock %s: daemon did not shut down", name)
}

func verifyRestartStatus(oldStatus, newStatus protocol.Response) error {
	if oldStatus.Session != newStatus.Session {
		return fmt.Errorf("restart changed herdr session from %q to %q", oldStatus.Session, newStatus.Session)
	}
	if oldStatus.DaemonPID == 0 {
		return errors.New("old daemon status did not report pid")
	}
	if newStatus.DaemonPID == 0 {
		return errors.New("new daemon status did not report pid")
	}
	if oldStatus.DaemonPID == newStatus.DaemonPID {
		return fmt.Errorf("restart kept daemon pid %d", newStatus.DaemonPID)
	}
	current := version.Get()
	if newStatus.DaemonVersion == "" {
		return errors.New("new daemon status did not report version")
	}
	if newStatus.DaemonVersion != current {
		return fmt.Errorf("new daemon version %s does not match current fledge %s", newStatus.DaemonVersion, current)
	}
	return nil
}

func replacementFailure(name, root, session string, err error) error {
	return fmt.Errorf("restart %s: replacement daemon failed; herdr session %q was left running; see %s: %w",
		name, session, daemonLogPath(root, name), err)
}

func daemonLogPath(root, name string) string {
	return filepath.Join(flock.Dir(root, name), protocol.LogName)
}

func displaySession(session string) string {
	if session == "" {
		return "(none)"
	}
	return session
}

func displayVersion(v string) string {
	if v == "" {
		return "(unknown)"
	}
	return v
}

// runFlockStop ends a flock's herdr session and waits for its daemon to
// follow. Only what the daemon reports being bound to is touched: a flock
// whose daemon is already down is left alone.
func runFlockStop(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("flock stop")
	}
	if err := rejectFlags("flock stop", args); err != nil {
		return usageErrorFor("flock stop", err)
	}
	if len(args) > 1 {
		return usageErrorf("flock stop", "flock stop: unexpected argument %q", args[1])
	}
	name, err := flockArg("flock stop", args)
	if err != nil {
		if len(args) == 1 {
			return usageErrorFor("flock stop", err)
		}
		return err
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	return stopFlock(root, name)
}

// stopFlock is the teardown behind `flock stop`, split out so a start that
// cannot finish can roll itself back through exactly the same path.
func stopFlock(root, name string) error {
	if !client.Running(root, name) {
		fmt.Printf("flock %s: daemon already down\n", name)
		return nil
	}
	resp, err := client.Do(root, name, protocol.Request{Op: protocol.OpStatus})
	if err != nil {
		return err
	}
	if resp.Session == "" {
		return fmt.Errorf("flock %s: daemon is bound to no herdr session; stop it by hand", name)
	}
	if err := herdr.Stop(resp.Session); err != nil {
		return err
	}

	// The daemon polls its session every few seconds and exits when the
	// socket goes away; give it comfortably more than one poll interval.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !client.Running(root, name) {
			// A managed session's record is useless once its flock is gone;
			// deleting it keeps herdr's session list from collecting one
			// corpse per stopped flock. A session the operator named with
			// --session is theirs, so only its server is stopped.
			if !strings.HasPrefix(resp.Session, "fledge-") {
				fmt.Printf("flock %s: stopped (herdr session %s ended, daemon followed)\n", name, resp.Session)
				return nil
			}
			if err := herdr.Delete(resp.Session); err != nil {
				fmt.Printf("flock %s: stopped, but the session record remains: %v\n", name, err)
				return nil
			}
			fmt.Printf("flock %s: stopped (herdr session %s ended and deleted, daemon followed)\n", name, resp.Session)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("flock %s: herdr session %s ended but the daemon is still up", name, resp.Session)
}

// runStop tears down the calling flock behind one confirmation when
// FLEDGE_FLOCK is set. Outside a flock it retains the workspace-wide bulk
// behavior. `fledge flock stop [name]` remains the non-interactive path.
//
// The whole command is interactive by design — the confirmation is the only
// thing standing between a typo and tearing down one or more flocks —
// so a missing terminal on either stream is a refusal rather than a default.
// `fledge flock stop <name>` remains the scriptable path.
func runStop(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("stop")
	}
	if err := rejectFlags("stop", args); err != nil {
		return usageErrorFor("stop", err)
	}
	if len(args) != 0 {
		return usageErrorf("stop", "stop: unexpected argument %q", args[0])
	}
	// Gated before the workspace lookup so a scripted run refuses outright
	// rather than reporting on flocks it was never going to touch.
	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return errors.New("fledge stop needs a terminal; run fledge flock stop <name> instead")
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	names, scoped, err := stopTargets(root)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Println("no flocks; run fledge start")
		return nil
	}

	if err := statusOverviewNames(root, names); err != nil {
		return err
	}
	if scoped {
		fmt.Printf("\nstop %s above? [y/N] ", names[0])
	} else {
		fmt.Print("\nstop all flocks above? [y/N] ")
	}
	// A read error means no confirmation was given, so it falls through to
	// the default No along with EOF and a bare enter.
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Println("aborted; nothing stopped")
		return nil
	}

	return stopFlocks(root, names, stopFlock)
}

// stopTargets resolves top-level stop's scope. A pane carrying FLEDGE_FLOCK is
// authoritative context and narrows the command to that exact flock. Outside
// a flock, stop retains its workspace-wide behavior.
func stopTargets(root string) (names []string, scoped bool, err error) {
	if name := os.Getenv(flock.Env); name != "" {
		if err := flock.Validate(name); err != nil {
			return nil, false, fmt.Errorf("%s: %w", flock.Env, err)
		}
		return []string{name}, true, nil
	}
	names, err = flock.List(root)
	return names, false, err
}

// stopFlocks tears every named flock down in order, carrying on past a failure
// so one stuck flock cannot strand the rest of the workspace. stop is injected
// so the aggregation is testable without a daemon per flock.
func stopFlocks(root string, names []string, stop func(root, name string) error) error {
	failed := 0
	for _, name := range names {
		if err := stop(root, name); err != nil {
			fmt.Println(err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d flocks failed to stop", failed, len(names))
	}
	return nil
}

// flockArg resolves the flock a subcommand targets: an explicit argument
// wins, else the calling flock from FLEDGE_FLOCK.
func flockArg(cmd string, args []string) (string, error) {
	switch len(args) {
	case 0:
		name := os.Getenv(flock.Env)
		if name == "" {
			return "", fmt.Errorf("%s: no flock named and %s is unset; run fledge flock list", cmd, flock.Env)
		}
		if err := flock.Validate(name); err != nil {
			return "", fmt.Errorf("%s: %w", flock.Env, err)
		}
		return name, nil
	case 1:
		if err := flock.Validate(args[0]); err != nil {
			return "", err
		}
		return args[0], nil
	default:
		return "", fmt.Errorf("%s: unexpected argument %q", cmd, args[1])
	}
}

// runFlockList is read-only discovery over every flock in the workspace, so
// it deliberately does not need FLEDGE_FLOCK.
func runFlockList(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("flock list")
	}
	if err := rejectFlags("flock list", args); err != nil {
		return usageErrorFor("flock list", err)
	}
	if len(args) != 0 {
		return usageErrorf("flock list", "flock list: unexpected argument %q", args[0])
	}
	root, err := workspaceRoot()
	if errors.Is(err, workspace.ErrNotFound) {
		// A directory with no workspace has no flocks; that is an answer,
		// not an error, just as it was before init ran.
		fmt.Println("no flocks; run fledge start")
		return nil
	}
	if err != nil {
		return err
	}
	return statusOverview(root)
}

func runFlockStatus(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("flock status")
	}
	if err := rejectFlags("flock status", args); err != nil {
		return usageErrorFor("flock status", err)
	}
	if len(args) > 1 {
		return usageErrorf("flock status", "flock status: unexpected argument %q", args[1])
	}
	name, err := flockArg("flock status", args)
	if err != nil {
		if len(args) == 1 {
			return usageErrorFor("flock status", err)
		}
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	return statusFlock(root, name)
}

func statusFlock(root, name string) error {
	fmt.Printf("flock:  %s\n", name)
	if !client.Running(root, name) {
		fmt.Println("daemon: down")
		fmt.Println("herdr:  none")
		return nil
	}
	fmt.Println("daemon: up")
	fmt.Printf("socket: %s\n", daemon.SocketPath(root, name))

	resp, err := daemonStatusForCLI(root, name)
	if err != nil {
		return err
	}
	if resp.DaemonPID != 0 {
		fmt.Printf("pid:    %d\n", resp.DaemonPID)
	}
	if resp.DaemonVersion != "" {
		fmt.Printf("version: %s\n", resp.DaemonVersion)
	}
	switch {
	case resp.Session == "":
		fmt.Println("herdr:  none")
	case herdr.Up(resp.SessionSocket):
		fmt.Printf("herdr:  %s (up)\n", resp.Session)
	default:
		fmt.Printf("herdr:  %s (down)\n", resp.Session)
	}
	printAgents(resp.Agents)
	return nil
}

func statusOverview(root string) error {
	names, err := flock.List(root)
	if err != nil {
		return err
	}
	return statusOverviewNames(root, names)
}

func statusOverviewNames(root string, names []string) error {
	if len(names) == 0 {
		fmt.Println("no flocks; run fledge start")
		return nil
	}

	widest := 0
	for _, n := range names {
		if len(n) > widest {
			widest = len(n)
		}
	}
	for _, n := range names {
		if !client.Running(root, n) {
			fmt.Printf("%-*s  down\n", widest, n)
			continue
		}
		resp, err := client.Do(root, n, protocol.Request{Op: protocol.OpStatus})
		if err != nil {
			fmt.Printf("%-*s  up    %v\n", widest, n, err)
			continue
		}
		live := 0
		for _, a := range resp.Agents {
			if a.Alive {
				live++
			}
		}
		session := resp.Session
		if session == "" {
			session = "none"
		}
		fmt.Printf("%-*s  up    herdr:%-16s agents:%d/%d\n", widest, n, session, live, len(resp.Agents))
	}
	return nil
}

func printAgents(agents []protocol.Agent) {
	if len(agents) == 0 {
		fmt.Println("\nno agents registered")
		return
	}

	fmt.Println()
	for _, row := range agentRows(agents) {
		fmt.Println(row)
	}
}

// agentRows renders the roster one line per agent. Spawned agents carry launch
// columns after their liveness; self-registered agents leave them empty, and
// trimming the resulting trailing space is what keeps a roster of nothing but
// self-registered agents rendering exactly as it always has.
func agentRows(agents []protocol.Agent) []string {
	var nameW, modelW, paneW int
	for _, a := range agents {
		if n := len(a.Name); n > nameW {
			nameW = n
		}
		if n := len(a.Model); n > modelW {
			modelW = n
		}
		if n := len(a.PaneID); n > paneW {
			paneW = n
		}
	}

	rows := make([]string, 0, len(agents))
	for _, a := range agents {
		liveness := "dead"
		if a.Alive {
			liveness = "alive"
		}
		row := fmt.Sprintf("%-*s  %-10s %-8d %-5s  %-6s %-*s %-*s %s",
			nameW, a.Name, a.Species, a.PID, liveness,
			a.Integration, modelW, a.Model, paneW, a.PaneID, a.State)
		if a.Agent != "" || a.Profile != "" || a.Source != "" {
			row += fmt.Sprintf("  agent=%s profile=%s source=%s", a.Agent, a.Profile, a.Source)
		}
		if a.WorkspaceID != "" || a.WorkspaceLabel != "" {
			row += fmt.Sprintf("  workspace=%s workspace_id=%s", a.WorkspaceLabel, a.WorkspaceID)
		}
		rows = append(rows, strings.TrimRight(row, " "))
	}
	return rows
}

func runAgent(args []string) error {
	if len(args) == 0 {
		return printHelp("agent")
	}
	if isHelpFlag(args[0]) {
		return printHelp("agent")
	}
	if args[0] == "help" {
		return runHelp("agent", args[1:])
	}
	switch args[0] {
	case "register":
		return runAgentRegister(args[1:])
	case "spawn":
		return runAgentSpawn(args[1:])
	case "ready":
		return runAgentReady(args[1:])
	case "stop":
		return runAgentStop(args[1:])
	case "list":
		return runAgentList(args[1:])
	case "models":
		return runAgentModels(args[1:])
	case "types":
		return runAgentTypes(args[1:])
	case "msg":
		return runAgentMsg(args[1:])
	default:
		return usageErrorf("agent", "unknown agent subcommand %q", args[0])
	}
}

func runAgentRegister(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent register")
	}
	slug, args, err := takeFlag(args, "--species", "-S")
	if err != nil {
		return usageErrorFor("agent register", err)
	}
	pidArg, args, err := takeFlag(args, "--pid", "-P")
	if err != nil {
		return usageErrorFor("agent register", err)
	}
	if err := rejectFlags("agent register", args); err != nil {
		return usageErrorFor("agent register", err)
	}
	if len(args) != 1 {
		return usageErrorf("agent register", "agent register: want exactly one type argument")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}

	// The agent's identity defaults to its session leader (the pane's
	// long-lived process), not fledge's immediate parent: agents shell out
	// through transient `sh -c` processes that die the moment the command
	// returns. --pid lets the spawning layer name the pane process directly.
	pid := sessionID()
	if pidArg != "" {
		pid, err = strconv.Atoi(pidArg)
		if err != nil || pid <= 0 {
			return usageErrorf("agent register", "agent register: bad --pid %q", pidArg)
		}
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	if err := agentcfg.Synchronize(root); err != nil {
		return err
	}
	typeName := args[0]
	var agentName, profile, source string
	if strings.HasSuffix(args[0], agentcfg.DefinitionSuffix) {
		d, _, err := agentcfg.FindDefinition(root, args[0])
		if err != nil {
			return err
		}
		typeName, agentName, profile, source = d.Name, d.Name, d.Profile, d.Source
	}
	resp, err := client.Do(root, flockName, protocol.Request{
		Op:      protocol.OpRegister,
		Type:    typeName,
		Species: slug,
		PID:     pid,
		Agent:   agentName,
		Profile: profile,
		Source:  source,
	})
	if err != nil {
		return err
	}
	fmt.Println(resp.Name)
	return nil
}

// runAgentSpawn launches an agent the daemon owns, named either by an entry in
// agents.json or by a bare model id the routing table knows.
var agentSpawnRequest = client.Do

func runAgentSpawn(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent spawn")
	}
	model, args, err := takeFlag(args, "--model", "-M")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	profile, args, err := takeFlag(args, "--profile", "-L")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	// -D, not -P: short flags are unique across the whole CLI, and agent
	// register already holds -P for --pid.
	provider, args, err := takeFlag(args, "--provider", "-D")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	integration, args, err := takeFlag(args, "--integration", "-I")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	cwd, args, err := takeFlag(args, "--cwd", "-C")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	slug, args, err := takeFlag(args, "--species", "-S")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	timeoutArg, args, err := takeFlag(args, "--timeout", "-T")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	workspaceSelector, args, err := takeFlag(args, "--workspace", "-W")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	tabSelector, args, err := takeFlag(args, "--tab", "-B")
	if err != nil {
		return usageErrorFor("agent spawn", err)
	}
	if err := rejectFlags("agent spawn", args); err != nil {
		return usageErrorFor("agent spawn", err)
	}
	if len(args) > 1 {
		return usageErrorf("agent spawn", "agent spawn: unexpected argument %q", args[1])
	}

	var agent string
	if len(args) == 1 {
		agent = args[0]
	}
	// Bare and interactive: offer the catalog rather than the usage error.
	// Requiring provider to be empty too keeps --provider without --model the
	// error it has always been, and a non-terminal stdin falls through to the
	// same error scripted callers already get.
	if agent == "" && profile == "" && model == "" && provider == "" && integration == "" && stdinIsTerminal() {
		root, err := workspaceRoot()
		if err != nil {
			return err
		}
		defs, profiles, err := agentcfg.LoadDefinitions(root)
		if err != nil {
			return err
		}
		if len(defs) == 0 {
			return usageErrorf("agent spawn",
				"agent spawn: choose an agent, --profile, or --model\nno configured agents")
		}
		reader := bufio.NewReader(os.Stdin)
		if agent, err = pickAgentDefinition(defs, reader, os.Stdout); err != nil {
			return err
		}
		if defs[agent].Profile == "" {
			if len(profiles) == 0 {
				return errors.New("selected agent is profile-agnostic and no profiles are configured")
			}
			if profile, err = pickAgentConfig(profiles, reader, os.Stdout); err != nil {
				return err
			}
		}
	}
	// The daemon rejects the same pair, but saying so here keeps the operator
	// from needing a running flock to learn they typed the command wrong.
	if integration != "" && model == "" {
		return usageErrorf("agent spawn", "agent spawn: --integration only applies to --model")
	}
	if provider != "" && model == "" {
		return usageErrorf("agent spawn", "agent spawn: --provider only applies to --model")
	}
	if model != "" && (agent != "" || profile != "") {
		return usageErrorf("agent spawn", "agent spawn: --model cannot be combined with an agent or --profile")
	}
	if model == "" && agent == "" && profile == "" {
		return usageErrorf("agent spawn", "agent spawn: choose an agent, --profile, or --model")
	}
	if (workspaceSelector == "") != (tabSelector == "") {
		return usageErrorf("agent spawn", "agent spawn: --workspace and --tab must be used together")
	}
	var timeout time.Duration
	if timeoutArg != "" {
		timeout, err = time.ParseDuration(timeoutArg)
		if err != nil || timeout <= 0 {
			return usageErrorf("agent spawn", "agent spawn: bad --timeout %q", timeoutArg)
		}
	}

	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	if err := agentcfg.Synchronize(root); err != nil {
		return err
	}
	resp, err := agentSpawnRequest(root, flockName, protocol.Request{
		Op:          protocol.OpSpawn,
		Agent:       agent,
		Profile:     profile,
		Model:       model,
		Provider:    provider,
		Integration: integration,
		Cwd:         cwd,
		Species:     slug,
		TimeoutMS:   timeout.Milliseconds(),
		Workspace:   workspaceSelector,
		Tab:         tabSelector,
	})
	if err != nil {
		return err
	}
	fmt.Println(resp.Name)
	if resp.PaneID != "" {
		fmt.Printf("pane: %s\n", resp.PaneID)
	}
	return nil
}

func runAgentReady(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent ready")
	}
	noWait, args := takeBoolFlag(args, "--no-wait", "-O")
	if err := rejectFlags("agent ready", args); err != nil {
		return usageErrorFor("agent ready", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent ready", "agent ready: unexpected argument %q", args[0])
	}
	name, token := os.Getenv(protocol.AgentNameEnv), os.Getenv(protocol.ReadyTokenEnv)
	if name == "" || token == "" {
		return errors.New("agent ready must run inside a Fledge-started agent")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	runtimeSessionID := strings.TrimSpace(os.Getenv(protocol.CodexThreadIDEnv))
	resp, err := agentMsgRequest(root, flockName, protocol.Request{
		Op: protocol.OpReady, Name: name, Token: token, NoWait: noWait,
		SessionID: runtimeSessionID,
	})
	if err != nil {
		if !errors.Is(err, client.ErrNotRunning) {
			return err
		}
		if !noWait {
			return err
		}
		if err := daemon.WriteReadySignalWithSession(root, flockName, name, token, runtimeSessionID); err != nil {
			return fmt.Errorf("publish readiness signal: %w", err)
		}
		fmt.Println(name)
		fmt.Fprintln(os.Stderr, "warning: automatic inbox delivery is unavailable: the current interactive launcher does not own a same-session integration control channel; messages remain durable for `fledge agent msg inbox` or `fledge agent msg wait`")
		return nil
	}
	if !noWait {
		resp, err = agentMsgRequest(root, flockName, protocol.Request{
			Op:    protocol.OpReceive,
			As:    name,
			Token: token,
		})
		if err != nil {
			return err
		}
		if err := json.NewEncoder(os.Stdout).Encode(resp.Message); err != nil {
			return err
		}
		return acknowledgeMessage(root, flockName, name, resp.Message)
	}
	fmt.Println(resp.Name)
	if resp.InboxDelivery == "manual" {
		fmt.Fprintln(os.Stderr, "warning: automatic inbox delivery is unavailable: the current interactive launcher does not own a same-session integration control channel; messages remain durable for `fledge agent msg inbox` or `fledge agent msg wait`")
	}
	return nil
}

func runAgentStop(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent stop")
	}
	if err := rejectFlags("agent stop", args); err != nil {
		return usageErrorFor("agent stop", err)
	}
	if len(args) != 1 {
		return usageErrorf("agent stop", "agent stop: want exactly one agent name")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	_, err = client.Do(root, flockName, protocol.Request{Op: protocol.OpStop, Name: args[0]})
	return err
}

// sessionID returns the calling session's leader pid, falling back to the
// parent pid. getsid has no stdlib wrapper, so this uses the raw syscall.
func sessionID() int {
	sid, _, errno := syscall.Syscall(syscall.SYS_GETSID, 0, 0, 0)
	if errno != 0 {
		return os.Getppid()
	}
	return int(sid)
}

func runAgentList(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent list")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	if err := rejectFlags("agent list", args); err != nil {
		return usageErrorFor("agent list", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent list", "agent list: unexpected argument %q", args[0])
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	resp, err := client.Do(root, flockName, protocol.Request{Op: protocol.OpList})
	if err != nil {
		return err
	}
	if asJSON {
		// An empty roster is [], not null: consumers index the result.
		if resp.Agents == nil {
			resp.Agents = []protocol.Agent{}
		}
		return encodeJSON(resp.Agents)
	}
	printAgents(resp.Agents)
	return nil
}

func encodeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func runAgentMsg(args []string) error {
	if len(args) == 0 {
		return printHelp("agent msg")
	}
	if isHelpFlag(args[0]) {
		return printHelp("agent msg")
	}
	if args[0] == "help" {
		return runHelp("agent msg", args[1:])
	}
	switch args[0] {
	case "send":
		return runAgentMsgSend(args[1:])
	case "reply":
		return runAgentMsgReply(args[1:])
	case "inbox":
		return runAgentMsgInbox(args[1:])
	case "wait":
		return runAgentMsgWait(args[1:])
	default:
		return usageErrorf("agent msg", "unknown agent msg subcommand %q", args[0])
	}
}

var agentMsgRequest = client.Do

func runAgentMsgSend(args []string) error {
	// Help only when it is the sole argument: the recipient and body are
	// positional by contract, so a flag-shaped value there (e.g. a body of
	// "-H") must not be read as a help request and silently drop the message.
	if len(args) == 1 && isHelpFlag(args[0]) {
		return printHelp("agent msg send")
	}
	replyTo, args, err := takeFlag(args, "--reply-to", "-R")
	if err != nil {
		return usageErrorFor("agent msg send", err)
	}
	bodyFile, args, err := takeFlag(args, "--body-file", "-F")
	if err != nil {
		return usageErrorFor("agent msg send", err)
	}
	if err := rejectFlags("agent msg send", args); err != nil {
		return usageErrorFor("agent msg send", err)
	}
	if len(args) < 1 || len(args) > 2 {
		return usageErrorf("agent msg send",
			"agent msg send: want a recipient and exactly one body or --body-file")
	}
	if (len(args) == 2) == (bodyFile != "") {
		return usageErrorf("agent msg send",
			"agent msg send: provide exactly one positional body or --body-file")
	}
	body := ""
	if bodyFile != "" {
		var data []byte
		if bodyFile == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(bodyFile)
		}
		if err != nil {
			return fmt.Errorf("agent msg send: read body file %q: %w", bodyFile, err)
		}
		body = string(data)
	} else {
		body = args[1]
	}
	from := strings.TrimSpace(os.Getenv(protocol.AgentNameEnv))
	if from == "" {
		return errors.New("agent msg send: FLEDGE_AGENT_NAME is required")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	roster, err := agentMsgRequest(root, flockName, protocol.Request{Op: protocol.OpList})
	if err != nil {
		return err
	}
	registered := false
	for _, agent := range roster.Agents {
		if agent.Name == from {
			registered = true
			break
		}
	}
	if !registered {
		return fmt.Errorf("agent msg send: no registered agent %q", from)
	}

	resp, err := agentMsgRequest(root, flockName, protocol.Request{
		Op:         protocol.OpSend,
		From:       from,
		To:         args[0],
		Body:       body,
		ReplyTo:    replyTo,
		Token:      os.Getenv(protocol.ReadyTokenEnv),
		Credential: os.Getenv(protocol.AgentCredentialEnv),
	})
	if err != nil {
		return err
	}
	fmt.Println(resp.ID)
	return nil
}

func runAgentMsgReply(args []string) error {
	if len(args) == 1 && isHelpFlag(args[0]) {
		return printHelp("agent msg reply")
	}
	bodyFile, args, err := takeFlag(args, "--body-file", "-F")
	if err != nil {
		return usageErrorFor("agent msg reply", err)
	}
	if err := rejectFlags("agent msg reply", args); err != nil {
		return usageErrorFor("agent msg reply", err)
	}
	if len(args) < 1 || len(args) > 2 {
		return usageErrorf("agent msg reply",
			"agent msg reply: want a message id and exactly one body or --body-file")
	}
	if (len(args) == 2) == (bodyFile != "") {
		return usageErrorf("agent msg reply",
			"agent msg reply: provide exactly one positional body or --body-file")
	}
	var body string
	if bodyFile != "" {
		var data []byte
		if bodyFile == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(bodyFile)
		}
		if err != nil {
			return fmt.Errorf("agent msg reply: read body file %q: %w", bodyFile, err)
		}
		body = string(data)
	} else {
		body = args[1]
	}
	from := strings.TrimSpace(os.Getenv(protocol.AgentNameEnv))
	if from == "" {
		return errors.New("agent msg reply: FLEDGE_AGENT_NAME is required")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	resp, err := agentMsgRequest(root, flockName, protocol.Request{
		Op: protocol.OpReply, From: from, ID: args[0], Body: body,
		Token:      os.Getenv(protocol.ReadyTokenEnv),
		Credential: os.Getenv(protocol.AgentCredentialEnv),
	})
	if err != nil {
		return err
	}
	fmt.Println(resp.ID)
	return nil
}

func runAgentMsgInbox(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent msg inbox")
	}
	replyTo, args, err := takeFlag(args, "--reply-to", "-R")
	if err != nil {
		return usageErrorFor("agent msg inbox", err)
	}
	from, args, err := takeFlag(args, "--from", "")
	if err != nil {
		return usageErrorFor("agent msg inbox", err)
	}
	if err := rejectFlags("agent msg inbox", args); err != nil {
		return usageErrorFor("agent msg inbox", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent msg inbox", "agent msg inbox: unexpected argument %q", args[0])
	}
	as := strings.TrimSpace(os.Getenv(protocol.AgentNameEnv))
	if as == "" {
		return errors.New("agent msg inbox: FLEDGE_AGENT_NAME is required")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	resp, err := agentMsgRequest(root, flockName, protocol.Request{
		Op:         protocol.OpPeek,
		As:         as,
		From:       from,
		ReplyTo:    replyTo,
		Token:      os.Getenv(protocol.ReadyTokenEnv),
		Credential: os.Getenv(protocol.AgentCredentialEnv),
	})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(resp.Message); err != nil {
		return err
	}
	return acknowledgeMessage(root, flockName, as, resp.Message)
}

func runAgentMsgWait(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent msg wait")
	}
	replyTo, args, err := takeFlag(args, "--reply-to", "-R")
	if err != nil {
		return usageErrorFor("agent msg wait", err)
	}
	from, args, err := takeFlag(args, "--from", "")
	if err != nil {
		return usageErrorFor("agent msg wait", err)
	}
	timeout, args, err := takeFlag(args, "--timeout", "-T")
	if err != nil {
		return usageErrorFor("agent msg wait", err)
	}
	if err := rejectFlags("agent msg wait", args); err != nil {
		return usageErrorFor("agent msg wait", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent msg wait", "agent msg wait: unexpected argument %q", args[0])
	}
	as := strings.TrimSpace(os.Getenv(protocol.AgentNameEnv))
	if as == "" {
		return errors.New("agent msg wait: FLEDGE_AGENT_NAME is required")
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}

	var timeoutMS int64
	if timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return usageErrorf("agent msg wait", "agent msg wait: %v", err)
		}
		// The daemon gates on TimeoutMS > 0, so a non-positive timeout would
		// wait forever rather than bounding the wait. Reject it, matching the
		// agent spawn guard.
		if d <= 0 {
			return usageErrorf("agent msg wait", "agent msg wait: bad --timeout %q", timeout)
		}
		timeoutMS = d.Milliseconds()
	}

	resp, err := agentMsgRequest(root, flockName, protocol.Request{
		Op:         protocol.OpReceive,
		As:         as,
		From:       from,
		ReplyTo:    replyTo,
		TimeoutMS:  timeoutMS,
		Token:      os.Getenv(protocol.ReadyTokenEnv),
		Credential: os.Getenv(protocol.AgentCredentialEnv),
	})
	if err != nil {
		return err
	}
	if err := json.NewEncoder(os.Stdout).Encode(resp.Message); err != nil {
		return err
	}
	return acknowledgeMessage(root, flockName, as, resp.Message)
}

func acknowledgeMessage(root, flockName, as string, msg *protocol.Message) error {
	if msg == nil {
		return nil
	}
	_, err := agentMsgRequest(root, flockName, protocol.Request{
		Op:         protocol.OpAck,
		As:         as,
		ID:         msg.ID,
		Token:      os.Getenv(protocol.ReadyTokenEnv),
		Credential: os.Getenv(protocol.AgentCredentialEnv),
	})
	return err
}

// runDaemon is hidden from public help: operators start the daemon
// through fledge start, and this exists for that command and for tests.
func runDaemon(args []string) error {
	if len(args) != 1 || args[0] != "run" {
		bad := "daemon"
		if len(args) > 0 {
			bad = args[0]
		}
		return usageErrorf("", "unknown command %q", bad)
	}
	flockName, err := flock.FromEnv()
	if err != nil {
		return err
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	return daemon.RunBound(root, flockName, os.Getenv(herdr.SessionEnv))
}

func runInit(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("init")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	fresh, args := takeBoolFlag(args, "--fresh", "-X")
	if err := rejectFlags("init", args); err != nil {
		return usageErrorFor("init", err)
	}
	if len(args) > 1 {
		return usageErrorf("init", "init: unexpected argument %q", args[1])
	}
	if fresh && asJSON {
		return usageErrorf("init", "init: --fresh cannot be combined with --json")
	}

	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	freshened := false
	if fresh {
		proceed, removed, err := prepareFreshInit(root, abs)
		if err != nil {
			return err
		}
		if !proceed {
			return nil
		}
		freshened = removed
	}

	var notes []string
	// Checked from the parent so re-initializing a workspace does not warn
	// about itself. A nested workspace shadows the enclosing one for every
	// command run beneath it, which is worth doing only on purpose.
	if parent, err := workspace.FindRoot(filepath.Dir(abs)); err == nil {
		notes = append(notes, fmt.Sprintf("nested inside workspace at %s", parent))
	}

	existed, err := scaffold.Ensure(root)
	if err != nil {
		return err
	}
	if err := agentcfg.MigrateLegacyGenerated(root); err != nil {
		return err
	}

	ignored, err := scaffold.EnsureGitignore(root)
	if err != nil {
		return err
	}

	configs, discoveryNotes := catalog.Discover()
	for _, n := range discoveryNotes {
		notes = append(notes, n.Detail)
	}
	models := map[string]int{}
	for _, c := range configs {
		models[c.Integration]++
	}
	// An empty discovery keeps whatever catalog the last init wrote: a broken
	// PATH is not "no models".
	if len(configs) > 0 {
		if err := catalog.Write(root, configs); err != nil {
			return err
		}
	}
	if err := agentcfg.Synchronize(root); err != nil {
		return err
	}
	if asJSON {
		return encodeJSON(initSummary{
			Root:           abs,
			Existed:        existed,
			GitignoreAdded: ignored,
			CatalogWritten: len(configs) > 0,
			Models:         models,
			Notes:          notes,
		})
	}

	if freshened {
		fmt.Printf("fledge freshly initialized in %s\n", abs)
	} else if existed {
		fmt.Printf("fledge re-initialized in %s\n", abs)
	} else {
		fmt.Printf("fledge initialized in %s\n", abs)
	}
	if len(ignored) > 0 {
		fmt.Printf("added %s to .gitignore\n", strings.Join(ignored, ", "))
	}
	if len(configs) > 0 {
		integrations := make([]string, 0, len(models))
		for integration := range models {
			integrations = append(integrations, integration)
		}
		sort.Strings(integrations)
		counts := make([]string, 0, len(integrations))
		for _, integration := range integrations {
			counts = append(counts, fmt.Sprintf("%d from %s", models[integration], integration))
		}
		fmt.Printf("wrote %s/%s (%s)\n", scaffold.DirName, agentcfg.CatalogName, strings.Join(counts, ", "))
	} else if freshened {
		fmt.Printf("no integration answered discovery; %s/%s was not created\n", scaffold.DirName, agentcfg.CatalogName)
	} else {
		fmt.Printf("no integration answered discovery; %s/%s left as it was\n", scaffold.DirName, agentcfg.CatalogName)
	}
	for _, note := range notes {
		fmt.Printf("note: %s\n", note)
	}
	return nil
}

// prepareFreshInit previews and removes an existing .fledge tree only after
// the operator has seen its contents and explicitly confirmed the destructive
// replacement. proceed is false only when the operator declines; removed says
// whether an existing tree was actually deleted.
func prepareFreshInit(root, abs string) (proceed, removed bool, err error) {
	target := filepath.Join(root, scaffold.DirName)
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}

	if info.IsDir() {
		names, err := flock.List(root)
		if err != nil {
			return false, false, err
		}
		var running []string
		for _, name := range names {
			if client.Running(root, name) {
				running = append(running, name)
			}
		}
		if len(running) > 0 {
			return false, false, fmt.Errorf("init --fresh: flocks still running: %s; run fledge flock stop first", strings.Join(running, ", "))
		}
	}

	if !stdinIsTerminal() || !stdoutIsTerminal() {
		return false, false, errors.New("init --fresh is interactive and needs terminals on stdin and stdout")
	}

	fmt.Println("WARNING: --fresh permanently deletes the entire .fledge tree, including user definitions, context artifacts, and flock history.")
	if err := printFledgeTree(root, target, info); err != nil {
		return false, false, err
	}
	risk := freshInitGitRisk(abs)
	switch {
	case risk.Unavailable != "":
		fmt.Printf("\nWARNING: Git could not verify this data (%s). Assume every item above is unrecoverable.\n", risk.Unavailable)
	case len(risk.Paths) > 0:
		fmt.Println("\nWARNING: these files are not tracked by Git and are not ignored; fresh init will permanently delete them:")
		for _, name := range risk.Paths {
			fmt.Printf("  %s\n", name)
		}
	}

	fmt.Printf("\ndestroy and freshly initialize %s in %s? [y/N] ", scaffold.DirName, abs)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Println("aborted; nothing removed")
		return false, false, nil
	}

	if err := os.RemoveAll(target); err != nil {
		return false, false, err
	}
	fmt.Printf("removed %s; rebuilding from scratch\n", filepath.Join(abs, scaffold.DirName))
	return true, true, nil
}

type freshGitRiskReport struct {
	Paths       []string
	Unavailable string
}

// freshInitGitRisk finds files that Git considers untracked and that no ignore
// rule covers. Those files have no Git recovery path after the confirmed
// removal. Outside a Git worktree the whole preview is conservatively treated
// as unverifiable.
func freshInitGitRisk(abs string) freshGitRiskReport {
	check := exec.Command("git", "-C", abs, "rev-parse", "--is-inside-work-tree")
	out, err := check.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "true" {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = "not a Git worktree"
		}
		return freshGitRiskReport{Unavailable: reason}
	}

	cmd := exec.Command("git", "-C", abs, "ls-files", "--others", "--exclude-standard", "-z", "--", scaffold.DirName)
	out, err = cmd.CombinedOutput()
	if err != nil {
		reason := strings.TrimSpace(string(out))
		if reason == "" {
			reason = err.Error()
		}
		return freshGitRiskReport{Unavailable: reason}
	}
	var paths []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			paths = append(paths, filepath.ToSlash(name))
		}
	}
	sort.Strings(paths)
	return freshGitRiskReport{Paths: paths}
}

func printFledgeTree(root, target string, info os.FileInfo) error {
	if !info.IsDir() {
		fmt.Println(scaffold.DirName)
		return nil
	}
	return filepath.WalkDir(target, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			rel += "/"
		}
		fmt.Println(rel)
		return nil
	})
}

// initSummary is init's --json shape.
type initSummary struct {
	Root           string         `json:"root"`
	Existed        bool           `json:"existed"`
	GitignoreAdded []string       `json:"gitignore_added,omitempty"`
	CatalogWritten bool           `json:"catalog_written"`
	Models         map[string]int `json:"models,omitempty"`
	Notes          []string       `json:"notes,omitempty"`
}

// runDeinit is init's inverse: it lists the .fledge tree, asks for
// confirmation on a real terminal, and removes it. There is deliberately no
// force flag — rm -rf .fledge is the scriptable path.
func runDeinit(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("deinit")
	}
	if err := rejectFlags("deinit", args); err != nil {
		return usageErrorFor("deinit", err)
	}
	if len(args) > 1 {
		return usageErrorf("deinit", "deinit: unexpected argument %q", args[1])
	}

	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	target := filepath.Join(root, scaffold.DirName)
	// Lstat so a symlinked .fledge is treated as the link itself: it is
	// listed as one entry and RemoveAll unlinks it without following.
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		fmt.Printf("nothing to remove: no %s in %s\n", scaffold.DirName, abs)
		return nil
	}
	if err != nil {
		return err
	}

	if info.IsDir() {
		names, err := flock.List(root)
		if err != nil {
			return err
		}
		var running []string
		for _, name := range names {
			if client.Running(root, name) {
				running = append(running, name)
			}
		}
		if len(running) > 0 {
			return fmt.Errorf("deinit: flocks still running: %s; run fledge flock stop first", strings.Join(running, ", "))
		}
	}

	if !stdinIsTerminal() {
		return fmt.Errorf("deinit is interactive and needs a terminal on stdin")
	}

	if err := printFledgeTree(root, target, info); err != nil {
		return err
	}

	fmt.Printf("\nremove everything above from %s? [y/N] ", abs)
	// A read error means no confirmation was given, so it falls through to
	// the default No along with EOF and a bare enter.
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		fmt.Println("aborted; nothing removed")
		return nil
	}

	if err := os.RemoveAll(target); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", filepath.Join(abs, scaffold.DirName))
	return nil
}

// configProvider names the vendor a config talks to. The claude and codex
// integrations carry no provider of their own — the field is pi-only, since it
// is a pi flag — but each CLI reaches exactly one vendor, so the catalog can
// say which without the config storing it. Pi's "opencode" provider is shown
// under its product name "opencode-zen", which also groups it after
// "opencode-go" in the listings.
func configProvider(c agentcfg.Config) string {
	switch c.Integration {
	case "claude":
		return "anthropic"
	case "codex":
		return "openai"
	}
	if c.Provider == "opencode" {
		return "opencode-zen"
	}
	return c.Provider
}

// modelRows renders the spawnable catalog under a header, grouped by provider
// alphabetically and by name within a provider, with a blank line between
// groups. Unlike agentRows this carries a header: names alone do not say which
// integration or model an entry launches.
func modelRows(configs map[string]agentcfg.Config) []string {
	names := groupedNames(configs)

	nameW, integrationW, providerW := len("NAME"), len("INTEGRATION"), len("PROVIDER")
	for name, c := range configs {
		if n := len(name); n > nameW {
			nameW = n
		}
		if n := len(c.Integration); n > integrationW {
			integrationW = n
		}
		if n := len(configProvider(c)); n > providerW {
			providerW = n
		}
	}

	format := func(name, integration, provider, model string) string {
		row := fmt.Sprintf("%-*s  %-*s  %-*s  %s",
			nameW, name, integrationW, integration, providerW, provider, model)
		return strings.TrimRight(row, " ")
	}

	rows := []string{format("NAME", "INTEGRATION", "PROVIDER", "MODEL")}
	prev := ""
	for i, name := range names {
		c := configs[name]
		provider := configProvider(c)
		if i > 0 && provider != prev {
			rows = append(rows, "")
		}
		prev = provider
		rows = append(rows, format(name, c.Integration, provider, c.Model))
	}
	return rows
}

// runAgentModels lists what can be spawned. It reads the config file directly,
// so it needs neither a flock nor a running daemon.
func runAgentModels(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent models")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	if err := rejectFlags("agent models", args); err != nil {
		return usageErrorFor("agent models", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent models", "agent models: unexpected argument %q", args[0])
	}

	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	if err := agentcfg.Synchronize(root); err != nil {
		return err
	}
	configs, err := agentcfg.Load(root)
	if err != nil {
		return err
	}
	if asJSON {
		return encodeJSON(modelEntries(configs))
	}
	if len(configs) == 0 {
		fmt.Printf("no models; add them to %s/%s\n", scaffold.DirName, agentcfg.FileName)
		return nil
	}

	for _, row := range modelRows(configs) {
		fmt.Println(row)
	}
	fmt.Printf("\n%d models\n", len(configs))
	return nil
}

type agentTypeEntry struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Tools       []string            `json:"tools,omitempty"`
	Profile     string              `json:"profile,omitempty"`
	Workspace   *agentcfg.Workspace `json:"workspace,omitempty"`
	Source      string              `json:"source"`
}

func runAgentTypes(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("agent types")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	if err := rejectFlags("agent types", args); err != nil {
		return usageErrorFor("agent types", err)
	}
	if len(args) != 0 {
		return usageErrorf("agent types", "agent types: unexpected argument %q", args[0])
	}
	root, err := workspaceRoot()
	if err != nil {
		return err
	}
	defs, _, err := agentcfg.LoadDefinitions(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]agentTypeEntry, 0, len(names))
	for _, name := range names {
		d := defs[name]
		entries = append(entries, agentTypeEntry{Name: d.Name, Description: d.Description, Tools: d.Tools, Profile: d.Profile, Workspace: d.Workspace, Source: d.Source})
	}
	if asJSON {
		return encodeJSON(entries)
	}
	for _, e := range entries {
		profile := e.Profile
		if profile == "" {
			profile = "(profile required)"
		}
		fmt.Printf("%-24s %-22s %s\n", e.Name, profile, e.Description)
	}
	return nil
}

func pickAgentDefinition(defs map[string]agentcfg.Definition, in io.Reader, out io.Writer) (string, error) {
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintln(out, "Configured agents:")
	for i, name := range names {
		fmt.Fprintf(out, "  %d. %-24s %s\n", i+1, name, defs[name].Description)
	}
	fmt.Fprint(out, "\nSpawn which agent? (number or name): ")
	line, _ := bufio.NewReader(in).ReadString('\n')
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errors.New("spawn cancelled")
	}
	if n, err := strconv.Atoi(answer); err == nil {
		if n < 1 || n > len(names) {
			return "", fmt.Errorf("invalid selection %q", answer)
		}
		return names[n-1], nil
	}
	if _, ok := defs[answer]; !ok {
		return "", fmt.Errorf("invalid selection %q", answer)
	}
	return answer, nil
}

// groupedNames orders config names by provider, then by name within a
// provider. Both renderings share it so the table and --json never disagree.
func groupedNames(configs map[string]agentcfg.Config) []string {
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := configProvider(configs[names[i]]), configProvider(configs[names[j]])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names
}

// pickerRows renders the interactive spawn menu: the catalog numbered over
// groupedNames, so the number an operator reads indexes the same slice
// pickAgentConfig resolves against and the two can never disagree. Group
// headers carry the integration in parens — unlike the models table, picker
// rows have no INTEGRATION column, and openai (the codex CLI) next to
// openai-codex (pi on codex limits) is ambiguous without it.
func pickerRows(configs map[string]agentcfg.Config) []string {
	names := groupedNames(configs)

	nameW, numW := 0, len(strconv.Itoa(len(names)))
	for name := range configs {
		if n := len(name); n > nameW {
			nameW = n
		}
	}

	rows := []string{"Configured agents:", ""}
	prev := ""
	for i, name := range names {
		c := configs[name]
		provider := configProvider(c)
		if provider != prev {
			rows = append(rows, fmt.Sprintf("  %s (%s)", provider, c.Integration))
		}
		prev = provider
		row := fmt.Sprintf("    %*d. %-*s   %s", numW, i+1, nameW, name, c.Model)
		rows = append(rows, strings.TrimRight(row, " "))
	}
	return rows
}

// pickAgentConfig prints the menu and reads one selection, returning the chosen
// config name. Input is either a menu number or an exact config name; a number
// is always read as an index, so an all-digit config name must be picked by its
// number rather than typed. Cancelling and mistyping are runtime outcomes, so
// the errors carry no help page.
func pickAgentConfig(configs map[string]agentcfg.Config, in io.Reader, out io.Writer) (string, error) {
	names := groupedNames(configs)
	for _, row := range pickerRows(configs) {
		fmt.Fprintln(out, row)
	}
	fmt.Fprint(out, "\nSpawn which agent? (number or name): ")

	// A read error leaves the answer empty, so it cancels along with EOF and a
	// bare enter.
	line, _ := bufio.NewReader(in).ReadString('\n')
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errors.New("spawn cancelled")
	}
	if n, err := strconv.Atoi(answer); err == nil {
		if n < 1 || n > len(names) {
			return "", fmt.Errorf("invalid selection %q: want 1-%d or a config name", answer, len(names))
		}
		return names[n-1], nil
	}
	if _, ok := configs[answer]; !ok {
		return "", fmt.Errorf("invalid selection %q: want 1-%d or a config name", answer, len(names))
	}
	return answer, nil
}

// orchestratorConfigNames splits the startup catalog into the two choices
// shown directly and the Pi choices available through the submenu. Managed
// fledge-* profiles remain explicitly spawnable but never appear at startup.
func orchestratorConfigNames(configs map[string]agentcfg.Config) (direct, pi []string) {
	var claude, codex []string
	for name, cfg := range configs {
		if strings.HasPrefix(name, "fledge-") {
			continue
		}
		switch cfg.Integration {
		case "claude":
			claude = append(claude, name)
		case "codex":
			codex = append(codex, name)
		case "pi":
			pi = append(pi, name)
		}
	}
	sort.Strings(claude)
	sort.Strings(codex)
	sort.Slice(pi, func(i, j int) bool {
		a, b := configProvider(configs[pi[i]]), configProvider(configs[pi[j]])
		if a != b {
			return a < b
		}
		return pi[i] < pi[j]
	})
	return append(claude, codex...), pi
}

func orchestratorModel(cfg agentcfg.Config) string {
	if cfg.Model == "" {
		return "(default model)"
	}
	return cfg.Model
}

func orchestratorPickerRows(configs map[string]agentcfg.Config) []string {
	direct, pi := orchestratorConfigNames(configs)
	nameW, count := 0, len(direct)
	if len(pi) > 0 {
		count++
	}
	for _, name := range direct {
		if len(name) > nameW {
			nameW = len(name)
		}
	}
	numW := len(strconv.Itoa(count))
	rows := []string{"Orchestrator profiles:", ""}
	previous := ""
	for i, name := range direct {
		cfg := configs[name]
		label := "Claude Code"
		if cfg.Integration == "codex" {
			label = "Codex"
		}
		if label != previous {
			rows = append(rows, "  "+label)
			previous = label
		}
		rows = append(rows, fmt.Sprintf("    %*d. %-*s   %s", numW, i+1, nameW, name, orchestratorModel(cfg)))
	}
	if len(pi) > 0 {
		rows = append(rows, "  Pi")
		rows = append(rows, fmt.Sprintf("    %*d. Browse Pi profiles…", numW, len(direct)+1))
	}
	return rows
}

func piPickerRows(configs map[string]agentcfg.Config, names []string) []string {
	nameW, numW := 0, len(strconv.Itoa(len(names)))
	for _, name := range names {
		if len(name) > nameW {
			nameW = len(name)
		}
	}
	rows := []string{"Pi profiles:", ""}
	previous := ""
	for i, name := range names {
		cfg := configs[name]
		provider := configProvider(cfg)
		if provider != previous {
			rows = append(rows, fmt.Sprintf("  %s", provider))
			previous = provider
		}
		rows = append(rows, fmt.Sprintf("    %*d. %-*s   %s", numW, i+1, nameW, name, orchestratorModel(cfg)))
	}
	rows = append(rows, "", "  0. Back")
	return rows
}

func readOrchestratorSelection(in *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, _ := in.ReadString('\n')
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", errors.New("spawn cancelled")
	}
	return answer, nil
}

// pickOrchestratorConfig is the startup-only two-level picker. Claude Code and
// Codex stay one keystroke away; Pi's potentially much larger provider catalog
// is browsed separately and can return to the first screen with 0.
func pickOrchestratorConfig(configs map[string]agentcfg.Config, in io.Reader, out io.Writer) (string, error) {
	direct, pi := orchestratorConfigNames(configs)
	if len(direct) == 0 && len(pi) == 0 {
		return "", errors.New("no profiles available for the fledge-orchestrator")
	}

	reader := bufio.NewReader(in)
	for {
		for _, row := range orchestratorPickerRows(configs) {
			fmt.Fprintln(out, row)
		}
		answer, err := readOrchestratorSelection(reader, out, "\nRun orchestrator with which profile? (number or name): ")
		if err != nil {
			return "", err
		}
		if n, parseErr := strconv.Atoi(answer); parseErr == nil {
			if n >= 1 && n <= len(direct) {
				return direct[n-1], nil
			}
			if len(pi) > 0 && n == len(direct)+1 {
				for {
					fmt.Fprintln(out)
					for _, row := range piPickerRows(configs, pi) {
						fmt.Fprintln(out, row)
					}
					piAnswer, readErr := readOrchestratorSelection(reader, out, "\nChoose a Pi profile (number or name): ")
					if readErr != nil {
						return "", readErr
					}
					if piAnswer == "0" {
						fmt.Fprintln(out)
						break
					}
					if piN, piParseErr := strconv.Atoi(piAnswer); piParseErr == nil {
						if piN >= 1 && piN <= len(pi) {
							return pi[piN-1], nil
						}
						return "", fmt.Errorf("invalid selection %q", piAnswer)
					}
					if slices.Contains(pi, piAnswer) {
						return piAnswer, nil
					}
					return "", fmt.Errorf("invalid selection %q", piAnswer)
				}
				continue
			}
			return "", fmt.Errorf("invalid selection %q", answer)
		}
		if slices.Contains(direct, answer) {
			return answer, nil
		}
		if slices.Contains(pi, answer) {
			return answer, nil
		}
		return "", fmt.Errorf("invalid selection %q", answer)
	}
}

// modelEntry is one catalog row as JSON. Provider is the derived one, so the
// claude integration reports anthropic here exactly as it does in the table.
type modelEntry struct {
	Name        string `json:"name"`
	Integration string `json:"integration"`
	Provider    string `json:"provider"`
	Model       string `json:"model,omitempty"`
}

func modelEntries(configs map[string]agentcfg.Config) []modelEntry {
	entries := make([]modelEntry, 0, len(configs))
	for _, name := range groupedNames(configs) {
		c := configs[name]
		entries = append(entries, modelEntry{
			Name:        name,
			Integration: c.Integration,
			Provider:    configProvider(c),
			Model:       c.Model,
		})
	}
	return entries
}
