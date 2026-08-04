package watch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	orchestratorAgent = "orchestrator"

	// maxEventFailures is how many consecutive subscription failures the
	// engine tolerates before it gives up on the event stream and polls for
	// the rest of the process's life.
	maxEventFailures = 3

	// minIntervalSeconds keeps a hand-edited zero in watch.json from turning
	// the loop into a hot spin. The config layer honours an explicit zero; the
	// engine is what refuses to act on it.
	minIntervalSeconds = 1
)

// ErrCorruptLog reports that the durable wake ledger can no longer be trusted.
// The ledger implementation is expected to map its own corruption sentinel onto
// this one so the engine can degrade instead of crash-looping.
var ErrCorruptLog = errors.New("corrupt wake ledger")

// WakeKind classifies why the watcher wants to wake the orchestrator.
type WakeKind string

const (
	KindStatus    WakeKind = "status"
	KindEvent     WakeKind = "event"
	KindDead      WakeKind = "dead"
	KindHeartbeat WakeKind = "heartbeat"
)

// WakeRecord is one wake still owed to the orchestrator. Wakes for the same
// kind and key collapse into a single record carrying the latest reason, so
// IDs holds every ledger entry the record speaks for.
type WakeRecord struct {
	ID     string
	IDs    []string
	Kind   WakeKind
	Key    string
	Reason string
}

// StatusSeen records how far the watcher has consumed one worker's status file.
type StatusSeen struct {
	Size      int64
	MtimeUnix int64
	Offset    int64
}

// Markers is the watcher's suppression state: what it has already seen and
// already woken for.
type Markers struct {
	StatusSeen      map[string]StatusSeen
	EventEscalated  map[string]bool
	DoneGrace       map[string]int64
	KnownAgents     []string
	LastWakeUnix    int64
	HeartbeatStreak int
}

// Ledger is the durable wake queue the engine appends to before it advances
// any suppression marker.
type Ledger interface {
	Append(kind WakeKind, key, reason string) (WakeRecord, error)
	Pending() ([]WakeRecord, error)
	MarkDelivered(ids []string, messageID string) error
	Compact() error
	LoadMarkers() (Markers, error)
	SaveMarkers(markers Markers) error
}

// Waker delivers one batched wake body to the orchestrator and returns the
// identifier of the message it created.
type Waker interface {
	Deliver(ctx context.Context, body string) (messageID string, err error)
}

// CompletionLog answers whether a worker has already told the orchestrator it
// finished, which is what distinguishes a normal finish from a swallowed one.
type CompletionLog interface {
	CompletionSince(worker string, since time.Time) (bool, error)
}

// Herdr is the slice of the Herdr CLI the engine needs.
type Herdr interface {
	List(ctx context.Context) ([]herdr.Session, error)
	Snapshot(ctx context.Context, session string) (herdr.Snapshot, error)
}

// Subscriber streams agent status events for panes until ctx ends or the
// stream breaks, calling onReady once the subscription is established. The
// engine always passes a ctx bounded by the poll budget: a wedged Herdr sends
// neither events nor EOF, so only that deadline ends the wait.
type Subscriber func(ctx context.Context, paneIDs []string, onReady func(), onEvent func(Event)) error

// Engine is the watcher's supervision loop. Every dependency is injected so
// the loop can be driven deterministically in tests.
type Engine struct {
	Root        string
	Session     string
	Config      Config
	Herdr       Herdr
	Ledger      Ledger
	Waker       Waker
	Completions CompletionLog
	Subscriber  Subscriber
	Now         func() time.Time
	Sleep       func(ctx context.Context, d time.Duration)
	Timeout     func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	Log         func(message string)

	started        time.Time
	terminal       map[string]bool
	queue          []WakeRecord
	degraded       bool
	failures       int
	eventsDisabled bool
}

// supervised is one worker the engine is watching this cycle.
type supervised struct {
	Name        string
	PaneID      string
	AgentStatus string
}

// pendingWake is a wake the current cycle decided on but has not queued yet.
type pendingWake struct {
	kind   WakeKind
	key    string
	reason string
}

// Run supervises the session until it disappears or ctx ends.
func (e *Engine) Run(ctx context.Context) error {
	for {
		alive, err := e.cycle(ctx)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
	}
}

