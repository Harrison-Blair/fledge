package run

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/inbox"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// Timings of the supervision rules.
const (
	quietAfter    = 30 * time.Second // idle or done this long before a quiet wake
	retryRingWait = 30 * time.Second // between attempts to ring one queued record
	idleExit      = 60 * time.Second // nothing to watch this long before exiting
)

// worker is one watched agent.
type worker struct {
	rec    identity.Record
	pane   string
	status string
	// quietSince starts the current idle-or-done episode; quietChecked marks
	// that its 30-second check ran.
	quietSince   time.Time
	quietChecked bool
}

// daemon applies the supervision rules. Only one goroutine calls its methods.
type daemon struct {
	c          libagent.Client
	store      *state.Store
	dir        string
	ackTimeout time.Duration
	log        *log.Logger
	ep         supervise.Episodes

	// watched maps each watched pane to its worker; agents is the last
	// agent.list, by terminal.
	watched map[string]*worker
	agents  map[string]herdr.AgentDetails
	// lastAttempt is when the daemon last tried to ring each queued record;
	// missing logs the queued records whose recipient has no live pane.
	lastAttempt map[string]time.Time
	missing     map[string]bool
	idleSince   time.Time
}

func newDaemon(c libagent.Client, s *state.Store, dir string, ack time.Duration, logger *log.Logger) (*daemon, error) {
	ep, err := supervise.LoadEpisodes(dir)
	if err != nil {
		return nil, err
	}
	return &daemon{c: c, store: s, dir: dir, ackTimeout: ack, log: logger, ep: ep, watched: map[string]*worker{}, agents: map[string]herdr.AgentDetails{}, lastAttempt: map[string]time.Time{}, missing: map[string]bool{}}, nil
}

// panes lists the watched panes, sorted.
func (d *daemon) panes() []string {
	var ps []string
	for p := range d.watched {
		ps = append(ps, p)
	}
	slices.Sort(ps)
	return ps
}

// watchedIDs lists the watched agent record ids, sorted.
func (d *daemon) watchedIDs() []string {
	ids := []string{}
	for _, w := range d.watched {
		ids = append(ids, w.rec.ID)
	}
	slices.Sort(ids)
	return ids
}

// reconcile rebuilds the watched set from agent.list and the store: live
// agents with a parent, agents owning an assigned task, and recipients of
// queued or unread rung records. A status that changed unseen is applied as
// a transition. It then rings deferred records and checks unread ones.
func (d *daemon) reconcile(ctx context.Context, now time.Time) error {
	list, err := d.c.List(ctx)
	if err != nil {
		return err
	}
	records, err := identity.LiveByTerminal(d.store)
	if err != nil {
		return err
	}
	tasks, err := task.List(d.store)
	if err != nil {
		return err
	}
	msgs, err := inbox.List(d.store)
	if err != nil {
		return err
	}
	owners, recipients := map[string]bool{}, map[string]bool{}
	for _, t := range tasks {
		if t.Status == task.Assigned && t.Owner != nil {
			owners[*t.Owner] = true
		}
	}
	for _, m := range msgs {
		if inbox.Unread(m) {
			recipients[m.To] = true
		}
	}
	// Fill the whole agent map first: a wake fired below may go to any agent.
	d.agents = map[string]herdr.AgentDetails{}
	for _, a := range list {
		d.agents[a.TerminalID] = a
	}
	byID := map[string]*worker{}
	for _, w := range d.watched {
		byID[w.rec.ID] = w
	}
	next := map[string]*worker{}
	for _, a := range list {
		rec, ok := identity.Attributed(records, a)
		if !ok || !(rec.Parent != nil || owners[rec.ID] || recipients[rec.ID]) {
			continue
		}
		w := byID[rec.ID]
		if w == nil {
			w = &worker{rec: rec, pane: a.PaneID, status: a.AgentStatus}
			d.log.Printf("watch %s (%s) in %s, status %s", rec.ID, name(rec), a.PaneID, a.AgentStatus)
			d.enter(ctx, w, "", a.AgentStatus, now)
		} else if w.status != a.AgentStatus {
			old := w.status
			w.status, w.rec, w.pane = a.AgentStatus, rec, a.PaneID
			d.enter(ctx, w, old, a.AgentStatus, now)
		}
		w.rec, w.pane = rec, a.PaneID
		next[a.PaneID] = w
		delete(byID, rec.ID)
	}
	d.watched = next
	// A watched agent gone from agent.list exited unseen; one still listed
	// only stopped qualifying, so it is no longer watched.
	for _, w := range byID {
		if _, listed := d.agents[w.rec.TerminalID]; listed {
			d.log.Printf("unwatch %s (%s)", w.rec.ID, w.pane)
			continue
		}
		d.exited(ctx, w)
	}
	d.ringQueued(ctx, msgs, now)
	d.checkUnread(ctx, msgs, now)
	return nil
}

