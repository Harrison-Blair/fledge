package watchproc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

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

// runDispatcher turns durable coordination events and Herdr push events into
// agent wakes without polling either source.
func runDispatcher(ctx context.Context, options Options) error {
	if options.Herdr == nil {
		return errors.New("dispatcher Herdr client is missing")
	}
	if options.WatchFile == nil {
		options.WatchFile = watchLedger
	}
	protocol, err := options.Herdr.Protocol(ctx)
	if err != nil {
		return fmt.Errorf("resolve Herdr protocol for dispatcher: %w", err)
	}
	if protocol < RequiredHerdrProtocol {
		return fmt.Errorf("Herdr protocol %d is unsupported; protocol %d or newer is required. Upgrade Herdr, then run fledge stop and fledge start", protocol, RequiredHerdrProtocol)
	}
	store := messaging.New(options.Root, options.Session)
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
	if err := drain(ctx, options.Herdr, options.Session, store); err != nil {
		return err
	}

	type generationEvent struct {
		generation int
		event      herdr.Event
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
	panes, err := activePanes(store)
	if err != nil {
		return err
	}

	generation := 0
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
	restart := func() {
		stopStream()
		// Bump the generation on every teardown, not only when resubscribing. The
		// stream just canceled will still deliver a terminal streamResult tagged
		// with its own generation; advancing here before the idle branch makes that
		// result — and any event or readiness the torn-down stream buffered —
		// compare stale, so an idle transition rests instead of mistaking the
		// canceled stream's end for a fatal stream failure.
		generation++
		if len(panes) == 0 {
			stopStream = func() {}
			announce()
			return
		}
		streamCtx, cancel := context.WithCancel(ctx)
		stopStream = cancel
		current := generation
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
	}
	restart()
	for {
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
			announce()
		case wrapped := <-events:
			// An event buffered by a torn-down stream must not run against the new
			// generation's just-reconciled registry and undo it.
			if wrapped.generation != generation {
				continue
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
			if err := drain(ctx, options.Herdr, options.Session, store); err != nil {
				return err
			}
			current, err := activePanes(store)
			if err != nil {
				return err
			}
			if strings.Join(current, "\x00") != strings.Join(panes, "\x00") {
				panes = current
				restart()
			}
		}
	}
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
	wakes, err := store.PendingWakes()
	if err != nil {
		return err
	}
	for _, wake := range wakes {
		if wake.Kind == "message" {
			message, getErr := store.Get(wake.ReferenceID)
			if getErr != nil {
				return getErr
			}
			if message.Status == messaging.StatusPending {
				if _, err := store.RecordAttempt(message.ID); err != nil {
					return err
				}
			}
		}
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
			// Re-read after the prompt fails: a reply or another dispatcher can
			// resolve an uncertain message while the prompt is in flight, and only
			// a still-uncertain message may take a delivery outcome. A cached
			// status would reintroduce that race and, worse, let RecordDelivery
			// reject an already-terminal message and strand this replayable wake.
			if wake.Kind == "message" {
				message, getErr := store.Get(wake.ReferenceID)
				if getErr != nil {
					return getErr
				}
				if message.Status == messaging.StatusUncertain {
					if _, recordErr := store.RecordDelivery(message.ID, false, err.Error()); recordErr != nil {
						return recordErr
					}
				}
			}
			// The wake is independent replayable state and must always terminalize,
			// even when its message was already delivered or failed.
			if _, recordErr := store.RecordWakeOutcome(wake.ID, false, err.Error()); recordErr != nil {
				return recordErr
			}
			continue
		}
		if wake.Kind == "message" {
			message, getErr := store.Get(wake.ReferenceID)
			if getErr != nil {
				return getErr
			}
			if message.Status == messaging.StatusUncertain {
				if _, err := store.RecordDelivery(message.ID, true, ""); err != nil {
					return err
				}
			}
		}
		if _, err := store.RecordWakeOutcome(wake.ID, true, ""); err != nil {
			return err
		}
	}
	return nil
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