// cycle runs one supervision pass and its wait. It reports whether the watcher
// should keep going; a stopped or deleted session ends the loop cleanly.
func (e *Engine) cycle(ctx context.Context) (bool, error) {
	e.init()

	if err := ctx.Err(); err != nil {
		return false, err
	}

	sessions, err := e.Herdr.List(ctx)
	if err != nil {
		e.logf("list Herdr sessions: %v", err)
		e.wait(ctx, e.pollInterval())
		return true, ctx.Err()
	}
	if !sessionRunning(sessions, e.Session) {
		e.logf("session %s is gone; watcher exiting", e.Session)
		return false, nil
	}

	snapshot, err := e.Herdr.Snapshot(ctx, e.Session)
	if err != nil {
		e.logf("snapshot session %s: %v", e.Session, err)
		e.wait(ctx, e.pollInterval())
		return true, ctx.Err()
	}

	markers := e.loadMarkers()
	beforeObservations := cloneMarkers(markers)
	workers := namedWorkers(snapshot)
	trustDepartures := snapshotHasOrchestrator(snapshot)
	if !trustDepartures {
		e.logf("suspect snapshot for %s: named orchestrator is missing; retaining known agents and skipping departures", e.Session)
	}

	var wakes []pendingWake
	wakes = append(wakes, e.scanStatus(ctx, workers, &markers)...)
	wakes = append(wakes, e.resolveDoneGrace(&markers)...)
	if trustDepartures {
		wakes = append(wakes, e.detectDeparted(workers, &markers)...)
	}
	wakes = append(wakes, e.heartbeat(markers, len(workers))...)

	// The durable queue is written before any marker advances: a crash between
	// the two costs a replayed wake, never a lost one.
	queued := e.enqueueAll(ctx, wakes)
	if !queued {
		markers = beforeObservations
	}
	if queued {
		e.drain(ctx, &markers)
	}
	e.saveMarkers(markers)

	switch {
	case len(workers) == 0:
		e.logf("no workers in %s; watching idle", e.Session)
		e.wait(ctx, e.idleInterval())
	case e.eventsActive():
		e.watchEvents(ctx, workers, &markers)
		e.saveMarkers(markers)
	default:
		e.wait(ctx, e.pollInterval())
	}

	return true, ctx.Err()
}

func (e *Engine) init() {
	if e.terminal == nil {
		e.terminal = make(map[string]bool)
	}
	if e.started.IsZero() {
		e.started = e.Now()
	}
	if e.Timeout == nil {
		e.Timeout = context.WithTimeout
	}
}

// --- detection ----------------------------------------------------------

// scanStatus reads whatever each worker appended since the last cycle. A
// changed file is given the signal grace to finish being written so a
// multi-line append is classified as one batch rather than torn in half.
func (e *Engine) scanStatus(ctx context.Context, workers []supervised, markers *Markers) []pendingWake {
	if !e.statusChanged(workers, *markers) {
		return nil
	}
	e.wait(ctx, time.Duration(e.Config.SignalGraceSeconds)*time.Second)

	var wakes []pendingWake
	for _, worker := range workers {
		wakes = append(wakes, e.readStatus(worker.Name, markers)...)
	}
	return wakes
}

func (e *Engine) statusChanged(workers []supervised, markers Markers) bool {
	for _, worker := range workers {
		info, err := os.Stat(e.statusPath(worker.Name))
		if err != nil {
			continue
		}
		seen := markers.StatusSeen[worker.Name]
		if info.Size() != seen.Size || info.ModTime().Unix() != seen.MtimeUnix {
			return true
		}
	}
	return false
}

func (e *Engine) readStatus(name string, markers *Markers) []pendingWake {
	path := e.statusPath(name)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	seen := markers.StatusSeen[name]
	offset := seen.Offset
	if info.Size() < offset {
		// The file was replaced or truncated; start over rather than skip.
		offset = 0
	}

	lines, consumed := e.readNewLines(path, offset)
	markers.StatusSeen[name] = StatusSeen{Size: info.Size(), MtimeUnix: info.ModTime().Unix(), Offset: offset + consumed}

	var wakes []pendingWake
	for _, line := range lines {
		verb, detail, ok := ParseStatusLine(line)
		if !ok {
			continue
		}

		switch ClassifyStatus(verb) {
		case ActionWake:
			delete(markers.DoneGrace, name)
			e.terminal[name] = verb == verbFailed
			wakes = append(wakes, pendingWake{kind: KindStatus, key: name, reason: statusReason(name, verb, detail)})
		case ActionAbsorb:
			delete(markers.DoneGrace, name)
			e.logf("absorbed: %s", statusReason(name, verb, detail))
		case ActionWakeAfterGrace:
			e.terminal[name] = true
			if _, waiting := markers.DoneGrace[name]; !waiting {
				markers.DoneGrace[name] = e.Now().Unix()
				e.logf("%s reported done; holding for the completion message", name)
			}
		}
	}

	return wakes
}

