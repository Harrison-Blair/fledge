// Package daemon runs the fledge message daemon: an in-memory view over the
// append-only journal in a flock's directory, served to CLI clients over a
// Unix socket or a sandbox-compatible file bridge. Every state-changing
// operation is journaled before it is
// acknowledged, so a restart replays into the same state.
//
// One daemon serves one flock. Flocks in the same workspace share nothing:
// separate sockets, journals and rosters.
package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/filebridge"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/species"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

// WatchInterval is how often a bound daemon probes its Herdr session.
const WatchInterval = 3 * time.Second

// writeTimeout bounds a single response write so a client that never reads
// cannot pin a handler goroutine. It is a var only so a test can shorten it.
var writeTimeout = 10 * time.Second

// Daemon serves one workspace.
type Daemon struct {
	ln net.Listener
	// journal is an interface, not the *os.File it always is in production, so
	// that a test can count and fail writes.
	journal   io.WriteCloser
	debug     *log.Logger
	done      chan struct{}
	root      string
	flockName string

	// session is the Herdr session this daemon's lifetime is bound to, zero
	// when it runs unbound.
	session herdr.Session

	// titled records that the window title has landed on an attached client.
	// Only the watch goroutine touches it, so it needs no lock.
	titled bool

	mu           sync.Mutex
	agents       map[string]protocol.Agent
	order        []string
	pending      []protocol.Message
	waiters      []*waiter
	readyTokens  map[string]string
	readyWaiters map[string]chan struct{}
	fileOnce     sync.Once
	// skipReadiness is a package-test seam for legacy spawn tests. Production
	// daemons leave it false, so every launch uses authenticated readiness.
	skipReadiness bool
}

// waiter is a blocked wait call. ch is buffered so a delivering sender never
// blocks on it.
type waiter struct {
	as      string
	replyTo string
	ch      chan protocol.Message
	done    bool
}

// maxSocketPath is the size of sun_path in struct sockaddr_un, minus the
// terminating NUL. A longer path cannot be bound at all. Darwin's 104 is the
// stricter of the two platforms fledge runs on, so it is the one enforced.
const maxSocketPath = 103

// socketDir is the directory holding every fledge socket for one workspace.
// Sockets live in the user runtime directory rather than the workspace so that
// a deeply nested or network-mounted workspace can still run a flock: sun_path
// is 108 bytes (104 on darwin), and network filesystems cannot bind unix
// sockets at all. Durable flock state stays under .fledge/flocks/<name>.
//
// The workspace is identified by a hash of its absolute path, which keeps
// concurrent flocks of the same name in different workspaces apart.
func socketDir(root string) string {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		// Unset on macOS and in stripped environments.
		runtime = os.TempDir()
	}
	return filepath.Join(runtime, "fledge", workspace.Hash(root))
}

// SocketPath is the daemon socket for one flock of the workspace at root.
func SocketPath(root, flockName string) string {
	return filepath.Join(socketDir(root), flockName+".sock")
}

func journalPath(root, flockName string) string {
	return filepath.Join(flock.Dir(root, flockName), protocol.JournalName)
}

// lockReclaim takes an exclusive flock(2) on a per-flock lock file in the socket
// directory, so the stale-socket probe/unlink/bind in New runs against no
// concurrent New for the same flock. It blocks until the lock is free and
// returns a release func. The lock file is never removed; only the advisory
// lock matters.
func lockReclaim(root, flockName string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(socketDir(root), flockName+".lock"), os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil // closing the fd releases the flock
}

// CheckSocketPath reports whether a flock's socket path fits the platform
// limit, so start can refuse with a clear error instead of leaving the
// operator to decode a bind failure from the daemon log. The runtime-directory
// scheme makes this practically impossible to trip — the workspace no longer
// contributes any length — so it is a guard against an exotic runtime dir, not
// a limit operators are expected to meet.
func CheckSocketPath(root, flockName string) error {
	sock := SocketPath(root, flockName)
	if len(sock) > maxSocketPath {
		return fmt.Errorf("socket path %s is %d characters, over the %d limit; use a shorter flock name",
			sock, len(sock), maxSocketPath)
	}
	return nil
}

