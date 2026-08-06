// Package dispatcher turns durable coordination events and Herdr push events
// into agent wakes without polling either source.
package dispatcher

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
	"github.com/Harrison-Blair/fledge/internal/watch"
)

const RequiredHerdrProtocol = 19

type Herdr interface {
	Protocol(context.Context) (int, error)
	List(context.Context) ([]herdr.Session, error)
	PromptAgent(context.Context, string, string, string) error
}

type FileWatcher interface {
	Events() <-chan struct{}
	Errors() <-chan error
	Close() error
}

type WatchFile func(string) (FileWatcher, error)
type Subscribe func(context.Context, []string, func(), func(watch.Event)) error

type Options struct {
	Root, Session string
	Herdr         Herdr
	WatchFile     WatchFile
	Subscribe     Subscribe
	Ready         func()
}

func Run(ctx context.Context, options Options) error {
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

	events := make(chan watch.Event, 16)
	type streamResult struct {
		generation int
		err        error
	}
	streamErrors := make(chan streamResult, 4)
	acked := make(chan int, 4)
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
		if len(panes) == 0 {
			stopStream = func() {}
			announce()
			return
		}
		generation++
		streamCtx, cancel := context.WithCancel(ctx)
		stopStream = cancel
		current := generation
		ids := append([]string(nil), panes...)
		go func() {
			streamErrors <- streamResult{generation: current, err: options.Subscribe(streamCtx, ids,
				func() { acked <- current }, func(event watch.Event) { events <- event })}
		}()
	}
	restart()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case current := <-acked:
			if current == generation {
				announce()
			}
		case event := <-events:
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
			if wake.Kind == "message" {
				if _, recordErr := store.RecordDelivery(wake.ReferenceID, false, err.Error()); recordErr != nil {
					return recordErr
				}
			}
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
	return func(streamCtx context.Context, panes []string, ready func(), event func(watch.Event)) error {
		return watch.Subscribe(streamCtx, dial, panes, ready, event)
	}, nil
}