// readNewLines returns the complete lines starting at offset and how many
// bytes they occupied. A trailing partial line is left for the next cycle.
func (e *Engine) readNewLines(path string, offset int64) ([]string, int64) {
	file, err := os.Open(path)
	if err != nil {
		e.logf("open status file %s: %v", path, err)
		return nil, 0
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		e.logf("seek status file %s: %v", path, err)
		return nil, 0
	}

	contents, err := io.ReadAll(file)
	if err != nil {
		e.logf("read status file %s: %v", path, err)
		return nil, 0
	}

	end := bytes.LastIndexByte(contents, '\n') + 1
	if end == 0 {
		return nil, 0
	}

	complete := contents[:end]
	if offset == 0 {
		complete = bytes.TrimPrefix(complete, []byte{0xef, 0xbb, 0xbf})
	}
	return strings.Split(strings.TrimSuffix(string(complete), "\n"), "\n"), int64(end)
}

// resolveDoneGrace decides the swallowed-finish question once a worker's done
// report has had its grace period: the orchestrator is woken only when no
// completion message from that worker ever reached it.
func (e *Engine) resolveDoneGrace(markers *Markers) []pendingWake {
	grace := time.Duration(e.Config.DoneMessageGraceSeconds) * time.Second

	var wakes []pendingWake
	for _, name := range sortedKeys(markers.DoneGrace) {
		doneAt := time.Unix(markers.DoneGrace[name], 0).UTC()
		if e.Now().Sub(doneAt) < grace {
			continue
		}

		// The window reaches back as far as it reaches forward: a worker that
		// messages the orchestrator and then writes its done line sent that
		// message before the timestamp being graced.
		completed, err := e.Completions.CompletionSince(name, doneAt.Add(-grace))
		if err != nil {
			e.logf("read completion log for %s: %v", name, err)
			continue
		}

		delete(markers.DoneGrace, name)
		if completed {
			e.logf("absorbed: %s finished and its completion message arrived", name)
			continue
		}
		wakes = append(wakes, pendingWake{
			kind:   KindStatus,
			key:    name,
			reason: fmt.Sprintf("%s reported done but no completion message reached you", name),
		})
	}

	return wakes
}

// detectDeparted wakes once for a worker that left the session without either
// reporting a terminal verb or messaging the orchestrator.
func (e *Engine) detectDeparted(workers []supervised, markers *Markers) []pendingWake {
	present := make(map[string]bool, len(workers))
	names := make([]string, 0, len(workers))
	for _, worker := range workers {
		present[worker.Name] = true
		names = append(names, worker.Name)
	}

	var wakes []pendingWake
	for _, name := range markers.KnownAgents {
		if present[name] {
			continue
		}

		delete(markers.StatusSeen, name)
		if e.terminal[name] {
			continue
		}
		if _, waiting := markers.DoneGrace[name]; waiting {
			continue
		}

		lookback := time.Duration(e.Config.DoneMessageGraceSeconds) * time.Second
		completed, err := e.Completions.CompletionSince(name, e.started.Add(-lookback))
		if err != nil {
			e.logf("read completion log for %s: %v", name, err)
			continue
		}
		if completed {
			continue
		}

		wakes = append(wakes, pendingWake{
			kind:   KindDead,
			key:    name,
			reason: fmt.Sprintf("%s vanished from the session without reporting done or messaging you", name),
		})
	}

	sort.Strings(names)
	markers.KnownAgents = names

	return wakes
}

// heartbeat proves the watcher is alive during long quiet stretches, backing
// off so a silent session does not accumulate noise.
func (e *Engine) heartbeat(markers Markers, workers int) []pendingWake {
	interval := heartbeatInterval(e.Config, markers.HeartbeatStreak)
	if interval <= 0 {
		return nil
	}

	last := e.started
	if markers.LastWakeUnix != 0 {
		last = time.Unix(markers.LastWakeUnix, 0).UTC()
	}
	if e.Now().Sub(last) < interval {
		return nil
	}

	return []pendingWake{{
		kind:   KindHeartbeat,
		key:    "heartbeat",
		reason: fmt.Sprintf("watcher heartbeat: %d worker(s) supervised, nothing actionable", workers),
	}}
}

