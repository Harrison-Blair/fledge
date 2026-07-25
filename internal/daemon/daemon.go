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
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	"github.com/Harrison-Blair/fledge/internal/contextdoc"
	"github.com/Harrison-Blair/fledge/internal/filebridge"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/herdrwire"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/species"
	"github.com/Harrison-Blair/fledge/internal/version"
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
	debugFile io.WriteCloser
	debug     *log.Logger
	done      chan struct{}
	root      string
	flockName string
	unlock    func()

	// session is the Herdr session this daemon's lifetime is bound to, zero
	// when it runs unbound.
	session herdr.Session

	// titled records that the window title has landed on an attached client.
	// Only the watch goroutine touches it, so it needs no lock.
	titled bool

	mu                 sync.Mutex
	agents             map[string]protocol.Agent
	order              []string
	pending            []protocol.Message
	notifyPending      []protocol.Message
	inboxNotifyArmed   map[string]bool
	inboxNotified      map[string]bool
	messages           map[string]protocol.Message
	messageOrder       []string
	messageDelivered   map[string]bool
	inboxNotify        chan struct{}
	inboxNotifyDone    chan struct{}
	inboxNotifyTasks   map[string]*inboxNotifyTask
	inboxNotifyFlights map[string]*inboxNotifyFlight
	inboxWake          inboxWakeFunc
	inboxWakeCancel    context.CancelFunc
	inboxNotifyStarted bool
	closing            bool
	stopping           map[string]bool
	waiters            []*waiter
	readyTokens        map[string]string
	identityTokens     map[string]string
	readyWaiters       map[string]chan struct{}
	launches           map[string]*launchLatch
	ownedTabs          map[string]ownedTab
	tabCreateIntents   map[string]pendingTabCreate
	tabCreates         map[string]*tabCreateLatch
	tabShells          map[string]*tabShellLatch
	closingTabs        map[string]bool
	closingWorkspaces  map[string]bool
	tabClosures        map[string]tabRecord
	workspaceClosures  map[string]event
	tabCloseRuns       map[string]*closeLatch
	workspaceCloseRuns map[string]*closeLatch
	fileOnce           sync.Once
	fileStopOnce       sync.Once
	fileStop           chan struct{}
	fileMu             sync.Mutex
	closeOnce          sync.Once
	finalCloseOnce     sync.Once
	closeDone          chan struct{}
	closeErr           error
	active             int
	shutdownDrained    bool
	// skipReadiness is a package-test seam for legacy spawn tests. Production
	// daemons leave it false, so every launch uses authenticated readiness.
	skipReadiness bool
}

// waiter is a blocked wait call. ch is buffered so a delivering sender never
// blocks on it.
type waiter struct {
	as          string
	from        string
	replyTo     string
	acknowledge bool
	ch          chan waiterResult
	cancel      chan struct{}
	done        bool
}

