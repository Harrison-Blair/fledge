package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"fledge/internal/herdr"
	"fledge/internal/project"
	"fledge/internal/session/bootstrap"
	"fledge/internal/session/lock"
	"fledge/internal/session/record"
	"fledge/internal/session/types"
)

// Herder is the Herder CLI surface needed to manage Fledge sessions.
type Herder interface {
	List(context.Context) ([]herdr.Session, error)
	Launch(context.Context, string, string) error
	Stop(context.Context, string) error
}

// AgentChoice is the agent to run in a fresh session's orchestrator pane.
type AgentChoice = types.AgentChoice

// Chooser obtains the agent to run in a fresh session.
type Chooser interface {
	Choose(context.Context) (AgentChoice, error)
}

// Bootstrapper is the Herder surface needed to prepare a fresh session.
type Bootstrapper = bootstrap.Server

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
	// bootstrapTiming is a test seam; production uses bootstrap.DefaultTiming.
	bootstrapTiming bootstrap.Timing
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
		acquire = lock.Acquire
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
	records, err := record.Load(root)
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
			if err := record.Unclaim(*claimed); err != nil {
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
			before, err := record.ReadStopIntent(*claimed)
			if err != nil {
				return finish(fmt.Errorf("start Fledge session: %w", err))
			}
			if claimed.PendingChoice != nil {
				if err := record.ClearPending(*claimed); err != nil {
					return finish(fmt.Errorf("start Fledge session: %w", err))
				}
			}
			if err := release(); err != nil {
				return fmt.Errorf("start Fledge session: release project lock: %w", err)
			}
			launchErr := deps.Herder.Launch(ctx, root, claimed.HerdrSessionName)
			return finishAttachedLaunch(ctx, root, *claimed, deps, before, launchErr)
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
		rec, err := record.Create(root, choice, maxNameLength, unavailable, deps.Entropy, deps.Now())
		if err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		return startClaimed(ctx, root, rec, deps, release)
	case 1:
		rec := recordByName(records, running[0])
		before, err := record.ReadStopIntent(rec)
		if err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		if err := record.Claim(rec); err != nil {
			return finish(fmt.Errorf("start Fledge session: %w", err))
		}
		if err := release(); err != nil {
			return fmt.Errorf("start Fledge session: release project lock: %w", err)
		}
		launchErr := deps.Herder.Launch(ctx, root, running[0])
		return finishAttachedLaunch(ctx, root, rec, deps, before, launchErr)
	default:
		return finish(fmt.Errorf("start Fledge session: multiple registered sessions are running: %s", strings.Join(running, ", ")))
	}
}

func claimedRecord(records []record.Record) *record.Record {
	for i := range records {
		if records[i].Claimed {
			return &records[i]
		}
	}
	return nil
}

func recordByName(records []record.Record, name string) record.Record {
	for _, rec := range records {
		if rec.HerdrSessionName == name {
			return rec
		}
	}
	return record.Record{}
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
	limit := min(record.MaxSessionLength, 103-len(base)-2-len("herdr.sock"), 103-len(base)-2-len("herdr-client.sock"))
	if limit < record.MinSessionLength {
		return 0, fmt.Errorf("default Herder session directory leaves no usable session-name capacity")
	}
	return limit, nil
}

func startClaimed(ctx context.Context, root string, rec record.Record, deps StartDependencies, release func() error) error {
	if rec.PendingChoice != nil && (deps.Scoped == nil || deps.Diagnostics == nil) {
		return joinStartRelease(fmt.Errorf("start Fledge session: claimed pending session requires bootstrap dependencies"), release)
	}
	before, err := record.ReadStopIntent(rec)
	if err != nil {
		return joinStartRelease(fmt.Errorf("start Fledge session: %w", err), release)
	}
	watchCtx, cancelWatch := context.WithCancelCause(ctx)
	bootCtx, cancelBoot := context.WithCancelCause(ctx)
	var watcher sync.WaitGroup
	watcher.Add(1)
	watchErr := make(chan error, 1)
	go func() {
		defer watcher.Done()
		watchErr <- watchClaimedRunning(watchCtx, deps.Herder, rec, release)
	}()

	var bootDone chan error
	var logPath string
	if rec.PendingChoice != nil {
		logPath = filepath.Join(rec.Path, bootstrap.LogName)
		log := io.Writer(io.Discard)
		file, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(deps.Diagnostics, "fledge: cannot write %s: %v\n", logPath, err)
		} else {
			defer file.Close()
			log = file
		}
		bootDone = make(chan error, 1)
		choice := *rec.PendingChoice
		timing := deps.bootstrapTiming
		if timing == (bootstrap.Timing{}) {
			timing = bootstrap.DefaultTiming
		}
		go func() {
			bootDone <- bootstrap.Run(bootCtx, deps.Scoped(rec.HerdrSessionName), bootstrap.Input{Root: root, Choice: choice, Log: log}, timing)
		}()
	}
	launchErr := deps.Herder.Launch(ctx, root, rec.HerdrSessionName)
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
	releaseErr := release()
	var classifyErr error
	if launchErr != nil && releaseErr == nil {
		var intentional bool
		intentional, classifyErr = classifyIntentionalStop(ctx, root, rec, deps, before)
		if intentional {
			launchErr = nil
		}
	}
	result := errors.Join(wrapLaunch(rec.HerdrSessionName, launchErr), wrapBootstrap(bootErr), wrapState(stateErr), classifyErr)
	if releaseErr != nil {
		result = errors.Join(result, fmt.Errorf("start Fledge session: release project lock: %w", releaseErr))
	}
	return result
}