// status applies a pane.agent_status_changed event.
func (d *daemon) status(ctx context.Context, pane, status string, now time.Time) {
	w := d.watched[pane]
	if w == nil || w.status == status {
		return
	}
	old := w.status
	w.status = status
	// Keep the cached agent current: deferred rings check its status.
	if a, ok := d.agents[w.rec.TerminalID]; ok {
		a.AgentStatus = status
		d.agents[w.rec.TerminalID] = a
	}
	d.log.Printf("status %s (%s) %s -> %s", w.rec.ID, pane, old, status)
	d.enter(ctx, w, old, status, now)
}

// enter applies w's move from old to status.
func (d *daemon) enter(ctx context.Context, w *worker, old, status string, now time.Time) {
	idle := status == "idle" || status == "done"
	switch {
	case status == "blocked" && !d.ep.Blocked[w.rec.ID]:
		d.ep.Blocked[w.rec.ID] = true
		d.save()
		d.wakeParent(ctx, w.rec, "blocked", fmt.Sprintf("Worker %s is blocked in pane %s: it waits on a dialog or an approval.\nInspect it: fledge agent read --pane %s\nAnswer it: fledge agent send --pane %s --key ...", name(w.rec), w.pane, w.pane, w.pane))
	case status != "blocked" && d.ep.Blocked[w.rec.ID]:
		delete(d.ep.Blocked, w.rec.ID)
		d.save()
	}
	switch {
	case idle && w.quietSince.IsZero():
		w.quietSince, w.quietChecked = now, false
	case !idle:
		w.quietSince, w.quietChecked = time.Time{}, false
		if d.ep.Quiet[w.rec.ID] {
			delete(d.ep.Quiet, w.rec.ID)
			d.save()
		}
	}
	if old == "blocked" && status != "blocked" {
		msgs, err := inbox.List(d.store)
		if err != nil {
			d.log.Printf("deferred rings for %s: %v", w.rec.ID, err)
			return
		}
		d.ringQueuedFor(ctx, w.rec.ID, msgs, now, true)
	}
}

// exitedPane applies a pane.exited or pane.closed event.
func (d *daemon) exitedPane(ctx context.Context, pane string) {
	if w := d.watched[pane]; w != nil {
		delete(d.watched, pane)
		d.exited(ctx, w)
	}
}

// exited reports w's exit once, unless Fledge ended its record first (agent
// stop and agent cleanup do, before closing the pane). The record itself is
// left to the lazy lookups that end records.
func (d *daemon) exited(ctx context.Context, w *worker) {
	var rec identity.Record
	if err := d.store.Get(identity.Kind, w.rec.ID, &rec); err != nil {
		d.log.Printf("exit of %s: %v", w.rec.ID, err)
		return
	}
	switch {
	case rec.EndedAt != nil:
		d.log.Printf("exit of %s (%s) suppressed: Fledge ended its record", w.rec.ID, w.pane)
		return
	case d.ep.Exited[w.rec.ID]:
		return
	}
	d.ep.Exited[w.rec.ID] = true
	d.save()
	tasks, _ := owned(d.store, w.rec.ID)
	var lines []string
	for _, t := range tasks {
		lines = append(lines, fmt.Sprintf("  %s %s %s", t.ID, t.Status, t.Title))
	}
	body := fmt.Sprintf("Worker %s exited: its pane %s closed while its record was live.\n", name(w.rec), w.pane)
	if len(lines) > 0 {
		body += "Its tasks:\n" + strings.Join(lines, "\n") + "\n"
	}
	body += "List them: fledge task list --owner " + w.rec.ID
	d.wakeParent(ctx, w.rec, "exited", body)
}

// moved follows a pane.moved event to the worker's new pane.
func (d *daemon) moved(previous, pane string) {
	if w := d.watched[previous]; w != nil {
		delete(d.watched, previous)
		w.pane = pane
		d.watched[pane] = w
		d.log.Printf("moved %s %s -> %s", w.rec.ID, previous, pane)
	}
}

// drop stops watching pane, whose subscription found no such pane.
func (d *daemon) drop(pane string) {
	if w := d.watched[pane]; w != nil {
		delete(d.watched, pane)
		d.log.Printf("drop %s (%s): pane not found", w.rec.ID, pane)
	}
}