type waiterResult struct {
	msg protocol.Message
	err error
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

func liveSocket(sock string) bool {
	c, err := net.Dial("unix", sock)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// lockOwnership takes an exclusive flock(2) on a per-flock lock file in the
// socket directory. The daemon holds it for its whole lifetime, so a
// replacement blocks until the old daemon has closed every request path and
// durable handle. While waiting, a live socket means this is just another
// healthy start attempt, so it fails immediately instead of waiting for exit.
func lockOwnership(root, flockName, sock string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(socketDir(root), flockName+".lock"), os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil // closing the fd releases the flock
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			return nil, err
		}
		if liveSocket(sock) {
			f.Close()
			return nil, errors.New("daemon already running")
		}
		time.Sleep(10 * time.Millisecond)
	}
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
		if err := d.recoverOwnedTabs(); err != nil {
			return err
		}
		d.replayInboxNotifications()
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

	sock := SocketPath(root, flockName)
	// A leftover socket file from a crashed daemon would block bind; a live
	// one would not, so probe before removing.
	if liveSocket(sock) {
		return nil, errors.New("daemon already running")
	}

	// Establish ownership before replay, which now mutates the journal (it
	// re-terminates or truncates a torn tail). Reclaiming a stale socket is
	// probe, then unlink, then bind. Two concurrent New calls for the same flock
	// could otherwise both probe empty, and the loser's unlink would delete the
	// winner's live socket, forking the journal and state authority. The lock is
	// held until final daemon close so an in-place replacement cannot replay the
	// journal before the old daemon has drained and closed its handles.
	unlock, err := lockOwnership(root, flockName, sock)
	if err != nil {
		return nil, err
	}
	locked := true
	release := func() {
		if locked {
			locked = false
			unlock()
		}
	}

	if liveSocket(sock) {
		release()
		return nil, errors.New("daemon already running")
	}

	// Ownership is ours; only now replay the journal, which may rewrite its tail.
	s, err := replay(journalPath(root, flockName))
	if err != nil {
		release()
		return nil, err
	}

	os.Remove(sock)

	ln, err := net.Listen("unix", sock)
	if err != nil {
		release()
		return nil, err
	}

	journal, err := os.OpenFile(journalPath(root, flockName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		ln.Close()
		release()
		return nil, err
	}
	// Tighten a journal an older fledge created at 0o644; O_CREATE's mode is
	// ignored for an existing file.
	if err := journal.Chmod(0o600); err != nil {
		journal.Close()
		ln.Close()
		release()
		return nil, err
	}

	debugFile, err := os.OpenFile(filepath.Join(flock.Dir(root, flockName), protocol.LogName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		journal.Close()
		ln.Close()
		release()
		return nil, err
	}
	if err := filebridge.ResetServer(root, flockName); err != nil {
		debugFile.Close()
		journal.Close()
		ln.Close()
		release()
		return nil, fmt.Errorf("file RPC: %w", err)
	}

	d := &Daemon{
		ln:                 ln,
		journal:            journal,
		debug:              log.New(debugFile, "", log.LstdFlags),
		debugFile:          debugFile,
		done:               make(chan struct{}),
		closeDone:          make(chan struct{}),
		fileStop:           make(chan struct{}),
		root:               root,
		flockName:          flockName,
		unlock:             unlock,
		agents:             s.agents,
		order:              s.order,
		pending:            s.pending,
		notifyPending:      s.notifyPending,
		inboxNotifyArmed:   s.inboxNotifyArmed,
		inboxNotified:      s.inboxNotified,
		messages:           s.messages,
		messageOrder:       s.messageOrder,
		messageDelivered:   s.messageDelivered,
		inboxNotify:        make(chan struct{}, 1),
		inboxNotifyDone:    make(chan struct{}),
		inboxNotifyTasks:   make(map[string]*inboxNotifyTask),
		inboxNotifyFlights: make(map[string]*inboxNotifyFlight),
		stopping:           make(map[string]bool),
		readyTokens:        s.tokens,
		identityTokens:     s.credentials,
		readyWaiters:       make(map[string]chan struct{}),
		launches:           make(map[string]*launchLatch),
		ownedTabs:          s.ownedTabs,
		tabCreateIntents:   s.tabCreateIntents,
		tabCreates:         make(map[string]*tabCreateLatch),
		tabShells:          make(map[string]*tabShellLatch),
		closingTabs:        make(map[string]bool),
		closingWorkspaces:  make(map[string]bool),
		tabClosures:        s.tabClosures,
		workspaceClosures:  s.workspaceClosures,
		tabCloseRuns:       make(map[string]*closeLatch),
		workspaceCloseRuns: make(map[string]*closeLatch),
	}
	for tabID := range d.tabClosures {
		d.closingTabs[tabID] = true
	}
	for workspaceID := range d.workspaceClosures {
		d.closingWorkspaces[workspaceID] = true
	}
	if err := d.append(event{Event: evStarted}); err != nil {
		d.Close()
		return nil, err
	}
	locked = false
	d.logStateSummary()
	d.startInboxNotifier()
	return d, nil
}

func (d *Daemon) logStateSummary() {
	d.debug.Printf("started: %d agents, %d pending", len(d.listLocked()), len(d.pending))
}

// Close begins shutdown and waits until every accepted request has responded,
// the notifier has stopped, durable handles are closed, and ownership is
// released. Agents live in herdr panes, which outlive the daemon by design.
func (d *Daemon) Close() error {
	d.initiateShutdown()
	if d.closeDone != nil {
		<-d.closeDone
	}
	return d.closeErr
}

func (d *Daemon) beginRequest() {
	d.mu.Lock()
	d.active++
	d.mu.Unlock()
}

func (d *Daemon) endRequest() {
	d.mu.Lock()
	if d.active > 0 {
		d.active--
	}
	shouldClose := d.closing && d.shutdownDrained && d.active == 0
	d.mu.Unlock()
	if shouldClose {
		d.finalClose()
	}
}

func (d *Daemon) maybeFinalClose() {
	d.mu.Lock()
	shouldClose := d.closing && d.shutdownDrained && d.active == 0
	d.mu.Unlock()
	if shouldClose {
		d.finalClose()
	}
}

func (d *Daemon) initiateShutdown() {
	d.closeOnce.Do(func() {
		var waiters []*waiter
		d.mu.Lock()
		d.closing = true
		for name := range d.inboxNotifyArmed {
			d.inboxNotifyArmed[name] = false
		}
		d.inboxNotifyTasks = make(map[string]*inboxNotifyTask)
		for _, w := range d.waiters {
			if w.done {
				continue
			}
			w.done = true
			waiters = append(waiters, w)
		}
		d.waiters = nil
		cancel := d.inboxWakeCancel
		d.mu.Unlock()

		for _, w := range waiters {
			w.ch <- waiterResult{err: errors.New("daemon is shutting down")}
		}
		if cancel != nil {
			cancel()
		}
		if d.ln != nil {
			d.ln.Close()
		}
		d.stopFilePolling()
		d.drainFileRequests()
		d.mu.Lock()
		d.shutdownDrained = true
		d.mu.Unlock()
		d.maybeFinalClose()
	})
}

func (d *Daemon) stopFilePolling() {
	d.fileStopOnce.Do(func() {
		if d.fileStop != nil {
			close(d.fileStop)
		}
	})
}

func (d *Daemon) finalClose() {
	d.finalCloseOnce.Do(func() {
		if d.done != nil {
			close(d.done)
		}
		if err := filebridge.CloseServer(d.root, d.flockName); err != nil && d.debug != nil {
			d.debug.Printf("close file RPC: %v", err)
		}

		d.mu.Lock()
		started := d.inboxNotifyStarted
		done := d.inboxNotifyDone
		d.mu.Unlock()
		// A notifier may still be returning from an integration command. Join
		// it before closing the journal and log it can still use.
		if started && done != nil {
			<-done
		}
		if d.journal != nil {
			if err := d.journal.Close(); err != nil {
				d.closeErr = err
			}
		}
		if d.debugFile != nil {
			if err := d.debugFile.Close(); err != nil && d.closeErr == nil {
				d.closeErr = err
			}
		}
		if d.unlock != nil {
			d.unlock()
		}
		if d.closeDone != nil {
			close(d.closeDone)
		}
	})
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
		case <-d.fileStop:
			return
		case <-ticker.C:
		}
		d.drainFileRequests()
	}
}