func heartbeatInterval(config Config, streak int) time.Duration {
	seconds := config.HeartbeatSeconds
	if seconds <= 0 {
		return 0
	}

	limit := config.HeartbeatMaxSeconds
	for range streak {
		if limit > 0 && seconds >= limit {
			break
		}
		seconds *= 2
	}
	if limit > 0 && seconds > limit {
		seconds = limit
	}

	return time.Duration(seconds) * time.Second
}

// --- event stream -------------------------------------------------------

// watchEvents spends the rest of the cycle on the push stream, so a worker
// that blocks is escalated in well under one poll interval.
func (e *Engine) watchEvents(ctx context.Context, workers []supervised, markers *Markers) {
	budget := e.pollInterval()

	names := make(map[string]string, len(workers))
	paneIDs := make([]string, 0, len(workers))
	for _, worker := range workers {
		if worker.PaneID == "" {
			continue
		}
		paneIDs = append(paneIDs, worker.PaneID)
		names[worker.PaneID] = worker.Name
	}
	if len(paneIDs) == 0 {
		e.wait(ctx, budget)
		return
	}

	started := e.Now()
	streamCtx, cancel := e.Timeout(ctx, budget)
	defer cancel()

	ready := false
	err := e.Subscriber(
		streamCtx,
		paneIDs,
		func() {
			ready = true
			e.reconcile(ctx, markers)
		},
		func(event Event) { e.handleEvent(ctx, event, markers, names) },
	)

	switch {
	case ctx.Err() != nil:
		return
	case ready && errors.Is(err, context.DeadlineExceeded):
		// The budget elapsed with the stream healthy: a clean end of cycle.
		e.failures = 0
		return
	}

	e.failures++
	e.logf("event stream failed (%d/%d): %v", e.failures, maxEventFailures, err)
	if e.failures >= maxEventFailures {
		e.eventsDisabled = true
		e.logf("event stream disabled for this process; falling back to polling")
	}

	// Serve out the rest of the budget so a refused socket cannot spin.
	if remaining := budget - e.Now().Sub(started); remaining > 0 {
		e.wait(ctx, remaining)
	}
}

// reconcile classifies every worker's current status right after the
// subscription is acknowledged, which is what catches a worker that was
// already blocked before the watcher started listening.
func (e *Engine) reconcile(ctx context.Context, markers *Markers) {
	snapshot, err := e.Herdr.Snapshot(ctx, e.Session)
	if err != nil {
		e.logf("reconcile snapshot: %v", err)
		return
	}

	for _, worker := range namedWorkers(snapshot) {
		e.applyTransition(ctx, worker.PaneID, worker.Name, worker.AgentStatus, markers)
	}
}

func (e *Engine) handleEvent(ctx context.Context, event Event, markers *Markers, names map[string]string) {
	name := names[event.PaneID]
	if name == "" {
		name = event.Agent
	}
	e.applyTransition(ctx, event.PaneID, name, event.AgentStatus, markers)
}

func (e *Engine) applyTransition(ctx context.Context, paneID, name, status string, markers *Markers) {
	switch ClassifyTransition(status) {
	case TransitionWake:
		if markers.EventEscalated[paneID] {
			return
		}
		markers.EventEscalated[paneID] = true
		if !e.enqueue(ctx, pendingWake{
			kind:   KindEvent,
			key:    paneID,
			reason: fmt.Sprintf("%s is blocked in pane %s", name, paneID),
		}) {
			delete(markers.EventEscalated, paneID)
			return
		}
		e.drain(ctx, markers)
	case TransitionClear:
		if markers.EventEscalated[paneID] {
			delete(markers.EventEscalated, paneID)
			e.logf("pane %s is working again; escalation re-armed", paneID)
		}
	}
}

// --- queue and delivery -------------------------------------------------

func (e *Engine) enqueueAll(ctx context.Context, wakes []pendingWake) bool {
	durable := true
	for _, wake := range wakes {
		if !e.enqueue(ctx, wake) {
			durable = false
		}
	}
	return durable
}