// tick runs the time-based rules: the quiet check of each idle episode.
func (d *daemon) tick(ctx context.Context, now time.Time) {
	for _, w := range d.watched {
		if w.quietSince.IsZero() || w.quietChecked || now.Sub(w.quietSince) < quietAfter {
			continue
		}
		w.quietChecked = true
		if d.ep.Quiet[w.rec.ID] {
			continue
		}
		d.quiet(ctx, w)
	}
}

// quiet wakes w's parent when, 30 seconds into an idle episode, w still owns
// an assigned task and has sent its parent no inbox record since the last
// doorbell rung for it.
func (d *daemon) quiet(ctx context.Context, w *worker) {
	tasks, err := owned(d.store, w.rec.ID)
	if err != nil {
		d.log.Printf("quiet check of %s: %v", w.rec.ID, err)
		return
	}
	var assigned []task.Record
	for _, t := range tasks {
		if t.Status == task.Assigned {
			assigned = append(assigned, t)
		}
	}
	if len(assigned) == 0 {
		return
	}
	msgs, err := inbox.List(d.store)
	if err != nil {
		d.log.Printf("quiet check of %s: %v", w.rec.ID, err)
		return
	}
	var lastBell time.Time
	for _, m := range msgs {
		if r := inbox.LastRing(m); m.To == w.rec.ID && r != nil {
			if at := parse(r.At); at.After(lastBell) {
				lastBell = at
			}
		}
	}
	for _, m := range msgs {
		if m.From.ID != nil && *m.From.ID == w.rec.ID && w.rec.Parent != nil && m.To == *w.rec.Parent && !parse(m.CreatedAt).Before(lastBell) {
			return
		}
	}
	d.ep.Quiet[w.rec.ID] = true
	d.save()
	t := assigned[0]
	target := "--pane " + w.pane
	if w.rec.Name != nil {
		target = "--name " + *w.rec.Name
	}
	d.wakeParent(ctx, w.rec, "quiet", fmt.Sprintf("Worker %s has been %s for %s while it owns assigned task %s (%s) and has sent you nothing since its last doorbell. A worker waiting on a background job can look idle; check before acting.\nInspect it: fledge agent read %s", name(w.rec), w.status, quietAfter, t.ID, t.Title, target))
}

// ringQueued rings each queued record whose recipient is live and not
// blocked, at most once per retryRingWait each.
func (d *daemon) ringQueued(ctx context.Context, msgs []inbox.Record, now time.Time) {
	for _, m := range msgs {
		if inbox.State(m) == "queued" {
			d.ringQueuedFor(ctx, m.To, []inbox.Record{m}, now, false)
		}
	}
}

// ringQueuedFor rings to's queued records among msgs; force skips the wait
// between attempts, for a recipient that just left blocked.
func (d *daemon) ringQueuedFor(ctx context.Context, to string, msgs []inbox.Record, now time.Time, force bool) {
	for _, m := range msgs {
		if m.To != to || inbox.State(m) != "queued" {
			continue
		}
		if last, ok := d.lastAttempt[m.ID]; ok && !force && now.Sub(last) < retryRingWait {
			continue
		}
		a, ok := d.live(to)
		if !ok {
			if !d.missing[m.ID] {
				d.missing[m.ID] = true
				d.log.Printf("queued %s for %s: the recipient has no live pane", m.ID, to)
			}
			continue
		}
		if a.AgentStatus == "blocked" {
			continue
		}
		d.lastAttempt[m.ID] = now
		d.ring(ctx, m, a.PaneID, "deferred")
	}
}

// checkUnread rings each unread record once more ackTimeout after its first
// successful ring, then, another ackTimeout later, tells a registered sender
// that it is still unread. A wake or an unregistered sender is only logged.
func (d *daemon) checkUnread(ctx context.Context, msgs []inbox.Record, now time.Time) {
	for _, m := range msgs {
		if inbox.State(m) != "rung" {
			continue
		}
		first := firstRing(m)
		rerung, done := d.ep.Rerung[m.ID]
		switch {
		case !done && now.Sub(first) >= d.ackTimeout:
			a, ok := d.live(m.To)
			if !ok {
				continue
			}
			d.ep.Rerung[m.ID] = now.UTC().Format(time.RFC3339Nano)
			d.save()
			d.ring(ctx, m, a.PaneID, "unread")
		case done && !d.ep.Unacknowledged[m.ID] && now.Sub(parse(rerung)) >= d.ackTimeout:
			d.ep.Unacknowledged[m.ID] = true
			d.save()
			if m.Kind == inbox.Wake || m.From.ID == nil {
				d.log.Printf("unacknowledged %s to %s from %s: logged only", m.ID, m.To, m.From)
				continue
			}
			pane := "-"
			if a, ok := d.live(m.To); ok {
				pane = a.PaneID
			}
			d.wake(ctx, *m.From.ID, "unacknowledged", fmt.Sprintf("Your %s %s to %s is still unread %s after its doorbell rang twice.\nInspect the recipient: fledge agent read --pane %s", m.Kind, m.ID, m.To, 2*d.ackTimeout, pane))
		}
	}
}