func (d *Daemon) drainFileRequests() {
	d.fileMu.Lock()
	defer d.fileMu.Unlock()

	pending, err := filebridge.Take(d.root, d.flockName)
	if err != nil {
		if d.debug != nil {
			d.debug.Printf("file RPC: %v", err)
		}
		return
	}
	for _, p := range pending {
		p := p
		d.beginRequest()
		go func() {
			defer d.endRequest()
			// A bridge wait has no connection to watch, so give it a
			// liveness signal built from the exchange's accepted marker: a
			// client that gives up removes it via Cleanup. Without this a
			// killed sandboxed `agent msg wait` (which sends no timeout)
			// would park in d.waiters forever and swallow the next message.
			var gone chan struct{}
			if p.Request.Op == protocol.OpWait || p.Request.Op == protocol.OpReceive {
				gone = make(chan struct{})
				done := make(chan struct{})
				defer close(done)
				go d.watchBridgeWaiter(p.ID, gone, done)
			}
			resp := d.dispatch(&p.Request, gone)
			if err := filebridge.Respond(d.root, d.flockName, p.ID, resp); err != nil && d.debug != nil {
				d.debug.Printf("file RPC response %s: %v", p.ID, err)
			}
			if p.Request.Op == protocol.OpShutdown {
				d.waitBridgeCleanup(p.ID, time.Second)
			}
		}()
	}
}