// Run serves the workspace at root until the process is signalled or ctx-less
// listener failure. It replays the journal, binds the socket, and blocks.
func Run(root, flockName string) error {
	return RunBound(root, flockName, "")
}

// RunBound is Run with the daemon's lifetime bound to a Herdr session: when
// that session ends, the daemon exits. An empty session name runs unbound,
// which is what a bare `fledge daemon run` and the tests use.
func RunBound(root, flockName, session string) error {
	d, err := New(root, flockName)
	if err != nil {
		return err
	}
	defer d.Close()

	if session != "" {
		s, found, err := herdr.Find(session)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("no herdr session %q", session)
		}
		d.session = s
		go d.WatchSession(func() bool { return !herdr.Up(s.SocketPath) }, WatchInterval)
	}
	d.consumeReadySignals()
	return d.Serve()
}

// WatchSession polls gone every interval and shuts the daemon's listener the
// first time it reports the Herdr session has ended, which makes Serve return.
// It blocks, so callers run it in a goroutine.
//
// This polls rather than subscribing because Herdr protocol 16 has no
// session-lifecycle event to subscribe to.
func (d *Daemon) WatchSession(gone func() bool, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
			d.assertWindowTitle()
			if gone() {
				d.debug.Printf("herdr session ended; exiting")
				d.ln.Close()
				return
			}
		}
	}
}

// assertWindowTitle brands the session's terminal window as fledge-managed.
// Herdr only applies a window title to an attached client and the operator
// attaches after the daemon starts, so this rides the watch tick until it
// lands instead of being set once at startup. A failure is logged and retried
// on the next tick: the title is decoration, never a reason to fail a flock.
func (d *Daemon) assertWindowTitle() {
	if d.titled || d.session.SocketPath == "" {
		return
	}
	changed, err := herdrwire.WindowTitleSet(d.session.SocketPath, flock.WindowTitle(d.flockName))
	if err != nil {
		d.debug.Printf("window title: %v", err)
		return
	}
	d.titled = changed
}