func finishAttachedLaunch(ctx context.Context, root string, rec record.Record, deps StartDependencies, before record.StopIntent, launchErr error) error {
	if launchErr == nil {
		return nil
	}
	intentional, classifyErr := classifyIntentionalStop(ctx, root, rec, deps, before)
	if intentional {
		launchErr = nil
	}
	return errors.Join(wrapLaunch(rec.HerdrSessionName, launchErr), classifyErr)
}

func classifyIntentionalStop(ctx context.Context, root string, rec record.Record, deps StartDependencies, before record.StopIntent) (bool, error) {
	if ctx.Err() != nil {
		return false, nil
	}
	acquire := deps.acquireLock
	if acquire == nil {
		acquire = lock.Acquire
	}
	release, err := acquire(ctx, filepath.Join(root, ".fledge"))
	if err != nil {
		return false, fmt.Errorf("start Fledge session: inspect intentional stop for %q: lock project: %w", rec.HerdrSessionName, err)
	}
	finish := func(result bool, operationErr error) (bool, error) {
		if releaseErr := release(); releaseErr != nil {
			operationErr = errors.Join(operationErr, fmt.Errorf("start Fledge session: inspect intentional stop for %q: release project lock: %w", rec.HerdrSessionName, releaseErr))
		}
		if result && ctx.Err() != nil {
			result = false
		}
		return result && operationErr == nil, operationErr
	}
	after, err := record.ReadStopIntent(rec)
	if err != nil {
		return finish(false, fmt.Errorf("start Fledge session: inspect intentional stop for %q: %w", rec.HerdrSessionName, err))
	}
	if !after.Exists || (before.Exists && before.ID == after.ID) {
		return finish(false, nil)
	}
	sessions, err := deps.Herder.List(ctx)
	if err != nil {
		return finish(false, fmt.Errorf("start Fledge session: inspect intentional stop for %q: list Herder sessions: %w", rec.HerdrSessionName, err))
	}
	for _, listed := range sessions {
		if listed.Name == rec.HerdrSessionName && listed.Running {
			return finish(false, nil)
		}
	}
	if ctx.Err() != nil {
		return finish(false, nil)
	}
	return finish(true, nil)
}

func watchClaimedRunning(ctx context.Context, h Herder, rec record.Record, release func() error) error {
	for {
		sessions, err := h.List(ctx)
		if err == nil {
			for _, listed := range sessions {
				if listed.Name == rec.HerdrSessionName && listed.Running {
					if rec.PendingChoice != nil {
						if err := record.ClearPending(rec); err != nil {
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
	// Scoped addresses the Herder server of one session by name.
	Scoped  func(sessionName string) PaneResolver
	Entropy io.Reader
	// acquireLock is a test seam; production uses the Linux directory lock.
	acquireLock func(context.Context, string) (func() error, error)
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
	records, err := record.Load(root)
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

	current := ""
	selfStop := false
	if deps.Getenv("HERDR_ENV") == "1" {
		validated, _, err := ValidateAmbientPane(ctx, deps.Getenv, targets, deps.Scoped)
		if err != nil {
			return fmt.Errorf("stop Fledge sessions: %w", err)
		}
		current = validated
		selfStop = true
	}
	confirmed, err := deps.Confirmer.Confirm(root, append([]string(nil), targets...), selfStop)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: confirm: %w", err)
	}
	if !confirmed {
		return nil
	}
	intentID, err := record.GenerateStopIntent(deps.Entropy)
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: %w", err)
	}
	acquire := deps.acquireLock
	if acquire == nil {
		acquire = lock.Acquire
	}
	release, err := acquire(ctx, filepath.Join(root, ".fledge"))
	if err != nil {
		return fmt.Errorf("stop Fledge sessions: lock project: %w", err)
	}

	stopOrder := append([]string(nil), targets...)
	if selfStop {
		stopOrder = moveLast(stopOrder, current)
	}
	var failures []error
	for _, name := range stopOrder {
		rec := recordByName(records, name)
		previous, err := record.ReadStopIntent(rec)
		if err != nil {
			failures = append(failures, fmt.Errorf("stop %q: %w", name, err))
			continue
		}
		if err := record.WriteStopIntent(rec, intentID); err != nil {
			failures = append(failures, fmt.Errorf("stop %q: %w", name, err))
			continue
		}
		if err := deps.Herder.Stop(ctx, name); err != nil {
			restoreErr := record.RestoreStopIntent(rec, previous)
			failures = append(failures, fmt.Errorf("stop %q: %w", name, errors.Join(err, restoreErr)))
		}
	}
	if releaseErr := release(); releaseErr != nil {
		failures = append(failures, fmt.Errorf("release project lock: %w", releaseErr))
	}
	if len(failures) != 0 {
		return fmt.Errorf("stop Fledge sessions: %w", errors.Join(failures...))
	}
	return nil
}

func registeredRunningNames(records []record.Record, sessions []herdr.Session) []string {
	registered := make(map[string]struct{}, len(records))
	for _, rec := range records {
		registered[rec.HerdrSessionName] = struct{}{}
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
	return target != "" && slices.Contains(names, target)
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

// sleep waits for d, reporting why the context ended when it ends first.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