func (d *Daemon) waitBridgeCleanup(id string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for filebridge.Awaiting(d.root, d.flockName, id) {
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
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
	d.beginRequest()
	defer d.endRequest()

	var req protocol.Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}

	// For blocking mailbox operations, watch the connection so a client that
	// dies mid-wait has its waiter dropped instead of swallowing the next
	// message.
	var gone chan struct{}
	if req.Op == protocol.OpWait || req.Op == protocol.OpReceive {
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
			DaemonPID:     os.Getpid(),
			DaemonVersion: version.Get(),
			Agents:        d.list(),
		}
	case protocol.OpSend:
		resp, err = d.send(req)
	case protocol.OpReply:
		resp, err = d.reply(req)
	case protocol.OpInbox:
		resp, err = d.inbox(req)
	case protocol.OpWait:
		resp, err = d.wait(req, gone)
	case protocol.OpPeek:
		resp, err = d.peek(req)
	case protocol.OpReceive:
		resp, err = d.receive(req, gone)
	case protocol.OpAck:
		resp, err = d.ack(req)
	case protocol.OpSpawn:
		resp, err = d.spawn(req)
	case protocol.OpReady:
		resp, err = d.ready(req)
	case protocol.OpStop:
		resp, err = d.stop(req)
	case protocol.OpShutdown:
		d.initiateShutdown()
		resp = protocol.Response{DaemonPID: os.Getpid(), DaemonVersion: version.Get()}
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
	if d.closing {
		return protocol.Response{}, errors.New("daemon is closing")
	}

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

	return d.listLocked()
}

// listLocked projects the current roster from the durable lifecycle state.
// Stopped agents remain in d.agents for replay, credential rejection, and name
// reuse, but current roster views omit them. Caller holds d.mu, except during
// single-threaded daemon construction before the daemon is published.
func (d *Daemon) listLocked() []protocol.Agent {
	agents := make([]protocol.Agent, 0, len(d.order))
	for _, name := range d.order {
		a := d.agents[name]
		if a.State == stateStopped {
			continue
		}
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
		return protocol.Response{}, errors.New("missing sender")
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
	if d.closing {
		d.mu.Unlock()
		return protocol.Response{}, errors.New("daemon is closing")
	}

	if err := d.authorizeMessageActorLocked(req.From, req.Token, req.Credential); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}
	if err := d.validateMessageRecipientLocked(req.To); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}
	if err := d.validateManagedContextMessageLocked(msg); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}

	if err := d.append(event{
		Event: evSent, ID: id, From: msg.From, To: msg.To, Body: msg.Body, ReplyTo: msg.ReplyTo,
	}); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}
	d.messages[id] = msg
	d.messageOrder = append(d.messageOrder, id)

	defer d.mu.Unlock()

	notify := d.shouldNotifyInboxLocked(msg)
	if w := d.matchWaiter(msg); w != nil {
		if w.acknowledge {
			d.pending = append(d.pending, msg)
			d.offer(w, msg)
		} else {
			if err := d.deliver(w, msg); err != nil {
				d.pending = append(d.pending, msg)
				d.failWaiter(w, err)
				if notify {
					d.queueInboxWakeLocked(msg.To, 0, time.Time{})
				}
				return protocol.Response{}, err
			}
		}
	} else {
		d.pending = append(d.pending, msg)
	}

	d.debug.Printf("sent %s %s -> %s", id, msg.From, msg.To)
	if notify {
		d.queueInboxWakeLocked(msg.To, 0, time.Time{})
	}
	return protocol.Response{ID: id}, nil
}

