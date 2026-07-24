package daemon

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

const inboxNotifyMaxBackoff = 5 * time.Second
const inboxWakeBatchSize = 64

var inboxNotifyInitialBackoff = 25 * time.Millisecond
var inboxWakeTimeout = 2 * time.Minute

type inboxWakeTarget struct {
	Name        string
	Integration string
	SessionID   string
	Cwd         string
	Credential  string
}

// inboxWakeMessage deliberately has no body field. It is the only message
// shape the integration wake adapter can encode.
type inboxWakeMessage struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	ReplyTo string `json:"reply_to,omitempty"`
}

type inboxWakeMetadata struct {
	Messages []inboxWakeMessage `json:"messages"`
}

type inboxWakeFunc func(context.Context, inboxWakeTarget, inboxWakeMetadata) error

type inboxNotifyTask struct {
	name    string
	attempt uint
	readyAt time.Time
}

type inboxNotifyFlight struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (d *Daemon) startInboxNotifier() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.inboxNotifyStarted {
		return
	}
	if d.inboxNotify == nil {
		d.inboxNotify = make(chan struct{}, 1)
	}
	if d.inboxNotifyDone == nil {
		d.inboxNotifyDone = make(chan struct{})
	}
	if d.inboxNotifyTasks == nil {
		d.inboxNotifyTasks = make(map[string]*inboxNotifyTask)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.inboxWakeCancel = cancel
	d.inboxNotifyStarted = true
	go d.runInboxNotifier(ctx)
}

func (d *Daemon) runInboxNotifier(ctx context.Context) {
	defer close(d.inboxNotifyDone)
	for {
		task, wait := d.nextInboxNotifyTask()
		if task == nil {
			select {
			case <-ctx.Done():
				return
			case <-d.inboxNotify:
			}
			continue
		}
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-d.inboxNotify:
				if !timer.Stop() {
					<-timer.C
				}
				continue
			case <-timer.C:
			}
			continue
		}
		d.notifyInboxAgent(ctx, task)
	}
}

func (d *Daemon) nextInboxNotifyTask() (*inboxNotifyTask, time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return nil, 0
	}
	var next *inboxNotifyTask
	for _, task := range d.inboxNotifyTasks {
		if next == nil || task.readyAt.Before(next.readyAt) ||
			(task.readyAt.Equal(next.readyAt) && task.name < next.name) {
			next = task
		}
	}
	if next == nil {
		return nil, 0
	}
	wait := time.Until(next.readyAt)
	copy := *next
	if wait > 0 {
		return &copy, wait
	}
	delete(d.inboxNotifyTasks, next.name)
	return &copy, 0
}

func (d *Daemon) queueInboxWake(msg protocol.Message) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.shouldNotifyInboxLocked(msg) {
		return
	}
	d.queueInboxWakeLocked(msg.To, 0, time.Time{})
}

func (d *Daemon) queueInboxWakeLocked(name string, attempt uint, readyAt time.Time) {
	if !d.inboxNotifyEligibleLocked(name) {
		return
	}
	if existing := d.inboxNotifyTasks[name]; existing != nil {
		// A new message coalesces into any already scheduled work without
		// resetting its failure backoff. A failed in-flight attempt upgrades an
		// immediate task queued during that attempt to the retry deadline.
		if attempt > 0 {
			if attempt > existing.attempt {
				existing.attempt = attempt
			}
			if existing.readyAt.IsZero() || readyAt.After(existing.readyAt) {
				existing.readyAt = readyAt
			}
		}
	} else {
		d.inboxNotifyTasks[name] = &inboxNotifyTask{name: name, attempt: attempt, readyAt: readyAt}
	}
	select {
	case d.inboxNotify <- struct{}{}:
	default:
	}
}

func (d *Daemon) replayInboxNotifications() {
	d.mu.Lock()
	if d.inboxWake == nil {
		// The current Herdr launcher owns each interactive TUI process. Fledge
		// therefore has no same-process input/control channel to replay against.
		// Old journals may say the removed resume-process broker was armed; keep
		// the mailbox durable but never resurrect that unsafe capability.
		for name := range d.inboxNotifyArmed {
			d.inboxNotifyArmed[name] = false
		}
		d.notifyPending = nil
		d.mu.Unlock()
		d.debug.Printf("inbox delivery degraded: no owned same-session integration channel")
		return
	}
	pending := append([]protocol.Message(nil), d.notifyPending...)
	d.notifyPending = nil
	for _, msg := range pending {
		if d.shouldNotifyInboxLocked(msg) {
			d.queueInboxWakeLocked(msg.To, 0, time.Time{})
		}
	}
	d.mu.Unlock()
}

func (d *Daemon) shouldNotifyInboxLocked(msg protocol.Message) bool {
	return d.inboxNotifyEligibleLocked(msg.To) && !d.inboxNotified[msg.ID]
}