// enqueue queues one wake and records it in the decision log. Compact empties
// the ledger of everything already delivered, so the decision log — not
// ledger.jsonl — is the durable audit trail; every wake is logged with the
// ledger ID it was queued under so it can be paired with its delivery.
func (e *Engine) enqueue(ctx context.Context, wake pendingWake) bool {
	if e.degraded {
		e.rememberAndLog(wake)
		return true
	}

	record, err := e.Ledger.Append(wake.kind, wake.key, wake.reason)
	if err != nil {
		e.noteLedgerError(ctx, err)
		if e.degraded {
			e.rememberAndLog(wake)
			return true
		}
		e.logf("queue %s wake for %s: %v; markers will retry the observation", wake.kind, wake.key, err)
		return false
	}

	e.logf("queued %s wake %s for %s: %s", wake.kind, record.ID, wake.key, wake.reason)
	return true
}

func (e *Engine) rememberAndLog(wake pendingWake) {
	e.remember(wake)
	e.logf("queued %s wake for %s in memory (no durable ID): %s", wake.kind, wake.key, wake.reason)
}

// remember holds a wake the durable ledger could not take, with the same
// keep-latest dedupe the ledger applies.
func (e *Engine) remember(wake pendingWake) {
	for i, record := range e.queue {
		if record.Kind == wake.kind && record.Key == wake.key {
			e.queue[i].Reason = wake.reason
			return
		}
	}
	e.queue = append(e.queue, WakeRecord{Kind: wake.kind, Key: wake.key, Reason: wake.reason})
}

// drain sends everything owed in one message, provided the rate window is open.
func (e *Engine) drain(ctx context.Context, markers *Markers) {
	if !e.windowOpen(*markers) {
		return
	}

	records := e.pending(ctx)
	if len(records) == 0 {
		return
	}

	reasons := make([]string, 0, len(records))
	for _, record := range records {
		reasons = append(reasons, record.Reason)
	}

	body := ComposeWakeBody(reasons)
	if body == "" {
		return
	}

	messageID, err := e.Waker.Deliver(ctx, body)
	if err != nil {
		e.logf("deliver wake: %v; %d wake(s) stay queued", err, len(records))
		return
	}

	ids := e.retire(ctx, records, messageID)

	markers.LastWakeUnix = e.Now().Unix()
	if onlyHeartbeats(records) {
		markers.HeartbeatStreak++
	} else {
		markers.HeartbeatStreak = 0
	}

	if len(ids) == 0 {
		e.logf("delivered %d wake(s) as %s", len(records), messageID)
		return
	}
	e.logf("delivered %d wake(s) as %s: %s", len(records), messageID, strings.Join(ids, " "))
}

func (e *Engine) pending(ctx context.Context) []WakeRecord {
	if e.degraded {
		return e.queue
	}

	records, err := e.Ledger.Pending()
	if err != nil {
		e.logf("read pending wakes: %v", err)
		e.noteLedgerError(ctx, err)
		return e.queue
	}

	return append(records, e.queue...)
}

// retire marks the delivered wakes in the ledger and returns the ledger IDs it
// retired, which the caller records in the decision log.
func (e *Engine) retire(ctx context.Context, records []WakeRecord, messageID string) []string {
	e.queue = nil
	if e.degraded {
		return nil
	}

	var ids []string
	for _, record := range records {
		// Every ID the record speaks for, not just the survivor: retiring the
		// survivor alone leaves the entries it superseded queued, and they
		// resurface on the next drain carrying stale reasons.
		ids = append(ids, record.IDs...)
	}
	if len(ids) == 0 {
		return nil
	}

	if err := e.Ledger.MarkDelivered(ids, messageID); err != nil {
		e.logf("retire delivered wakes: %v", err)
		e.noteLedgerError(ctx, err)
		return nil
	}
	// Compaction drops the retired entries and their delivered markers; it is
	// best effort because a full ledger is a housekeeping problem, not a
	// reason to fail the cycle.
	if err := e.Ledger.Compact(); err != nil {
		e.logf("compact wake ledger: %v", err)
	}

	return ids
}