func (d *Daemon) reply(req *protocol.Request) (protocol.Response, error) {
	if req.From == "" {
		return protocol.Response{}, errors.New("missing sender")
	}
	if req.ID == "" {
		return protocol.Response{}, errors.New("missing inbound message id")
	}
	id, err := newID()
	if err != nil {
		return protocol.Response{}, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return protocol.Response{}, errors.New("daemon is closing")
	}
	if err := d.authorizeMessageActorLocked(req.From, req.Token, req.Credential); err != nil {
		return protocol.Response{}, err
	}
	original, ok := d.messages[req.ID]
	if !ok || original.To != req.From {
		return protocol.Response{}, fmt.Errorf("message %q is not inbound to %q", req.ID, req.From)
	}
	if !d.messageDelivered[req.ID] {
		return protocol.Response{}, fmt.Errorf("message %q has not been claimed by %q", req.ID, req.From)
	}
	if err := d.validateMessageRecipientLocked(original.From); err != nil {
		return protocol.Response{}, fmt.Errorf("original sender cannot receive reply: %w", err)
	}
	msg := protocol.Message{
		ID: id, From: req.From, To: original.From, Body: req.Body, ReplyTo: original.ID,
	}
	if err := d.validateManagedContextMessageLocked(msg); err != nil {
		return protocol.Response{}, err
	}
	if err := d.append(event{
		Event: evSent, ID: msg.ID, From: msg.From, To: msg.To,
		Body: msg.Body, ReplyTo: msg.ReplyTo,
	}); err != nil {
		return protocol.Response{}, err
	}
	d.messages[id] = msg
	d.messageOrder = append(d.messageOrder, id)
	notify := d.shouldNotifyInboxLocked(msg)
	if w := d.matchWaiter(msg); w != nil {
		if w.acknowledge {
			d.pending = append(d.pending, msg)
			d.offer(w, msg)
		} else {
			if err := d.deliver(w, msg); err != nil {
				d.pending = append(d.pending, msg)
				d.failWaiter(w, err)
				if notify {
					d.queueInboxWakeLocked(msg.To, 0, time.Time{})
				}
				return protocol.Response{}, err
			}
		}
	} else {
		d.pending = append(d.pending, msg)
	}
	if notify {
		d.queueInboxWakeLocked(msg.To, 0, time.Time{})
	}
	d.debug.Printf("replied %s %s -> %s reply_to=%s", id, msg.From, msg.To, msg.ReplyTo)
	return protocol.Response{ID: id}, nil
}

const (
	contextForagerType  = "fledge-forager"
	contextAnalyzerType = "fledge-analyzer"
)

// validateManagedContextMessageLocked makes the context protocol a daemon
// boundary rather than a prompt convention. Managed requests and replies are
// rejected before msg.sent is appended, so malformed or uncorrelated payloads
// can never enter a recipient's mailbox. Caller holds d.mu.
func (d *Daemon) validateManagedContextMessageLocked(msg protocol.Message) error {
	sender := d.agents[msg.From]
	recipient := d.agents[msg.To]

	switch {
	case sender.Type == contextForagerType && recipient.Type == contextAnalyzerType:
		if msg.ReplyTo != "" {
			return errors.New("managed context analyzer request must not set reply_to")
		}
		if err := contextdoc.ValidateComposedAnalyzerRequest([]byte(msg.Body)); err != nil {
			return fmt.Errorf("managed context analyzer request rejected before send: %w", err)
		}
	case sender.Type == contextAnalyzerType && recipient.Type == contextForagerType:
		if msg.ReplyTo == "" {
			return errors.New("managed context analyzer reply requires reply_to")
		}
		original, ok := d.messages[msg.ReplyTo]
		if !ok {
			return fmt.Errorf("managed context analyzer reply references unknown message %q", msg.ReplyTo)
		}
		if original.From != msg.To || original.To != msg.From {
			return fmt.Errorf(
				"managed context analyzer reply %q does not correlate %q -> %q",
				msg.ReplyTo, msg.To, msg.From,
			)
		}
		if !d.messageDelivered[msg.ReplyTo] {
			return fmt.Errorf(
				"managed context analyzer request %q has not been claimed by %q",
				msg.ReplyTo, msg.From,
			)
		}
		if err := contextdoc.ValidateAnalyzerReply(
			[]byte(msg.Body), []byte(original.Body),
		); err != nil {
			return fmt.Errorf("managed context analyzer reply rejected before send: %w", err)
		}
	}
	return nil
}

