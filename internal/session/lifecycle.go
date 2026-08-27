package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/project"
)

// Herder is the Herder CLI surface needed to manage Fledge sessions.
type Herder interface {
	List(context.Context) ([]herdr.Session, error)
	Launch(context.Context, string, string) error
	Stop(context.Context, string) error
}

// AgentChoice is the agent to run in a fresh session's orchestrator pane. An
// empty Harness leaves the pane at a shell prompt, and an empty Model accepts
// the harness default.
type AgentChoice struct {
	Harness string
	Model   string
}

// Chooser obtains the agent to run in a fresh session.
type Chooser interface {
	Choose(context.Context) (AgentChoice, error)
}

// Bootstrapper is the Herder surface needed to prepare a fresh session.
type Bootstrapper interface {
	Status(context.Context) (herdr.Status, error)
	Workspaces(context.Context) ([]herdr.Workspace, error)
	Panes(context.Context, string) ([]herdr.Pane, error)
	RenameWorkspace(context.Context, string, string) error
	RenameTab(context.Context, string, string) error
	StartAgent(context.Context, herdr.StartAgentOptions) (herdr.Agent, error)
}

// StartDependencies contains the external operations used by Start.
type StartDependencies struct {
	Herder  Herder
	Entropy io.Reader
	Now     func() time.Time
	Getenv  func(string) string
	Chooser Chooser
	// New discards this project's existing claim before choosing an agent, so
	// the start creates a fresh session instead of reattaching.
	New bool
	// Scoped addresses the Herder server of one session by name.
	Scoped func(sessionName string) Bootstrapper
	// Diagnostics receives the bootstrap report written after Herder exits.
	Diagnostics io.Writer
	// acquireLock is a test seam; production uses the Linux directory lock.
	acquireLock func(context.Context, string) (func() error, error)
	// bootstrapTiming is a test seam; production uses defaultBootstrapTiming.
	bootstrapTiming bootstrapTiming
}

// Start attaches to the sole running session registered by the nearest Fledge
// project, or publishes and launches a fresh session when none is running.
func Start(ctx context.Context, path string, deps StartDependencies) error {
	if deps.Herder == nil {
		return fmt.Errorf("start Fledge session: Herder client is nil")
	}
	if deps.Now == nil {
		return fmt.Errorf("start Fledge session: clock is nil")
	}
	if deps.Getenv == nil {
		return fmt.Errorf("start Fledge session: environment lookup is nil")
	}
	if deps.Getenv("HERDR_ENV") == "1" {
		return fmt.Errorf("start Fledge session: cannot start Herder from inside Herder")
	}

	root, err := project.Find(path)
	if err != nil {
		return fmt.Errorf("start Fledge session: %w", err)
	}
	acquire := deps.acquireLock
	if acquire == nil {
		acquire = acquireProjectLock
	}
	release, err := acquire(ctx, filepath.Join(root, ".fledge"))
	if err != nil {
		return fmt.Errorf("start Fledge session: lock project: %w", err)
	}
	release = cachedRelease(release)
	finish := func(err error) error {
		if releaseErr := release(); releaseErr != nil {
			return errors.Join(err, fmt.Errorf("release project lock: %w", releaseErr))
		}
		return err
	}
	records, err := Load(root)
	if err != nil {
		return finish(fmt.Errorf("start Fledge session: %w", err))
	}
	sessions, err := deps.Herder.List(ctx)
	if err != nil {
		return finish(fmt.Errorf("start Fledge session: list Herder sessions: %w", err))
	}

	running := registeredRunningNames(records, sessions)
	claimed := claimedRecord(records)
	// --new refuses while a registered session is running and otherwise
	// discards any existing claim so a fresh session is chosen below.
	if deps.New {
		if len(running) != 0 {
			return finish(fmt.Errorf("start Fledge session: registered sessions are still running: %s; run \"fledge stop\" first", strings.Join(running, ", ")))
		}
		if claimed != nil {
			if err := Unclaim(*claimed); err != nil {
				return finish(fmt.Errorf("start Fledge session: %w", err))
			}
			claimed = nil
		}
	}
	if claimed != nil {
		for _, name := range running {
			if name != claimed.HerdrSessionName {
				return finish(fmt.Errorf("start Fledge session: claimed session %q conflicts with running registered session %q", claimed.HerdrSessionName, name))
			}
		}
		if contains(running, claimed.HerdrSessionName) {
			if claimed.PendingChoice != nil {
				if err := ClearPending(*claimed); err != nil {
					return finish(fmt.Errorf("start Fledge session: %w", err))
				}
			}
			if err := release(); err != nil {
				return fmt.Errorf("start Fledge session: release project lock: %w", err)
			}
			if err := deps.Herder.Launch(ctx, root, claimed.HerdrSessionName); err != nil {
				return fmt.Errorf("start Fledge session: launch %q: %w", claimed.HerdrSessionName, err)
			}
			return nil
		}
		return startClaimed(ctx, root, *claimed, deps, release)
	}

	switch len(running) {
	case 0:
		maxNameLength, err := initialNameLimit(sessions)
		if err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		if deps.Chooser == nil {
			return finish(fmt.Errorf("start Fledge session: chooser is nil"))
		}
		if deps.Scoped == nil {
			return finish(fmt.Errorf("start Fledge session: scoped Herder client is nil"))
		}
		if deps.Diagnostics == nil {
			return finish(fmt.Errorf("start Fledge session: diagnostics is nil"))
		}
		unavailable := make(map[string]struct{}, len(sessions))
		for _, listed := range sessions {
			unavailable[listed.Name] = struct{}{}
		}
		// The choice is made before the record so a dismissed picker leaves
		// nothing behind.
		choice, err := deps.Chooser.Choose(ctx)
		if err != nil {
			return finish(fmt.Errorf("start Fledge session: choose agent: %w", err))
		}
		record, err := Create(root, choice, maxNameLength, unavailable, deps.Entropy, deps.Now())
		if err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		return startClaimed(ctx, root, record, deps, release)
	case 1:
		record := recordByName(records, running[0])
		if err := Claim(record); err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		if err := release(); err != nil {
			return fmt.Errorf("start Fledge session: release project lock: %w", err)
		}
		if err := deps.Herder.Launch(ctx, root, running[0]); err != nil {
			return fmt.Errorf("start Fledge session: launch %q: %w", running[0], err)
		}
		return nil
	default:
		return finish(fmt.Errorf("start Fledge session: multiple registered sessions are running: %s", strings.Join(running, ", ")))
	}
}

