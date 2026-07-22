// Package pirpc supervises a single `pi --mode rpc` subprocess, exchanging
// JSONL frames over its stdin and stdout. Framing is LF-only: frames are never
// split on U+2028 or U+2029.
package pirpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// maxFrameBytes caps one stdout frame. Agent frames carry whole tool results,
// so the default scanner limit is far too small.
const maxFrameBytes = 1 << 20

// stopGrace is how long Stop waits for the agent to exit on its own after
// stdin closes, before it resorts to SIGKILL.
var stopGrace = 3 * time.Second

// Event is one frame from the agent's stdout. Raw carries the whole frame for
// logging; only Type and ID are interpreted.
type Event struct {
	Type string
	ID   string
	Raw  json.RawMessage
}

// Runner supervises one pi rpc subprocess.
type Runner struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	done   chan struct{}

	// readerDone closes when the reader goroutine leaves the scan loop. The
	// waiter goroutine blocks on it so cmd.Wait cannot close the read end of
	// stdout while frames are still undrained.
	readerDone chan struct{}

	// readErr and waitErr are written before done closes, so any goroutine
	// that has observed done may read them.
	readErr error
	waitErr error

	mu     sync.Mutex
	closed bool

	stopOnce sync.Once
	stopErr  error
}

// Start launches argv in cwd, with env appended to the current environment as
// "K=V" entries. onEvent is called from the reader goroutine for every frame
// that parses; unparseable lines are skipped. stderr is copied to stderrTo,
// which may be nil to discard it.
func Start(argv []string, cwd string, env []string, stderrTo io.Writer, onEvent func(Event)) (*Runner, error) {
	if len(argv) == 0 {
		return nil, errors.New("empty argv")
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), env...)
	cmd.Stderr = stderrTo

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	r := &Runner{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     stdout,
		done:       make(chan struct{}),
		readerDone: make(chan struct{}),
	}
	go r.read(stdout, onEvent)
	go r.wait()
	return r, nil
}

// read consumes frames until the scan loop ends, recording why. Reaping is the
// waiter's job: a descendant that inherited stdout can hold the pipe open long
// after the agent itself is gone, so reading must not gate process exit.
func (r *Runner) read(stdout io.Reader, onEvent func(Event)) {
	defer close(r.readerDone)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFrameBytes)
	for scanner.Scan() {
		line := scanner.Bytes()
		var head struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			continue
		}
		onEvent(Event{Type: head.Type, ID: head.ID, Raw: append(json.RawMessage(nil), line...)})
	}
	if r.readErr = scanner.Err(); r.readErr != nil {
		// A scanner error (e.g. an oversized frame) leaves the stream
		// unusable and the agent likely wedged writing to a pipe we will
		// never drain again. Unlike a clean EOF or a descendant holding the
		// pipe open, it will not exit on its own, so force it down here:
		// without this the waiter blocks in cmd.Wait forever, done never
		// closes, and the daemon keeps the agent registered as alive. This
		// runs before readerDone closes, so the kill still precedes the
		// waiter's cmd.Wait, and Stop observes the read error rather than
		// masking it behind a kill it never had to order.
		r.mu.Lock()
		if !r.closed {
			r.closed = true
			r.stdin.Close()
		}
		r.mu.Unlock()
		r.cmd.Process.Kill()
		r.stdout.Close()
	}
}

// wait reaps the process once the reader has drained stdout, and closing done
// is its last act.
func (r *Runner) wait() {
	defer close(r.done)

	<-r.readerDone
	r.waitErr = r.cmd.Wait()
}

// PID reports the agent's process id.
func (r *Runner) PID() int {
	return r.cmd.Process.Pid
}

// Prompt writes a prompt frame addressed to id.
func (r *Runner) Prompt(id, message string) error {
	frame, err := json.Marshal(struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Message string `json:"message"`
	}{ID: id, Type: "prompt", Message: message})
	if err != nil {
		return err
	}
	return r.write(append(frame, '\n'))
}

func (r *Runner) write(frame []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("agent %d: stdin closed", r.cmd.Process.Pid)
	}
	_, err := r.stdin.Write(frame)
	return err
}

// Stop shuts the agent down: an abort frame, then stdin EOF, then SIGKILL if it
// has not exited within the grace period. It is safe to call more than once and
// returns only once the process has been reaped. The error reports an abnormal
// exit or a failed read, but not the fallout of a kill Stop itself ordered.
func (r *Runner) Stop() error {
	r.stopOnce.Do(func() {
		r.write([]byte(`{"type":"abort"}` + "\n"))

		r.mu.Lock()
		r.closed = true
		r.stdin.Close()
		r.mu.Unlock()

		timer := time.NewTimer(stopGrace)
		defer timer.Stop()
		killed := false
		var killErr error
		select {
		case <-r.done:
		case <-timer.C:
			killErr = r.cmd.Process.Kill()
			// SIGKILL alone need not end the reader: a descendant may still
			// hold the write end. Closing our read end always does.
			r.stdout.Close()
			killed = killErr == nil
		}

		<-r.done
		switch {
		case killErr != nil:
			r.stopErr = killErr
		case killed:
			// The kill caused both the wait error and the read error.
		default:
			r.stopErr = errors.Join(r.waitErr, r.readErr)
		}
	})

	<-r.done
	return r.stopErr
}

// Done is closed once the process has exited and been reaped.
func (r *Runner) Done() <-chan struct{} {
	return r.done
}