// matchWaiter finds the first live waiter msg satisfies. The waiter remains
// parked until the authoritative delivery append succeeds. Caller holds d.mu.
func (d *Daemon) matchWaiter(msg protocol.Message) *waiter {
	for _, w := range d.waiters {
		if w.done || !w.matches(msg) {
			continue
		}
		return w
	}
	return nil
}

// matches reports whether msg satisfies this wait. Sender and reply
// constraints are conjunctive; anything else stays pending.
func (w *waiter) matches(msg protocol.Message) bool {
	if msg.To != w.as {
		return false
	}
	if w.from != "" && msg.From != w.from {
		return false
	}
	if w.replyTo != "" && msg.ReplyTo != w.replyTo {
		return false
	}
	return true
}

// offer hands a message to an acknowledge-after-output waiter without removing
// it from durable pending state. If the receiver disappears before ack, the
// same correlation remains available to a retry.
func (d *Daemon) offer(w *waiter, msg protocol.Message) {
	w.done = true
	d.dropWaiter(w)
	w.ch <- waiterResult{msg: msg}
	d.debug.Printf("offered %s to %s", msg.ID, msg.To)
}

// deliver is the legacy eager-delivery path used by old wire clients. Caller
// holds d.mu.
func (d *Daemon) deliver(w *waiter, msg protocol.Message) error {
	if err := d.append(event{Event: evDelivered, ID: msg.ID, To: msg.To}); err != nil {
		return err
	}
	d.messageDelivered[msg.ID] = true
	w.done = true
	d.dropWaiter(w)
	w.ch <- waiterResult{msg: msg}
	d.debug.Printf("delivered %s to %s", msg.ID, msg.To)
	return nil
}

// failWaiter releases a parked wait after a delivery append failure. The
// message is already back in pending before this is called, so the client can
// retry immediately without a daemon restart. Caller holds d.mu.
func (d *Daemon) failWaiter(w *waiter, err error) {
	w.done = true
	d.dropWaiter(w)
	w.ch <- waiterResult{err: err}
}

// inbox returns the oldest matching message immediately. Undelivered mailbox
// messages retain send order. A nil message is a successful empty inbox check.
func (d *Daemon) inbox(req *protocol.Request) (protocol.Response, error) {
	if req.As == "" {
		return protocol.Response{}, errors.New("missing --as")
	}

	filter := &waiter{as: req.As, from: req.From, replyTo: req.ReplyTo}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return protocol.Response{}, errors.New("daemon is closing")
	}
	if err := d.authorizeMessageActorLocked(req.As, req.Token, req.Credential); err != nil {
		return protocol.Response{}, err
	}
	if req.From != "" {
		if _, ok := d.agents[req.From]; !ok {
			return protocol.Response{}, fmt.Errorf("no registered agent %q", req.From)
		}
	}
	for i, msg := range d.pending {
		if !filter.matches(msg) {
			continue
		}
		if err := d.append(event{Event: evDelivered, ID: msg.ID, To: msg.To}); err != nil {
			return protocol.Response{}, err
		}
		d.pending = append(d.pending[:i], d.pending[i+1:]...)
		d.messageDelivered[msg.ID] = true
		d.debug.Printf("inbox delivered %s to %s", msg.ID, msg.To)
		return protocol.Response{Message: &msg}, nil
	}
	return protocol.Response{}, nil
}

// peek returns the oldest matching message without finalizing delivery. The
// CLI acknowledges only after it has written the JSON response successfully.
func (d *Daemon) peek(req *protocol.Request) (protocol.Response, error) {
	if req.As == "" {
		return protocol.Response{}, errors.New("missing --as")
	}

	filter := &waiter{as: req.As, from: req.From, replyTo: req.ReplyTo}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return protocol.Response{}, errors.New("daemon is closing")
	}
	if err := d.authorizeMessageActorLocked(req.As, req.Token, req.Credential); err != nil {
		return protocol.Response{}, err
	}
	if req.From != "" {
		if _, ok := d.agents[req.From]; !ok {
			return protocol.Response{}, fmt.Errorf("no registered agent %q", req.From)
		}
	}
	for _, msg := range d.pending {
		if filter.matches(msg) {
			d.debug.Printf("peeked %s for %s", msg.ID, msg.To)
			return protocol.Response{Message: &msg}, nil
		}
	}
	return protocol.Response{}, nil
}