// New replays the journal, opens the socket, and records daemon.started.
func New(root, flockName string) (*Daemon, error) {
	base := filepath.Join(root, scaffold.DirName)
	if _, err := os.Stat(base); err != nil {
		return nil, fmt.Errorf("no %s directory here; run fledge init", scaffold.DirName)
	}
	if err := flock.Validate(flockName); err != nil {
		return nil, err
	}
	if err := CheckSocketPath(root, flockName); err != nil {
		return nil, err
	}
	// Flock directories are made on demand, not by init: a workspace does not
	// know its flocks until one is started. 0o700 because the flock directory
	// can hold a readiness digest whose file path is bearer-equivalent; Chmod
	// because MkdirAll never downgrades a directory an older fledge left at
	// 0o755.
	flockDir := flock.Dir(root, flockName)
	if err := os.MkdirAll(flockDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(flockDir, 0o700); err != nil {
		return nil, err
	}

	// Only the daemon creates the socket directory. For a client a missing
	// directory is a failed dial, which already means "daemon not running".
	if err := os.MkdirAll(socketDir(root), 0o700); err != nil {
		return nil, err
	}

	// Establish ownership before replay, which now mutates the journal (it
	// re-terminates or truncates a torn tail). Reclaiming a stale socket is
	// probe, then unlink, then bind. Two concurrent New calls for the same flock
	// could otherwise both probe empty, and the loser's unlink would delete the
	// winner's live socket, forking the journal and state authority. An
	// exclusive flock(2) held across the whole sequence serializes them: the
	// loser blocks, then probes a now-live socket and is refused here.
	unlock, err := lockReclaim(root, flockName)
	if err != nil {
		return nil, err
	}
	defer unlock()

	sock := SocketPath(root, flockName)
	// A leftover socket file from a crashed daemon would block bind; a live
	// one would not, so probe before removing.
	if c, err := net.Dial("unix", sock); err == nil {
		c.Close()
		return nil, errors.New("daemon already running")
	}

	// Ownership is ours; only now replay the journal, which may rewrite its tail.
	s, err := replay(journalPath(root, flockName))
	if err != nil {
		return nil, err
	}

	os.Remove(sock)

	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}

	journal, err := os.OpenFile(journalPath(root, flockName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		ln.Close()
		return nil, err
	}
	// Tighten a journal an older fledge created at 0o644; O_CREATE's mode is
	// ignored for an existing file.
	if err := journal.Chmod(0o600); err != nil {
		journal.Close()
		ln.Close()
		return nil, err
	}

	debugFile, err := os.OpenFile(filepath.Join(flock.Dir(root, flockName), protocol.LogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		journal.Close()
		ln.Close()
		return nil, err
	}
	if err := filebridge.ResetServer(root, flockName); err != nil {
		debugFile.Close()
		journal.Close()
		ln.Close()
		return nil, fmt.Errorf("file RPC: %w", err)
	}

	d := &Daemon{
		ln:           ln,
		journal:      journal,
		debug:        log.New(debugFile, "", log.LstdFlags),
		done:         make(chan struct{}),
		root:         root,
		flockName:    flockName,
		agents:       s.agents,
		order:        s.order,
		pending:      s.pending,
		readyTokens:  s.tokens,
		readyWaiters: make(map[string]chan struct{}),
	}
	if err := d.append(event{Event: evStarted}); err != nil {
		d.Close()
		return nil, err
	}
	d.debug.Printf("started: %d agents, %d pending", len(d.agents), len(d.pending))
	return d, nil
}

// Close releases the listener, socket file, and log handles. Agents live in
// herdr panes, which outlive the daemon by design.
func (d *Daemon) Close() error {
	if d.done != nil {
		select {
		case <-d.done:
		default:
			close(d.done)
		}
	}
	if d.ln != nil {
		d.ln.Close()
	}
	if err := filebridge.CloseServer(d.root, d.flockName); err != nil {
		d.debug.Printf("close file RPC: %v", err)
	}
	if d.journal != nil {
		d.journal.Close()
	}
	return nil
}

// Serve accepts connections until the listener is closed. A temporary Accept
// error (e.g. EMFILE under fd pressure) is retried with capped backoff rather
// than mistaken for shutdown; only an intentional listener close returns nil,
// and any other error is returned so the fault is not hidden as a clean exit.
func (d *Daemon) Serve() error {
	d.fileOnce.Do(func() { go d.serveFileRequests() })
	var backoff time.Duration
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Temporary() {
				if backoff == 0 {
					backoff = 5 * time.Millisecond
				} else {
					backoff *= 2
				}
				if backoff > time.Second {
					backoff = time.Second
				}
				d.debug.Printf("accept: %v; retrying in %s", err, backoff)
				time.Sleep(backoff)
				continue
			}
			return err
		}
		backoff = 0
		go d.handle(conn)
	}
}

func (d *Daemon) serveFileRequests() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
		}
		pending, err := filebridge.Take(d.root, d.flockName)
		if err != nil {
			d.debug.Printf("file RPC: %v", err)
			continue
		}
		for _, p := range pending {
			p := p
			go func() {
				// A bridge wait has no connection to watch, so give it a
				// liveness signal built from the exchange's accepted marker: a
				// client that gives up removes it via Cleanup. Without this a
				// killed sandboxed `agent msg wait` (which sends no timeout)
				// would park in d.waiters forever and swallow the next message.
				var gone chan struct{}
				if p.Request.Op == protocol.OpWait {
					gone = make(chan struct{})
					done := make(chan struct{})
					defer close(done)
					go d.watchBridgeWaiter(p.ID, gone, done)
				}
				resp := d.dispatch(&p.Request, gone)
				if err := filebridge.Respond(d.root, d.flockName, p.ID, resp); err != nil {
					d.debug.Printf("file RPC response %s: %v", p.ID, err)
				}
			}()
		}
	}
}

