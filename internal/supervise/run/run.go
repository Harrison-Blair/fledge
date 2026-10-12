package run

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/fledgedir"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/repoconfig"
	"github.com/Harrison-Blair/fledge/internal/lib/supervise"
)

// Loop timings.
const (
	reconcileEvery = 30 * time.Second
	heartbeatEvery = 10 * time.Second
	tickEvery      = time.Second
	defaultAck     = 5 * time.Minute
)

// Options configures one daemon. Cwd locates the repository; Version is
// recorded in the lock so a starter of another version replaces it; Log
// receives one line per event, wake, ring, reconnect, start, and exit.
type Options struct {
	Cwd     string
	Version string
	Log     io.Writer
	// subscribe opens event streams; nil uses the client's Herdr socket.
	subscribe subscriber
	now       func() time.Time
}

// Run is the daemon of the caller's Herdr session: it holds the session's
// lock until ctx ends or, for 60 seconds, it has nothing to watch and no
// unread record to ring. A daemon that finds the lock held exits at once
// without a word. It ignores the caller's pane: c must not carry one.
func Run(ctx context.Context, c libagent.Client, o Options) error {
	if o.now == nil {
		o.now = time.Now
	}
	if o.subscribe == nil {
		o.subscribe = socketSubscriber(os.Getenv("HERDR_SOCKET_PATH"))
	}
	c.CallerPane = ""
	root, err := fledgedir.Root(ctx, o.Cwd)
	if err != nil {
		return err
	}
	session := os.Getenv("HERDR_SESSION")
	dir := supervise.Dir(root, session)
	lock, err := supervise.Acquire(dir)
	if errors.Is(err, supervise.ErrHeld) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Release()
	logger := log.New(o.Log, "", log.LstdFlags)
	started := o.now()
	logger.Printf("start pid %d version %s session %q", os.Getpid(), o.Version, session)
	status := supervise.Status{PID: os.Getpid(), Version: o.Version, StartedAt: started.UTC().Format(time.RFC3339Nano)}
	beat := func(watched []string) {
		status.HeartbeatAt, status.Watched = o.now().UTC().Format(time.RFC3339Nano), watched
		if err := lock.Write(status); err != nil {
			logger.Printf("heartbeat: %v", err)
		}
	}
	beat([]string{})
	s, err := identity.Existing(ctx, root)
	if err != nil || s == nil {
		logger.Printf("exit: no Fledge state in %s: %v", root, err)
		return err
	}
	ack := defaultAck
	if cfg, err := repoconfig.Read(ctx, root); err != nil {
		logger.Printf("config: %v; ack_timeout defaults to %s", err, defaultAck)
	} else if cfg.Inbox.AckTimeout > 0 {
		ack = cfg.Inbox.AckTimeout
	}
	d, err := newDaemon(c, s, dir, ack, logger)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events := make(chan event, 64)
	go keep(ctx, o.subscribe, globalSubscriptions, events, "", logger)
	subs := map[string]context.CancelFunc{}
	sync := func() {
		watched := map[string]bool{}
		for _, p := range d.panes() {
			watched[p] = true
			if subs[p] == nil {
				sub, stop := context.WithCancel(ctx)
				subs[p] = stop
				go keep(sub, o.subscribe, paneSubscriptions(p), events, p, logger)
			}
		}
		for p, stop := range subs {
			if !watched[p] {
				stop()
				delete(subs, p)
			}
		}
	}
	reconcile := func(now time.Time) {
		if err := d.reconcile(ctx, now); err != nil {
			logger.Printf("reconcile: %v", err)
		}
		sync()
	}
	now := o.now()
	reconcile(now)
	beat(d.watchedIDs())
	lastReconcile, lastBeat := now, now
	ticker := time.NewTicker(tickEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Printf("exit: stopped")
			return nil
		case e := <-events:
			now := o.now()
			switch e.kind {
			case "status":
				d.status(ctx, e.pane, e.status, now)
			case "exited":
				d.exitedPane(ctx, e.pane)
				sync()
			case "moved":
				d.moved(e.previous, e.pane)
				sync()
			case "drop":
				d.drop(e.pane)
				reconcile(now)
			case "detected":
				reconcile(now)
			case "subscribed":
				if e.retry {
					reconcile(now)
				}
			}
		case <-ticker.C:
			now := o.now()
			d.tick(ctx, now)
			if now.Sub(lastReconcile) >= reconcileEvery {
				reconcile(now)
				lastReconcile = now
			}
			if now.Sub(lastBeat) >= heartbeatEvery {
				beat(d.watchedIDs())
				lastBeat = now
			}
			if d.idle(now) {
				logger.Printf("exit: nothing to watch for %s", idleExit)
				return nil
			}
		}
	}
}