func (d *Daemon) wait(req *protocol.Request, gone <-chan struct{}) (protocol.Response, error) {
	return d.waitMode(req, gone, false)
}

// receive blocks like wait but leaves the matched message pending until ack.
func (d *Daemon) receive(req *protocol.Request, gone <-chan struct{}) (protocol.Response, error) {
	return d.waitMode(req, gone, true)
}

func (d *Daemon) waitMode(req *protocol.Request, gone <-chan struct{}, acknowledge bool) (protocol.Response, error) {
	if req.As == "" {
		return protocol.Response{}, errors.New("missing --as")
	}

	w := &waiter{
		as: req.As, from: req.From, replyTo: req.ReplyTo, acknowledge: acknowledge,
		ch: make(chan waiterResult, 1), cancel: make(chan struct{}),
	}

	d.mu.Lock()
	if d.closing {
		d.mu.Unlock()
		return protocol.Response{}, errors.New("daemon is closing")
	}
	if err := d.authorizeMessageActorLocked(req.As, req.Token, req.Credential); err != nil {
		d.mu.Unlock()
		return protocol.Response{}, err
	}
	if req.From != "" {
		if _, ok := d.agents[req.From]; !ok {
			d.mu.Unlock()
			return protocol.Response{}, fmt.Errorf("no registered agent %q", req.From)
		}
	}
	// Pending messages are consumed in send order.
	for i, msg := range d.pending {
		if !w.matches(msg) {
			continue
		}
		if acknowledge {
			d.mu.Unlock()
			return protocol.Response{Message: &msg}, nil
		}
		if err := d.deliver(w, msg); err != nil {
			d.mu.Unlock()
			return protocol.Response{}, err
		}
		d.pending = append(d.pending[:i], d.pending[i+1:]...)
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
	case result := <-w.ch:
		if result.err != nil {
			return protocol.Response{}, result.err
		}
		return protocol.Response{Message: &result.msg}, nil
	case <-w.cancel:
		return protocol.Response{}, fmt.Errorf("agent %q stopped while waiting", req.As)
	case <-timeout:
		d.mu.Lock()
		defer d.mu.Unlock()
		// A sender may have claimed this waiter between the timer firing and
		// the lock; that delivery is already journaled, so honor it.
		select {
		case result := <-w.ch:
			if result.err != nil {
				return protocol.Response{}, result.err
			}
			return protocol.Response{Message: &result.msg}, nil
		case <-w.cancel:
			return protocol.Response{}, fmt.Errorf("agent %q stopped while waiting", req.As)
		default:
		}
		d.dropWaiter(w)
		return protocol.Response{}, errors.New("timed out waiting for a message")
	case <-gone:
		d.mu.Lock()
		defer d.mu.Unlock()
		select {
		case result := <-w.ch:
			if result.err != nil {
				return protocol.Response{}, result.err
			}
			if acknowledge {
				d.debug.Printf("offered %s to %s, but its receive disconnected; message remains pending", result.msg.ID, result.msg.To)
			} else {
				// Legacy wait clients finalize before the response write and
				// retain their historical crash window.
				d.debug.Printf("delivered %s to %s, but its wait disconnected", result.msg.ID, result.msg.To)
			}
			return protocol.Response{Message: &result.msg}, nil
		case <-w.cancel:
			return protocol.Response{}, fmt.Errorf("agent %q stopped while waiting", req.As)
		default:
		}
		d.dropWaiter(w)
		return protocol.Response{}, errors.New("wait abandoned")
	}
}

