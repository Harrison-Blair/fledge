package watchproc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

const RequiredHerdrProtocol = 19

type FileWatcher interface {
	Events() <-chan struct{}
	Errors() <-chan error
	Close() error
}

type WatchFile func(string) (FileWatcher, error)
type Subscribe func(context.Context, []string, func(), func(herdr.Event)) error

type dispatcherTimer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(time.Duration) bool
}

type runtimeTimer struct{ timer *time.Timer }

func (t *runtimeTimer) C() <-chan time.Time            { return t.timer.C }
func (t *runtimeTimer) Stop() bool                     { return t.timer.Stop() }
func (t *runtimeTimer) Reset(delay time.Duration) bool { return t.timer.Reset(delay) }

func newRuntimeTimer(delay time.Duration) dispatcherTimer {
	return &runtimeTimer{timer: time.NewTimer(delay)}
}

// runDispatcher turns durable coordination events and Herdr push events into
// agent wakes without polling either source.
func runDispatcher(ctx context.Context, options Options) error {
	if options.Herdr == nil {
		return errors.New("dispatcher Herdr client is missing")
	}
	if options.WatchFile == nil {
		options.WatchFile = watchLedger
	}
	if options.clock == nil {
		options.clock = time.Now
	}
	if options.newTimer == nil {
		options.newTimer = newRuntimeTimer
	}
	protocol, err := options.Herdr.Protocol(ctx)
	if err != nil {
		return fmt.Errorf("resolve Herdr protocol for dispatcher: %w", err)
	}
	if protocol < RequiredHerdrProtocol {
		return fmt.Errorf("Herdr protocol %d is unsupported; protocol %d or newer is required. Upgrade Herdr, then run fledge stop and fledge start", protocol, RequiredHerdrProtocol)
	}
	store := messaging.New(options.Root, options.Session, messaging.WithClock(options.clock))
	if _, err := store.Ensure(); err != nil {
		return err
	}
	fileEvents, err := options.WatchFile(store.LogPath())
	if err != nil {
		return fmt.Errorf("watch session ledger: %w", err)
	}
	defer fileEvents.Close()
	if options.Subscribe == nil {
		subscribe, err := socketSubscriber(ctx, options.Herdr, options.Session)
		if err != nil {
			return err
		}
		options.Subscribe = subscribe
	}
	panes, err := activePanes(store)
	if err != nil {
		return err
	}
	// No pane has acknowledged event coverage yet. Deliver unrelated work and
	// replay activations whose recipients are no longer live, but leave a live
	// assignee's activation pending until its subscription is ready. Otherwise a
	// fast worker can run working -> idle entirely before the stream exists and
	// leave only the latest idle snapshot for startup supervision to observe.
	if err := drainCovered(ctx, options.Herdr, options.Session, store, nil); err != nil {
		return err
	}

	var deadlineTimer dispatcherTimer
	var deadlineEvents <-chan time.Time
	deadlineAuditReady := true
	selectDeadlineEvents := func() {
		deadlineEvents = nil
		if deadlineAuditReady && deadlineTimer != nil {
			deadlineEvents = deadlineTimer.C()
		}
	}
	stopDeadlineTimer := func() {
		if deadlineTimer == nil {
			return
		}
		if !deadlineTimer.Stop() {
			select {
			case <-deadlineTimer.C():
			default:
			}
		}
	}
	defer stopDeadlineTimer()
	rescheduleDeadline := func() error {
		deadline, err := store.NextAgentIdleDeadline()
		if err != nil {
			return err
		}
		if deadline.IsZero() {
			stopDeadlineTimer()
			deadlineTimer = nil
			selectDeadlineEvents()
			return nil
		}
		delay := deadline.Sub(options.clock().UTC())
		if delay < 0 {
			delay = 0
		}
		if deadlineTimer == nil {
			deadlineTimer = options.newTimer(delay)
		} else {
			stopDeadlineTimer()
			deadlineTimer.Reset(delay)
		}
		selectDeadlineEvents()
		return nil
	}
	if err := rescheduleDeadline(); err != nil {
		return err
	}

	type generationEvent struct {
		generation int
		event      herdr.Event
	}
	// eventIngress serializes one acknowledged subscription's callbacks with a
	// deadline audit. The callback holds the gate until its event is queued; the
	// deadline path can therefore drain to an exact boundary and keep later
	// callbacks from overtaking the audit.
	type eventIngress struct {
		gate      sync.Mutex
		accepting bool
	}
	events := make(chan generationEvent, 16)
	type streamResult struct {
		generation int
		err        error
	}
	streamErrors := make(chan streamResult, 4)
	// streamReady carries a snapshot taken from the acknowledged subscription
	// callback. The callback blocks on release until the run loop has reconciled
	// that snapshot, so post-ack status events cannot be applied before it.
	type streamReady struct {
		generation int
		panes      []string
		snapshot   herdr.Snapshot
		err        error
		release    chan struct{}
	}
	readyResults := make(chan streamReady, 4)
	generation := 0
	var currentIngress *eventIngress
	var coveredPanes []string
	applyStreamEvent := func(wrapped generationEvent) error {
		// An event buffered by a torn-down stream must not run against the new
		// generation's just-reconciled registry and undo it.
		if wrapped.generation != generation {
			return nil
		}
		event := wrapped.event
		var err error
		if event.Type == "pane.closed" {
			err = store.StopAgentByPane(event.PaneID)
		} else {
			err = store.RecordAgentStatus(event.PaneID, event.AgentStatus)
		}
		if err != nil {
			return err
		}
		if err := rescheduleDeadline(); err != nil {
			return err
		}
		if options.eventApplied != nil {
			options.eventApplied()
		}
		return nil
	}
	// quiesceStreamEvents applies everything accepted by the current
	// subscription and returns with its ingress gate held. Draining before
	// taking the gate prevents a full events channel from deadlocking with an
	// onEvent callback that already holds the gate. Once the gate is acquired,
	// that callback has either queued its event or observed cancellation, and a
	// final drain establishes a precise ordering boundary.
	quiesceStreamEvents := func() (func(), error) {
		for {
			select {
			case wrapped := <-events:
				if err := applyStreamEvent(wrapped); err != nil {
					return nil, err
				}
				continue
			default:
			}

			ingress := currentIngress
			if ingress == nil {
				return func() {}, nil
			}
			ingress.gate.Lock()
			for {
				select {
				case wrapped := <-events:
					if err := applyStreamEvent(wrapped); err != nil {
						ingress.gate.Unlock()
						return nil, err
					}
				default:
					return ingress.gate.Unlock, nil
				}
			}
		}
	}
	stopStream := func() {}
	defer func() { stopStream() }()
	announced := false
	announce := func() {
		if !announced {
			announced = true
			if options.Ready != nil {
				options.Ready()
			}
		}
	}
	// A session with no live pane is a resting state, not a failure: the last
	// agent has stopped and the next spawn will register another. The dispatcher
	// keeps consuming the ledger and simply holds no subscription, so it stays
	// available to deliver that spawn's wakes instead of exiting for good.
	restart := func() error {
		// Establish the outgoing generation's exact acceptance boundary before
		// canceling it. Events whose callbacks acquired ingress before this point
		// are durable and must be applied; callbacks acquiring it afterward see
		// accepting=false and are deterministically discarded.
		releaseIngress, err := quiesceStreamEvents()
		if err != nil {
			return err
		}
		panes, err = activePanes(store)
		if err != nil {
			releaseIngress()
			return err
		}
		if currentIngress != nil {
			currentIngress.accepting = false
		}
		stopStream()
		// Bump the generation on every teardown, not only when resubscribing. The
		// stream just canceled will still deliver a terminal streamResult tagged
		// with its own generation; advancing here before the idle branch makes that
		// result — and any event or readiness the torn-down stream buffered —
		// compare stale, so an idle transition rests instead of mistaking the
		// canceled stream's end for a fatal stream failure.
		generation++
		currentIngress = nil
		coveredPanes = nil
		deadlineAuditReady = len(panes) == 0
		selectDeadlineEvents()
		releaseIngress()
		if len(panes) == 0 {
			stopStream = func() {}
			announce()
			return nil
		}
		streamCtx, cancel := context.WithCancel(ctx)
		stopStream = cancel
		current := generation
		ingress := &eventIngress{accepting: true}
		currentIngress = ingress
		ids := append([]string(nil), panes...)
		// onReady runs synchronously inside Subscribe once Herdr acknowledges the
		// subscription and before any stream event is read. Snapshotting from here
		// and blocking on release until the run loop has reconciled it closes the
		// resubscribe window: a pane.closed that arrived while the previous stream
		// was torn down and this one not yet live reaches neither stream, but the
		// snapshot still reflects it. All sends/waits select on streamCtx.Done() so
		// stopStream cannot leak this goroutine.
		onReady := func() {
			snapshot, snapErr := options.Herdr.Snapshot(streamCtx, options.Session)
			if streamCtx.Err() != nil {
				return
			}
			ready := streamReady{generation: current, panes: ids, snapshot: snapshot,
				err: snapErr, release: make(chan struct{})}
			select {
			case readyResults <- ready:
			case <-streamCtx.Done():
				return
			}
			select {
			case <-ready.release:
			case <-streamCtx.Done():
			}
		}
		onEvent := func(event herdr.Event) {
			ingress.gate.Lock()
			defer ingress.gate.Unlock()
			if !ingress.accepting {
				return
			}
			select {
			case events <- generationEvent{generation: current, event: event}:
			case <-streamCtx.Done():
			}
		}
		go func() {
			result := streamResult{generation: current, err: options.Subscribe(streamCtx, ids, onReady, onEvent)}
			// Mirror the onReady/onEvent sends: streamCtx is canceled either by a
			// restart tearing this stream down or by Run's defer stopStream, so a
			// rapid burst of canceled subscriptions cannot pin sender goroutines on a
			// full streamErrors channel after the run loop has stopped receiving.
			select {
			case streamErrors <- result:
			case <-streamCtx.Done():
			}
		}()
		return nil
	}
	if err := restart(); err != nil {
		return err
	}
	drainLedger := func() error {
		current, err := activePanes(store)
		if err != nil {
			return err
		}
		changed := !samePanes(current, panes)
		// The outgoing acknowledged stream remains authoritative until restart
		// quiesces its ingress. Deliver everything it already covers, plus all
		// non-activation wakes, before replacing it. A skipped wake never blocks a
		// later eligible wake in ledger order.
		if err := drainCovered(ctx, options.Herdr, options.Session, store, coveredPanes); err != nil {
			return err
		}
		if changed {
			if err := restart(); err != nil {
				return err
			}
			// Quiescing can itself append status wakes. Drain those immediately,
			// while continuing to hold live activation wakes until replacement
			// readiness establishes coverage.
			if err := drainCovered(ctx, options.Herdr, options.Session, store, coveredPanes); err != nil {
				return err
			}
		}
		return rescheduleDeadline()
	}
	for {
		if options.selectPrepared != nil {
			options.selectPrepared(deadlineEvents != nil)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ready := <-readyResults:
			// A stale generation's callback is unblocked and ignored: its stream is
			// already being torn down and its snapshot describes a pane set we no
			// longer subscribe to.
			if ready.generation != generation {
				close(ready.release)
				continue
			}
			// Subscribe promises one readiness callback. Treat any duplicate as
			// stale so an older snapshot can never overwrite status reconciled by a
			// later deadline audit in this generation.
			if deadlineAuditReady {
				close(ready.release)
				continue
			}
			// Reconcile before releasing the callback. Only then can Herdr deliver
			// post-ack events, so a status change queued after the snapshot cannot
			// be applied and then overwritten by older snapshot state. A snapshot
			// error is fatal: announcing readiness over a registry we could not
			// reconcile would strand exactly the missed transition D2 must catch.
			// The outer dispatcher process owns restart.
			if ready.err != nil {
				close(ready.release)
				return fmt.Errorf("reconcile Herdr snapshot for subscription: %w", ready.err)
			}
			if err := reconcileSnapshot(store, ready.panes, ready.snapshot); err != nil {
				close(ready.release)
				return err
			}
			close(ready.release)
			// Coverage becomes usable only after reconciliation has completed and
			// the synchronous onReady callback has been released. Settle deferred
			// activations immediately afterward, before selecting another source.
			coveredPanes = append(coveredPanes[:0], ready.panes...)
			deadlineAuditReady = true
			selectDeadlineEvents()
			if err := drainCovered(ctx, options.Herdr, options.Session, store, coveredPanes); err != nil {
				return err
			}
			if err := rescheduleDeadline(); err != nil {
				return err
			}
			announce()
		case wrapped := <-events:
			if err := applyStreamEvent(wrapped); err != nil {
				return err
			}
		case result := <-streamErrors:
			if result.generation != generation {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("Herdr dispatcher event stream ended: %w", result.err)
		case err := <-fileEvents.Errors():
			if err != nil {
				return fmt.Errorf("watch session ledger: %w", err)
			}
		case <-fileEvents.Events():
			if err := drainLedger(); err != nil {
				return err
			}
		case <-deadlineEvents:
			// The timer is only a prompt to re-check authoritative state. Apply all
			// pushes already accepted by this acknowledged subscription before the
			// snapshot. Pushes accepted while that snapshot and its reconciliation
			// are in flight are applied afterward, in stream order. Holding the
			// ingress gate across the audit then gives the audit a precise place in
			// that ordering and prevents a queued working observation from being
			// hidden by a latest-idle snapshot.
			releaseIngress, err := quiesceStreamEvents()
			if err != nil {
				return err
			}
			releaseIngress()
			subscribed := append([]string(nil), coveredPanes...)
			snapshot, err := options.Herdr.Snapshot(ctx, options.Session)
			if err != nil {
				return fmt.Errorf("reconcile Herdr snapshot for agent-idle deadline: %w", err)
			}
			if err := reconcileSnapshot(store, subscribed, snapshot); err != nil {
				return err
			}
			releaseIngress, err = quiesceStreamEvents()
			if err != nil {
				return err
			}
			_, auditErr := store.AuditDueAgentIdle()
			releaseIngress()
			if auditErr != nil {
				return auditErr
			}
			if err := drainLedger(); err != nil {
				return err
			}
		}
	}
}

func samePanes(left, right []string) bool {
	return strings.Join(left, "\x00") == strings.Join(right, "\x00")
}

func activePanes(store *messaging.Store) ([]string, error) {
	agents, err := store.Agents()
	if err != nil {
		return nil, err
	}
	var panes []string
	for _, agent := range agents {
		if agent.Active {
			panes = append(panes, agent.PaneID)
		}
	}
	sort.Strings(panes)
	return panes, nil
}

// reconcileSnapshot catches any pane close or status change that fell into the
// resubscribe window — between the previous stream's teardown and this stream
// going live — by treating the acknowledged subscription's snapshot as the
// authority. It is scoped to the pane IDs this generation subscribed to, never
// the whole registry, so a concurrent later registration this stream never
// covered is not mistaken for a departed pane. It is idempotent: reconciling an
// already-correct registry is a no-op because StopAgentByPane ignores
// inactive/unknown panes and RecordAgentStatus ignores an unchanged status.
func reconcileSnapshot(store *messaging.Store, subscribed []string, snapshot herdr.Snapshot) error {
	live := make(map[string]herdr.Pane, len(snapshot.Panes))
	for _, pane := range snapshot.Panes {
		live[pane.PaneID] = pane
	}
	for _, paneID := range subscribed {
		pane, ok := live[paneID]
		if !ok {
			if err := store.StopAgentByPane(paneID); err != nil {
				return err
			}
			continue
		}
		// A present pane with a blank status keeps its current registry status;
		// the store rejects a blank transition and the snapshot simply carries no
		// status to project.
		if pane.AgentStatus != "" {
			if err := store.RecordAgentStatus(paneID, pane.AgentStatus); err != nil {
				return err
			}
		}
	}
	return nil
}

// drain submits every pending wake. One undeliverable recipient must not stall
// the rest: recording its terminal failure outcome stops it being replayed, and
// the remaining wakes still go out. Only a storage failure, which would make
// outcomes unrecordable, ends the dispatcher.
func drain(ctx context.Context, client Herdr, session string, store *messaging.Store) error {
	panes, err := activePanes(store)
	if err != nil {
		return err
	}
	return drainCovered(ctx, client, session, store, panes)
}

// drainCovered submits every eligible pending wake without letting a deferred
// activation stall later ledger entries. A task activation for a live pane is
// eligible only while an acknowledged subscription covers that pane. An
// activation whose pane is no longer registered is still attempted so replay
// can reach a terminal outcome rather than remaining pending forever.
func drainCovered(ctx context.Context, client Herdr, session string, store *messaging.Store, coveredPanes []string) error {
	covered := make(map[string]struct{}, len(coveredPanes))
	for _, paneID := range coveredPanes {
		covered[paneID] = struct{}{}
	}
	wakes, err := store.PendingWakes()
	if err != nil {
		return err
	}
	for _, wake := range wakes {
		if activationWake(wake.Kind) {
			if _, ok := covered[wake.RecipientPane]; !ok {
				if _, agentErr := store.AgentByPane(wake.RecipientPane); agentErr == nil {
					continue
				} else if !errors.Is(agentErr, messaging.ErrAgentNotFound) {
					return agentErr
				}
			}
		}
		// A message wake's delivery status is projected onto its message by the
		// wake attempt and outcome transitions, so the wake is the sole record
		// the dispatcher writes.
		if _, err := store.RecordWakeAttempt(wake.ID); err != nil {
			return err
		}
		envelope := fmt.Sprintf("[Fledge wake]\nDelivery-ID: %s\nKind: %s\n\n%s", wake.ID, wake.Kind, wake.Body)
		err := client.PromptAgent(ctx, session, wake.Recipient, envelope)
		if err != nil {
			// A canceled context says nothing about the wake, so it stays
			// uncertain and the next dispatcher replays it.
			if ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if _, recordErr := store.RecordWakeOutcome(wake.ID, false, err.Error()); recordErr != nil {
				return recordErr
			}
			continue
		}
		if _, err := store.RecordWakeOutcome(wake.ID, true, ""); err != nil {
			return err
		}
	}
	return nil
}

func activationWake(kind string) bool {
	return kind == "task-assigned" || kind == "task-resumed"
}

func socketSubscriber(ctx context.Context, client Herdr, session string) (Subscribe, error) {
	sessions, err := client.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve Herdr dispatcher socket: %w", err)
	}
	path := ""
	for _, candidate := range sessions {
		if candidate.Name == session && candidate.Running {
			path = candidate.SocketPath
			break
		}
	}
	if path == "" {
		return nil, fmt.Errorf("running Herdr session %s has no event socket", session)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect Herdr event socket: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil, fmt.Errorf("Herdr event path %q is not a socket", path)
	}
	dial := func(dialCtx context.Context) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(dialCtx, "unix", path)
	}
	return func(streamCtx context.Context, panes []string, ready func(), event func(herdr.Event)) error {
		return herdr.Subscribe(streamCtx, dial, panes, ready, event)
	}, nil
}