func claimedRecord(records []Record) *Record {
	for i := range records {
		if records[i].Claimed {
			return &records[i]
		}
	}
	return nil
}

func recordByName(records []Record, name string) Record {
	for _, record := range records {
		if record.HerdrSessionName == name {
			return record
		}
	}
	return Record{}
}

func initialNameLimit(sessions []herdr.Session) (int, error) {
	var defaults []herdr.Session
	for _, session := range sessions {
		if session.Default {
			defaults = append(defaults, session)
		}
	}
	if len(defaults) != 1 {
		return 0, fmt.Errorf("expected exactly one default Herder session, got %d", len(defaults))
	}
	dir := defaults[0].SessionDir
	if dir == "" || strings.IndexByte(dir, 0) >= 0 || !filepath.IsAbs(dir) {
		return 0, fmt.Errorf("default Herder session has invalid session_dir")
	}
	base := dir
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	base += "sessions"
	limit := min(maxSessionLength, 103-len(base)-2-len("herdr.sock"), 103-len(base)-2-len("herdr-client.sock"))
	if limit < minSessionLength {
		return 0, fmt.Errorf("default Herder session directory leaves no usable session-name capacity")
	}
	return limit, nil
}

func min(values ...int) int {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func startClaimed(ctx context.Context, root string, record Record, deps StartDependencies, release func() error) error {
	if record.PendingChoice != nil && (deps.Scoped == nil || deps.Diagnostics == nil) {
		return joinStartRelease(fmt.Errorf("start Fledge session: claimed pending session requires bootstrap dependencies"), release)
	}
	watchCtx, cancelWatch := context.WithCancelCause(ctx)
	bootCtx, cancelBoot := context.WithCancelCause(ctx)
	var watcher sync.WaitGroup
	watcher.Add(1)
	watchErr := make(chan error, 1)
	go func() {
		defer watcher.Done()
		watchErr <- watchClaimedRunning(watchCtx, deps.Herder, record, release)
	}()

	var bootDone chan error
	var logPath string
	if record.PendingChoice != nil {
		logPath = filepath.Join(record.Path, bootstrapLogName)
		log := io.Writer(io.Discard)
		file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(deps.Diagnostics, "fledge: cannot write %s: %v\n", logPath, err)
		} else {
			defer file.Close()
			log = file
		}
		bootDone = make(chan error, 1)
		choice := *record.PendingChoice
		timing := deps.bootstrapTiming
		if timing == (bootstrapTiming{}) {
			timing = defaultBootstrapTiming
		}
		go func() {
			bootDone <- bootstrap(bootCtx, deps.Scoped(record.HerdrSessionName), bootstrapInput{Root: root, Choice: choice, Log: log}, timing)
		}()
	}
	launchErr := deps.Herder.Launch(ctx, root, record.HerdrSessionName)
	var stateErr error
	stateDoneBeforeLaunchReturn := false
	select {
	case stateErr = <-watchErr:
		stateDoneBeforeLaunchReturn = true
	default:
	}
	var bootErr error
	bootDoneBeforeLaunchReturn := false
	if bootDone != nil {
		select {
		case bootErr = <-bootDone:
			bootDoneBeforeLaunchReturn = true
		default:
		}
	}
	cancelWatch(errLaunchReturned)
	cancelBoot(errLaunchReturned)
	watcher.Wait()
	if !stateDoneBeforeLaunchReturn {
		stateErr = <-watchErr
	}
	if bootDone != nil && !bootDoneBeforeLaunchReturn {
		bootErr = <-bootDone
	}
	if canceledByLaunchReturn(stateErr, watchCtx) {
		stateErr = nil
	}
	if !bootDoneBeforeLaunchReturn && canceledByLaunchReturn(bootErr, bootCtx) {
		bootErr = nil
	}
	if bootErr != nil {
		fmt.Fprintf(deps.Diagnostics, "fledge: session bootstrap failed (see %s): %v\n", logPath, bootErr)
	}
	return joinStartRelease(errors.Join(wrapLaunch(record.HerdrSessionName, launchErr), wrapBootstrap(bootErr), wrapState(stateErr)), release)
}