// ack finalizes a message only after the receiving CLI has emitted it. Ack is
// idempotent so a caller may safely repeat it if the acknowledgement response
// itself is lost.
func (d *Daemon) ack(req *protocol.Request) (protocol.Response, error) {
	if req.As == "" {
		return protocol.Response{}, errors.New("missing --as")
	}
	if req.ID == "" {
		return protocol.Response{}, errors.New("missing message id")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return protocol.Response{}, errors.New("daemon is closing")
	}
	if err := d.authorizeMessageActorLocked(req.As, req.Token, req.Credential); err != nil {
		return protocol.Response{}, err
	}
	msg, ok := d.messages[req.ID]
	if !ok || msg.To != req.As {
		return protocol.Response{}, fmt.Errorf("message %q is not inbound to %q", req.ID, req.As)
	}
	if d.messageDelivered[req.ID] {
		return protocol.Response{ID: req.ID}, nil
	}
	index := -1
	for i, pending := range d.pending {
		if pending.ID == req.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return protocol.Response{}, fmt.Errorf("message %q is not pending for %q", req.ID, req.As)
	}
	if err := d.append(event{Event: evDelivered, ID: msg.ID, To: msg.To}); err != nil {
		return protocol.Response{}, err
	}
	d.pending = append(d.pending[:index], d.pending[index+1:]...)
	d.messageDelivered[msg.ID] = true
	d.debug.Printf("acknowledged %s by %s", msg.ID, req.As)
	return protocol.Response{ID: msg.ID}, nil
}

// authenticateIdentityLocked checks the launch credential for a spawned
// identity. Self-registered agents predate launch credentials and continue to
// rely on the same-user Unix-socket boundary. Caller holds d.mu.
func (d *Daemon) authenticateIdentityLocked(name, token, credential string) error {
	want := d.identityTokens[name]
	if want == "" {
		return nil
	}
	if credential != "" && subtle.ConstantTimeCompare([]byte(credential), []byte(want)) == 1 {
		return nil
	}
	if token == "" {
		return fmt.Errorf("agent %q requires its injected identity token", name)
	}
	sum := sha256.Sum256([]byte(token))
	got := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return fmt.Errorf("invalid identity token for %q", name)
	}
	return nil
}

// authorizeMessageActorLocked authenticates the claimed identity before
// checking lifecycle authority. A stopped spawned agent deliberately retains
// its credential: stale panes remain identifiable but cannot message.
func (d *Daemon) authorizeMessageActorLocked(name, token, credential string) error {
	a, ok := d.agents[name]
	if !ok {
		return fmt.Errorf("no registered agent %q", name)
	}
	if err := d.authenticateIdentityLocked(name, token, credential); err != nil {
		return err
	}
	if d.agentWorkspaceClosingLocked(name) {
		return fmt.Errorf("agent %q belongs to a closing workspace and is not authorized for messaging", name)
	}
	if d.stopping[name] {
		return fmt.Errorf("agent %q is stopping and is not authorized for messaging", name)
	}
	if a.Integration != "" {
		if a.State != stateRunning {
			return fmt.Errorf("agent %q is %s and is not authorized for messaging", name, a.State)
		}
		return nil
	}
	if !alive(a.PID) {
		return fmt.Errorf("agent %q is not running and is not authorized for messaging", name)
	}
	return nil
}

// validateMessageRecipientLocked allows durable work to queue while a spawned
// recipient is starting, but never after it stops or becomes orphaned.
func (d *Daemon) validateMessageRecipientLocked(name string) error {
	a, ok := d.agents[name]
	if !ok {
		return fmt.Errorf("no registered agent %q", name)
	}
	if d.agentWorkspaceClosingLocked(name) {
		return fmt.Errorf("agent %q belongs to a closing workspace and cannot receive messages", name)
	}
	if d.stopping[name] {
		return fmt.Errorf("agent %q is stopping and cannot receive messages", name)
	}
	if a.Integration != "" {
		if a.State == stateStopped || a.State == stateOrphaned {
			return fmt.Errorf("agent %q is %s and cannot receive messages", name, a.State)
		}
		return nil
	}
	if !alive(a.PID) {
		return fmt.Errorf("agent %q is not running and cannot receive messages", name)
	}
	return nil
}

// agentWorkspaceClosingLocked reports durable teardown intent, not merely an
// in-progress RPC. Once workspace.closing is journaled, every identity in that
// workspace has lost messaging authority until the closure is completed or
// recovered, even if the external close succeeded before its completion facts
// could be appended. Caller holds d.mu.
func (d *Daemon) agentWorkspaceClosingLocked(name string) bool {
	a, ok := d.agents[name]
	if !ok || a.WorkspaceID == "" {
		return false
	}
	_, closing := d.workspaceClosures[a.WorkspaceID]
	return closing
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