// watchBridgeWaiter closes gone once the bridge client abandons exchange id,
// determined by filebridge.Awaiting: the accepted marker is gone (graceful
// Cleanup) or the client pid stamped in it no longer runs (a killed client
// that ran no defer). This mirrors the socket path's connection-death signal
// so an abandoned bridge wait is dropped instead of swallowing the next
// message. The probe is cheap; the poll is kept short to narrow the window in
// which a send can still be handed to an already-dead waiter (the accepted,
// gone-branch-tolerated crash window). Returns when done or d.done close.
func (d *Daemon) watchBridgeWaiter(id string, gone chan struct{}, done <-chan struct{}) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-d.done:
			return
		case <-ticker.C:
			if !filebridge.Awaiting(d.root, d.flockName, id) {
				close(gone)
				return
			}
		}
	}
}

func (d *Daemon) handle(conn net.Conn) {
	defer conn.Close()

	var req protocol.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	// For wait, watch the connection so a client that dies mid-wait has its
	// waiter dropped instead of silently swallowing the next message.
	var gone chan struct{}
	if req.Op == protocol.OpWait {
		gone = make(chan struct{})
		go func() {
			io.Copy(io.Discard, conn)
			close(gone)
		}()
	}

	resp := d.dispatch(&req, gone)
	line, err := json.Marshal(resp)
	if err != nil {
		d.debug.Printf("marshal response: %v", err)
		return
	}
	// Bound the write so a client that stops reading cannot pin this goroutine.
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := conn.Write(append(line, '\n')); err != nil {
		d.debug.Printf("write response: %v", err)
	}
}

func (d *Daemon) dispatch(req *protocol.Request, gone <-chan struct{}) protocol.Response {
	var (
		resp protocol.Response
		err  error
	)
	switch req.Op {
	case protocol.OpRegister:
		resp, err = d.register(req)
	case protocol.OpList:
		resp = protocol.Response{Agents: d.list()}
	case protocol.OpStatus:
		resp = protocol.Response{
			Session:       d.session.Name,
			SessionSocket: d.session.SocketPath,
			Agents:        d.list(),
		}
	case protocol.OpSend:
		resp, err = d.send(req)
	case protocol.OpWait:
		resp, err = d.wait(req, gone)
	case protocol.OpSpawn:
		resp, err = d.spawn(req)
	case protocol.OpReady:
		resp, err = d.ready(req)
	case protocol.OpStop:
		resp, err = d.stop(req)
	default:
		err = fmt.Errorf("unknown op %q", req.Op)
	}
	if err != nil {
		d.debug.Printf("%s: %v", req.Op, err)
		return protocol.Response{Error: err.Error()}
	}
	return resp
}

// alive reports whether pid still names a running process. A registration
// whose process is gone frees its name for reuse. EPERM means the process
// exists but is not ours to signal, which is still alive.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func (d *Daemon) register(req *protocol.Request) (protocol.Response, error) {
	if err := validType(req.Type); err != nil {
		return protocol.Response{}, err
	}
	if strings.HasPrefix(req.Type, "fledge-") && req.Agent == "" {
		return protocol.Response{}, errors.New("the fledge-* namespace is reserved for managed definitions")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// The species pool is per-type: a slug is taken only if this type already
	// holds it under a live process.
	taken := func(slug string) bool {
		a, ok := d.agents[req.Type+"-"+slug]
		return ok && alive(a.PID)
	}

	var name, slug string
	if req.Type == agentcfg.ReservedOrchestrator {
		// The reserved name is fixed, so its pool is that one name rather than
		// the species list — matching how spawn treats it, so an orchestrator
		// is the same agent whichever way it joined the roster. A second live
		// registration collides exactly as an exhausted species pool does.
		if req.Species != "" {
			return protocol.Response{}, fmt.Errorf("%s takes no species", req.Type)
		}
		if a, ok := d.agents[req.Type]; ok && alive(a.PID) {
			return protocol.Response{}, fmt.Errorf("%s is already running", req.Type)
		}
		name = req.Type
	} else {
		picked, err := species.Pick(taken, req.Species)
		if err != nil {
			return protocol.Response{}, fmt.Errorf("%s: %w", req.Type, err)
		}
		name, slug = req.Type+"-"+picked, picked
	}

	if _, ok := d.agents[name]; !ok {
		d.order = append(d.order, name)
	}
	d.agents[name] = protocol.Agent{Name: name, Type: req.Type, Species: slug, PID: req.PID}
	a := d.agents[name]
	a.Agent, a.Profile, a.Source = req.Agent, req.Profile, req.Source
	d.agents[name] = a

	if err := d.append(event{Event: evRegistered, Name: name, Type: req.Type, Species: slug, PID: req.PID, Agent: req.Agent, Profile: req.Profile, Source: req.Source}); err != nil {
		return protocol.Response{}, err
	}
	d.debug.Printf("registered %s pid=%d", name, req.PID)
	return protocol.Response{Name: name}, nil
}