// noteLedgerError degrades to in-memory supervision when the durable ledger is
// corrupt. The watcher tells the orchestrator once and keeps watching: a
// crash-looping or silent watcher is worse than one without durable replay.
func (e *Engine) noteLedgerError(ctx context.Context, err error) {
	if !errors.Is(err, ErrCorruptLog) {
		return
	}
	if e.degraded {
		return
	}

	e.degraded = true
	e.logf("wake ledger is corrupt: %v; durable replay disabled, continuing in memory", err)

	notice := fmt.Sprintf(
		"Watcher: the wake ledger at %s is corrupt; durable replay is disabled until Fledge is restarted. Supervision continues in memory.\nAutomated watcher notification — do not reply to this message ID.",
		statedir.WatchSession(e.Root, e.Session),
	)
	if _, err := e.Waker.Deliver(ctx, notice); err != nil {
		e.logf("deliver ledger corruption notice: %v", err)
	}
}

func (e *Engine) windowOpen(markers Markers) bool {
	if markers.LastWakeUnix == 0 {
		return true
	}
	window := time.Duration(e.Config.WakeMinIntervalSeconds) * time.Second
	return e.Now().Sub(time.Unix(markers.LastWakeUnix, 0).UTC()) >= window
}

// --- markers and helpers ------------------------------------------------

func (e *Engine) loadMarkers() Markers {
	markers, err := e.Ledger.LoadMarkers()
	if err != nil {
		e.logf("load watch markers: %v", err)
	}

	if markers.StatusSeen == nil {
		markers.StatusSeen = make(map[string]StatusSeen)
	}
	if markers.EventEscalated == nil {
		markers.EventEscalated = make(map[string]bool)
	}
	if markers.DoneGrace == nil {
		markers.DoneGrace = make(map[string]int64)
	}

	return markers
}

func (e *Engine) saveMarkers(markers Markers) {
	if err := e.Ledger.SaveMarkers(markers); err != nil {
		e.logf("save watch markers: %v", err)
	}
}

func cloneMarkers(markers Markers) Markers {
	clone := markers
	clone.StatusSeen = make(map[string]StatusSeen, len(markers.StatusSeen))
	for name, seen := range markers.StatusSeen {
		clone.StatusSeen[name] = seen
	}
	clone.EventEscalated = make(map[string]bool, len(markers.EventEscalated))
	for paneID, escalated := range markers.EventEscalated {
		clone.EventEscalated[paneID] = escalated
	}
	clone.DoneGrace = make(map[string]int64, len(markers.DoneGrace))
	for name, doneAt := range markers.DoneGrace {
		clone.DoneGrace[name] = doneAt
	}
	clone.KnownAgents = append([]string(nil), markers.KnownAgents...)
	return clone
}

func (e *Engine) statusPath(name string) string {
	return statedir.StatusFile(e.Root, e.Session, name)
}

func (e *Engine) eventsActive() bool {
	return e.Config.EventStream && !e.eventsDisabled
}

func (e *Engine) pollInterval() time.Duration {
	return interval(e.Config.PollIntervalSeconds)
}

func (e *Engine) idleInterval() time.Duration {
	return interval(e.Config.IdlePollIntervalSeconds)
}

func interval(seconds int) time.Duration {
	if seconds < minIntervalSeconds {
		seconds = minIntervalSeconds
	}
	return time.Duration(seconds) * time.Second
}

func (e *Engine) wait(ctx context.Context, d time.Duration) {
	if d > 0 {
		e.Sleep(ctx, d)
	}
}

func (e *Engine) logf(format string, args ...any) {
	e.Log(fmt.Sprintf(format, args...))
}

func namedWorkers(snapshot herdr.Snapshot) []supervised {
	var workers []supervised
	for _, agent := range snapshot.Agents {
		if agent.Name == nil {
			continue
		}
		name := *agent.Name
		if name == "" || name == orchestratorAgent {
			continue
		}
		workers = append(workers, supervised{Name: name, PaneID: agent.PaneID, AgentStatus: agent.AgentStatus})
	}
	return workers
}

func snapshotHasOrchestrator(snapshot herdr.Snapshot) bool {
	for _, agent := range snapshot.Agents {
		if agent.Name != nil && *agent.Name == orchestratorAgent {
			return true
		}
	}
	return false
}

func sessionRunning(sessions []herdr.Session, name string) bool {
	for _, session := range sessions {
		if session.Name == name {
			return session.Running
		}
	}
	return false
}

func statusReason(name, verb, detail string) string {
	if detail == "" {
		return fmt.Sprintf("%s %s", name, verb)
	}
	return fmt.Sprintf("%s %s: %s", name, verb, detail)
}

func onlyHeartbeats(records []WakeRecord) bool {
	for _, record := range records {
		if record.Kind != KindHeartbeat {
			return false
		}
	}
	return true
}

func sortedKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