func (d *Daemon) inboxNotifyEligibleLocked(name string) bool {
	return !d.closing && !d.stopping[name] && !d.agentWorkspaceClosingLocked(name) &&
		d.inboxWake != nil && d.inboxNotifyArmed[name]
}

func (d *Daemon) validateInboxNotifierArmLocked(name, runtimeSessionID string) error {
	if name != agentcfg.ReservedOrchestrator {
		return nil
	}
	a, ok := d.agents[name]
	if !ok {
		return fmt.Errorf("cannot arm inbox delivery for unknown agent %q", name)
	}
	if d.inboxWake == nil {
		return fmt.Errorf("cannot arm inbox delivery for %q: no owned same-session integration control channel", name)
	}
	sessionID := a.SessionID
	if runtimeSessionID != "" {
		sessionID = runtimeSessionID
	}
	switch a.Integration {
	case "claude", "pi", "codex":
		if sessionID == "" {
			return fmt.Errorf("cannot arm inbox delivery for %q: %s did not provide a same-session handle", name, a.Integration)
		}
	default:
		return fmt.Errorf("cannot arm inbox delivery for %q: integration %q has no same-session adapter", name, a.Integration)
	}
	return nil
}

func (d *Daemon) pendingInboxWakesLocked(name string) []protocol.Message {
	var pending []protocol.Message
	for _, id := range d.messageOrder {
		msg := d.messages[id]
		if msg.To == name && !d.inboxNotified[msg.ID] {
			pending = append(pending, msg)
			if len(pending) == inboxWakeBatchSize {
				break
			}
		}
	}
	return pending
}

func (d *Daemon) notifyInboxAgent(ctx context.Context, task *inboxNotifyTask) {
	d.mu.Lock()
	if !d.inboxNotifyEligibleLocked(task.name) {
		d.mu.Unlock()
		return
	}
	a, ok := d.agents[task.name]
	if !ok || a.State == stateStopped || a.State == stateOrphaned {
		d.mu.Unlock()
		return
	}
	messages := d.pendingInboxWakesLocked(task.name)
	if len(messages) == 0 {
		d.mu.Unlock()
		return
	}
	target := inboxWakeTarget{
		Name: task.name, Integration: a.Integration, SessionID: a.SessionID,
		Cwd: a.Cwd, Credential: d.identityTokens[task.name],
	}
	metadata := inboxWakeMetadata{Messages: make([]inboxWakeMessage, 0, len(messages))}
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		metadata.Messages = append(metadata.Messages, inboxWakeMessage{
			ID: msg.ID, From: msg.From, To: msg.To, ReplyTo: msg.ReplyTo,
		})
		ids = append(ids, msg.ID)
	}
	wake := d.inboxWake
	wakeCtx, cancel := context.WithTimeout(ctx, inboxWakeTimeout)
	flight := &inboxNotifyFlight{cancel: cancel, done: make(chan struct{})}
	d.inboxNotifyFlights[task.name] = flight
	d.mu.Unlock()

	err := wake(wakeCtx, target, metadata)
	cancel()
	close(flight.done)
	d.mu.Lock()
	if d.inboxNotifyFlights[task.name] == flight {
		delete(d.inboxNotifyFlights, task.name)
	}
	stopped := !d.inboxNotifyEligibleLocked(task.name)
	d.mu.Unlock()
	if stopped {
		return
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			d.debug.Printf("inbox wake %s: %v", task.name, err)
		}
		d.retryInboxNotify(task)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.inboxNotifyEligibleLocked(task.name) {
		return
	}
	// Journal only the IDs actually present in the successful external turn.
	// If this append fails they remain durable and unnotified, so retry may
	// duplicate the turn (at-least-once) but can never duplicate a claim.
	sort.Strings(ids)
	if err := d.append(event{Event: evInboxNotified, To: task.name, IDs: ids}); err != nil {
		d.debug.Printf("inbox wake %s: journal: %v", task.name, err)
		d.retryInboxNotifyLocked(task)
		return
	}
	for _, id := range ids {
		d.inboxNotified[id] = true
	}
	if len(d.pendingInboxWakesLocked(task.name)) > 0 {
		d.queueInboxWakeLocked(task.name, 0, time.Time{})
	}
}

func (d *Daemon) retryInboxNotify(task *inboxNotifyTask) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.retryInboxNotifyLocked(task)
}

func (d *Daemon) retryInboxNotifyLocked(task *inboxNotifyTask) {
	if !d.inboxNotifyEligibleLocked(task.name) {
		return
	}
	if task.attempt < 31 {
		task.attempt++
	}
	delay := inboxNotifyInitialBackoff
	for i := uint(1); i < task.attempt && delay < inboxNotifyMaxBackoff; i++ {
		delay *= 2
	}
	if delay > inboxNotifyMaxBackoff {
		delay = inboxNotifyMaxBackoff
	}
	d.queueInboxWakeLocked(task.name, task.attempt, time.Now().Add(delay))
}