// validType accepts lowercase alphanumerics only, so that the first hyphen in
// a full name always separates type from species.
// validType mirrors agentcfg's naming rule, including its one carve-out: the
// reserved orchestrator name is hyphenated by design, so it has to survive
// both validation seams or the profile could never be spawned.
func validType(t string) error {
	if t == "" {
		return errors.New("missing agent type")
	}
	if strings.HasPrefix(t, "-") || strings.HasSuffix(t, "-") || strings.Contains(t, "--") {
		return fmt.Errorf("invalid type %q: use kebab-case", t)
	}
	for _, r := range t {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return fmt.Errorf("invalid type %q: use kebab-case", t)
		}
	}
	return nil
}

func (d *Daemon) list() []protocol.Agent {
	d.mu.Lock()
	defer d.mu.Unlock()

	agents := make([]protocol.Agent, 0, len(d.order))
	for _, name := range d.order {
		a := d.agents[name]
		a.Alive = alive(a.PID)
		agents = append(agents, a)
	}
	return agents
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (d *Daemon) send(req *protocol.Request) (protocol.Response, error) {
	if req.From == "" {
		return protocol.Response{}, errors.New("missing --from")
	}
	if req.To == "" {
		return protocol.Response{}, errors.New("missing recipient")
	}

	id, err := newID()
	if err != nil {
		return protocol.Response{}, err
	}
	msg := protocol.Message{ID: id, From: req.From, To: req.To, Body: req.Body, ReplyTo: req.ReplyTo}

	d.mu.Lock()

	to, ok := d.agents[req.To]
	if !ok {
		d.mu.Unlock()
		return protocol.Response{}, fmt.Errorf("no registered agent %q", req.To)
	}

	if err := d.append(event{
		Event: evSent, ID: id, From: msg.From, To: msg.To, Body: msg.Body, ReplyTo: msg.ReplyTo,
	}); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}

	// A spawned agent is driven, not polled: its message goes straight into the
	// pane rather than waiting for a wait to claim it.
	if bridged(to) {
		d.mu.Unlock()

		// A hand-off that fails puts the message on the pending queue, exactly
		// as an unbridged send would: the live queue is what holds it, and
		// journaling no delivery line is what makes a replay agree.
		if err := d.bridge(to, msg); err != nil {
			d.mu.Lock()
			// Same disposal as an unbridged send: a parked wait takes it, and
			// only an unwanted message queues.
			if w := d.matchWaiter(msg); w != nil {
				if derr := d.deliver(w, msg); derr != nil {
					d.debug.Printf("deliver %s after failed bridge: %v", msg.ID, derr)
					d.pending = append(d.pending, msg)
				}
			} else {
				d.pending = append(d.pending, msg)
			}
			d.mu.Unlock()
			return protocol.Response{}, err
		}

		d.mu.Lock()
		defer d.mu.Unlock()
		if err := d.append(event{Event: evDelivered, ID: msg.ID, To: msg.To}); err != nil {
			return protocol.Response{}, err
		}
		d.debug.Printf("bridged %s %s -> %s", id, msg.From, msg.To)
		return protocol.Response{ID: id}, nil
	}

	defer d.mu.Unlock()

	if w := d.matchWaiter(msg); w != nil {
		if err := d.deliver(w, msg); err != nil {
			return protocol.Response{}, err
		}
	} else {
		d.pending = append(d.pending, msg)
	}

	d.debug.Printf("sent %s %s -> %s", id, msg.From, msg.To)
	return protocol.Response{ID: id}, nil
}

