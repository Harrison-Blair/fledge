//go:build linux

package board

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// A real PTY exercises the raw-mode/alternate-screen lifecycle, including a
// runtime failure. These owned descriptors never touch a user's terminal.
func TestProgramRestoresTerminal(t *testing.T) {
	for _, mode := range []string{"q", "ctrl-c", "cancel", "runtime"} {
		t.Run(mode, func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
				t.Fatal(err)
			}
			number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if err != nil {
				t.Fatal(err)
			}
			slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer slave.Close()
			if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 120}); err != nil {
				t.Fatal(err)
			}
			before, err := term.GetState(int(slave.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			var output lockedBuffer
			go func() { _, _ = io.Copy(&output, master) }()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := libagent.Client{Cwd: identitytest.Repository(t), API: apiFunc(func(context.Context, string, any, any) error { return fmt.Errorf("unavailable") })}
			done := make(chan libagent.Outcome, 1)
			var input io.Reader = slave
			if mode == "runtime" {
				input = failingTerminal{slave}
			}
			go func() { done <- Run(ctx, c, input, slave) }()
			deadline := time.Now().Add(3 * time.Second)
			for !strings.Contains(output.String(), "Task board") && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if !strings.Contains(output.String(), "Task board") {
				t.Fatal("UI never rendered")
			}
			switch mode {
			case "q":
				_, _ = master.Write([]byte("q"))
			case "ctrl-c":
				_, _ = master.Write([]byte{3})
			case "cancel":
				cancel()
			case "runtime":
				_, _ = master.Write([]byte("!"))
			}
			var outcome libagent.Outcome
			select {
			case outcome = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("board did not exit")
			}
			want := 0
			if mode == "runtime" {
				want = 1
			}
			if outcome.ExitCode() != want {
				t.Fatalf("outcome %+v", outcome)
			}
			after, err := term.GetState(int(slave.Fd()))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("terminal attributes not restored")
			}
			// The copy goroutine can still hold the restore sequence when Run returns.
			deadline = time.Now().Add(3 * time.Second)
			for !strings.Contains(output.String(), "\x1b[?1049l") && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if !strings.Contains(output.String(), "\x1b[?1049l") {
				t.Fatal("alternate screen not restored")
			}
		})
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

// Fail an actual terminal read after startup, without calling Program.Kill.
type failingTerminal struct{ *os.File }

func (f failingTerminal) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	if bytes.Contains(p[:n], []byte("!")) {
		return 0, io.ErrUnexpectedEOF
	}
	return n, err
}