// wakeParent writes a wake about rec to its parent, else to the creator of a
// task it owns, else only logs it.
func (d *daemon) wakeParent(ctx context.Context, rec identity.Record, class, body string) {
	to := ""
	if rec.Parent != nil {
		to = *rec.Parent
	} else if tasks, err := owned(d.store, rec.ID); err == nil {
		for _, t := range tasks {
			if t.CreatedBy != nil {
				to = *t.CreatedBy
				break
			}
		}
	}
	if to == "" {
		d.log.Printf("wake %s about %s: no parent or task creator; logged only", class, rec.ID)
		return
	}
	d.wake(ctx, to, class, body)
}

// wake stores a wake for to and rings it at once when to has a live pane.
func (d *daemon) wake(ctx context.Context, to, class, body string) {
	m, err := inbox.Create(d.store, inbox.Record{Kind: inbox.Wake, From: inbox.Supervisor(), To: to, Body: body})
	if err != nil {
		d.log.Printf("wake %s for %s: %v", class, to, err)
		return
	}
	d.log.Printf("wake %s %s for %s", class, m.ID, to)
	if a, ok := d.live(to); ok {
		d.ring(ctx, m, a.PaneID, "wake")
	} else {
		d.log.Printf("wake %s for %s queued: the recipient has no live pane", m.ID, to)
	}
}

// ring types m's doorbell into pane and records the attempt.
func (d *daemon) ring(ctx context.Context, m inbox.Record, pane, why string) {
	title := ""
	if m.Task != nil {
		if t, err := task.Get(d.store, *m.Task); err == nil {
			title = t.Title
		}
	}
	_, err := d.c.Prompt(ctx, pane, inbox.Doorbell(m, title))
	if _, rerr := inbox.AddRing(d.store, m.ID, pane, err); rerr != nil {
		d.log.Printf("ring %s: record attempt: %v", m.ID, rerr)
	}
	if err != nil {
		d.log.Printf("ring %s (%s) in %s failed: %v", m.ID, why, pane, err)
		return
	}
	d.log.Printf("ring %s (%s) in %s", m.ID, why, pane)
}

// live returns the live agent of record id from the last agent.list.
func (d *daemon) live(id string) (herdr.AgentDetails, bool) {
	var rec identity.Record
	if err := d.store.Get(identity.Kind, id, &rec); err != nil || rec.EndedAt != nil {
		return herdr.AgentDetails{}, false
	}
	a, ok := d.agents[rec.TerminalID]
	if !ok || identity.Mismatched(rec, a) {
		return herdr.AgentDetails{}, false
	}
	return a, true
}

// idle reports whether the daemon has had nothing to do for idleExit: no
// watched agent and no queued or unread rung record.
func (d *daemon) idle(now time.Time) bool {
	busy := len(d.watched) > 0
	if !busy {
		msgs, err := inbox.List(d.store)
		busy = err != nil || slices.ContainsFunc(msgs, inbox.Unread)
	}
	if busy {
		d.idleSince = time.Time{}
		return false
	}
	if d.idleSince.IsZero() {
		d.idleSince = now
	}
	return now.Sub(d.idleSince) >= idleExit
}

func (d *daemon) save() {
	if err := d.ep.Save(d.dir); err != nil {
		d.log.Printf("save episodes: %v", err)
	}
}

// owned lists the tasks rec owns.
func owned(s *state.Store, id string) ([]task.Record, error) {
	all, err := task.List(s)
	var mine []task.Record
	for _, t := range all {
		if t.Owner != nil && *t.Owner == id {
			mine = append(mine, t)
		}
	}
	return mine, err
}

func firstRing(m inbox.Record) time.Time {
	for _, r := range m.Rings {
		if r.OK {
			return parse(r.At)
		}
	}
	return time.Time{}
}

func parse(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func name(rec identity.Record) string {
	if rec.Name != nil && *rec.Name != "" {
		return fmt.Sprintf("%s (%s)", *rec.Name, rec.ID)
	}
	return rec.ID
}