// matchWaiter finds the first live waiter msg satisfies, removing it from the
// waiter list. Caller holds d.mu.
func (d *Daemon) matchWaiter(msg protocol.Message) *waiter {
	for i, w := range d.waiters {
		if w.done || !w.matches(msg) {
			continue
		}
		w.done = true
		d.waiters = append(d.waiters[:i], d.waiters[i+1:]...)
		return w
	}
	return nil
}

// matches reports whether msg satisfies this wait. A wait with --reply-to
// takes only the correlated reply; anything else stays pending.
func (w *waiter) matches(msg protocol.Message) bool {
	if msg.To != w.as {
		return false
	}
	if w.replyTo != "" && msg.ReplyTo != w.replyTo {
		return false
	}
	return true
}

// deliver journals the delivery and hands the message to the waiter. Caller
// holds d.mu.
func (d *Daemon) deliver(w *waiter, msg protocol.Message) error {
	if err := d.append(event{Event: evDelivered, ID: msg.ID, To: msg.To}); err != nil {
		return err
	}
	w.ch <- msg
	d.debug.Printf("delivered %s to %s", msg.ID, msg.To)
	return nil
}

func (d *Daemon) wait(req *protocol.Request, gone <-chan struct{}) (protocol.Response, error) {
	if req.As == "" {
		return protocol.Response{}, errors.New("missing --as")
	}

	w := &waiter{as: req.As, replyTo: req.ReplyTo, ch: make(chan protocol.Message, 1)}

	d.mu.Lock()
	if _, ok := d.agents[req.As]; !ok {
		d.mu.Unlock()
		return protocol.Response{}, fmt.Errorf("no registered agent %q", req.As)
	}
	// Pending messages are consumed in send order.
	for i, msg := range d.pending {
		if !w.matches(msg) {
			continue
		}
		d.pending = append(d.pending[:i], d.pending[i+1:]...)
		if err := d.deliver(w, msg); err != nil {
			d.mu.Unlock()
			return protocol.Response{}, err
		}
		d.mu.Unlock()
		return protocol.Response{Message: &msg}, nil
	}
	d.waiters = append(d.waiters, w)
	d.mu.Unlock()

	var timeout <-chan time.Time
	if req.TimeoutMS > 0 {
		t := time.NewTimer(time.Duration(req.TimeoutMS) * time.Millisecond)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case msg := <-w.ch:
		return protocol.Response{Message: &msg}, nil
	case <-timeout:
		d.mu.Lock()
		defer d.mu.Unlock()
		// A sender may have claimed this waiter between the timer firing and
		// the lock; that delivery is already journaled, so honor it.
		select {
		case msg := <-w.ch:
			return protocol.Response{Message: &msg}, nil
		default:
		}
		d.dropWaiter(w)
		return protocol.Response{}, errors.New("timed out waiting for a message")
	case <-gone:
		d.mu.Lock()
		defer d.mu.Unlock()
		select {
		case msg := <-w.ch:
			// Already journaled as delivered but the client is gone; the
			// response write will fail. This is the accepted crash window.
			d.debug.Printf("delivered %s to %s, but its wait disconnected", msg.ID, msg.To)
			return protocol.Response{Message: &msg}, nil
		default:
		}
		d.dropWaiter(w)
		return protocol.Response{}, errors.New("wait abandoned")
	}
}

// dropWaiter removes an abandoned waiter. Caller holds d.mu.
func (d *Daemon) dropWaiter(w *waiter) {
	for i, other := range d.waiters {
		if other == w {
			d.waiters = append(d.waiters[:i], d.waiters[i+1:]...)
			return
		}
	}
}