func watchClaimedRunning(ctx context.Context, h Herder, record Record, release func() error) error {
	for {
		sessions, err := h.List(ctx)
		if err == nil {
			for _, listed := range sessions {
				if listed.Name == record.HerdrSessionName && listed.Running {
					if record.PendingChoice != nil {
						if err := ClearPending(record); err != nil {
							return err
						}
					}
					// The lock is no longer needed once Herder has published the
					// exact claimed session. Its result is collected in Start's
					// final release slot so it is reported exactly once.
					release()
					return nil
				}
			}
		}
		if err := sleep(ctx, 200*time.Millisecond); err != nil {
			return err
		}
	}
}

var errLaunchReturned = errors.New("Herder launch returned")

func canceledByLaunchReturn(err error, ctx context.Context) bool {
	return errors.Is(err, context.Canceled) && abortedByLaunchReturn(ctx)
}

func abortedByLaunchReturn(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errLaunchReturned)
}

func cachedRelease(release func() error) func() error {
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() { result = release() })
		return result
	}
}

func wrapLaunch(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("start Fledge session: launch %q: %w", name, err)
}
func wrapBootstrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("start Fledge session: bootstrap: %w", err)
}
func wrapState(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("start Fledge session: watcher: %w", err)
}
func joinStartRelease(err error, release func() error) error {
	if releaseErr := release(); releaseErr != nil {
		return errors.Join(err, fmt.Errorf("start Fledge session: release project lock: %w", releaseErr))
	}
	return err
}

// Confirmer obtains approval to stop an immutable session snapshot.
type Confirmer interface {
	Confirm(projectRoot string, names []string, selfStop bool) (bool, error)
}

// StopDependencies contains the external operations used by Stop.
type StopDependencies struct {
	Herder    Herder
	Confirmer Confirmer
	Output    io.Writer
	Getenv    func(string) string
}

// Stop confirms and stops all running sessions registered by the nearest
// Fledge project. Local records are retained.
func Stop(ctx context.Context, path string, deps StopDependencies) error {
	if deps.Herder == nil {
		return fmt.Errorf("stop Fledge sessions: Herder client is nil")
	}
	if deps.Output == nil {
		return fmt.Errorf("stop Fledge sessions: output is nil")
	}
	if deps.Getenv == nil {
		return fmt.Errorf("stop Fledge sessions: environment lookup is nil")
	}

	root, err := project.Find(path)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: %w", err)
	}
	records, err := Load(root)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: %w", err)
	}
	sessions, err := deps.Herder.List(ctx)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: list Herder sessions: %w", err)
	}

	targets := registeredRunningNames(records, sessions)
	if len(targets) == 0 {
		if _, err := fmt.Fprintf(deps.Output, "No running Fledge sessions for %q.\n", root); err != nil {
			return fmt.Errorf("stop Fledge sessions: report status: %w", err)
		}
		return nil
	}
	if deps.Confirmer == nil {
		return fmt.Errorf("stop Fledge sessions: confirmer is nil")
	}

	current := deps.Getenv("HERDR_SESSION")
	selfStop := contains(targets, current)
	confirmed, err := deps.Confirmer.Confirm(root, append([]string(nil), targets...), selfStop)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: confirm: %w", err)
	}
	if !confirmed {
		return nil
	}

	stopOrder := append([]string(nil), targets...)
	if selfStop {
		stopOrder = moveLast(stopOrder, current)
	}
	var failures []error
	for _, name := range stopOrder {
		if err := deps.Herder.Stop(ctx, name); err != nil {
			failures = append(failures, fmt.Errorf("stop %q: %w", name, err))
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("stop Fledge sessions: %w", errors.Join(failures...))
	}
	return nil
}

func registeredRunningNames(records []Record, sessions []herdr.Session) []string {
	registered := make(map[string]struct{}, len(records))
	for _, record := range records {
		registered[record.HerdrSessionName] = struct{}{}
	}

	running := make(map[string]struct{})
	for _, listed := range sessions {
		if !listed.Running {
			continue
		}
		if _, ok := registered[listed.Name]; ok {
			running[listed.Name] = struct{}{}
		}
	}

	names := make([]string, 0, len(running))
	for name := range running {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func contains(names []string, target string) bool {
	if target == "" {
		return false
	}
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func moveLast(names []string, target string) []string {
	for i, name := range names {
		if name != target {
			continue
		}
		copy(names[i:], names[i+1:])
		names[len(names)-1] = target
		break
	}
	return names
}
