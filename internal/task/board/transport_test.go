package board

import (
	"bufio"
	"context"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/sockettest"
)

func peer(t *testing.T, reply string) string {
	t.Helper()
	l, socket := sockettest.Listen(t)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = bufio.NewReader(conn).ReadBytes('\n')
		if reply == "hold" {
			<-done
		} else if reply != "close" {
			_, _ = conn.Write([]byte(reply + "\n"))
		}
	}()
	return socket
}
func TestLoadActualTransportDeadlineAndBrokenReplies(t *testing.T) {
	for _, reply := range []string{"hold", "close", "{", `{"id":"fledge","result":{"type":"agent_list","agents":null}}`, `{"id":"other","result":{"type":"agent_list","agents":[]}}`} {
		t.Run(reply, func(t *testing.T) {
			c := libagent.Client{Cwd: identitytest.Repository(t), API: herdr.Client{Socket: peer(t, reply), Timeout: 20 * time.Second}}
			r := repository(t, c.Cwd)
			start := time.Now()
			// A short parent deadline proves the socket request honors the
			// caller's context; TestRefreshHasTotalDeadline checks the budget.
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			out := Load(ctx, c, r, Workers)
			if out.Err == nil {
				t.Fatal("broken peer accepted")
			}
			elapsed := time.Since(start)
			if reply == "hold" && (elapsed < 100*time.Millisecond || elapsed > 2*time.Second) {
				t.Fatalf("parent deadline elapsed %v", elapsed)
			}
		})
	}
}
func TestQuitCancelsInFlightObservationWithoutBlockingInput(t *testing.T) {
	m := boardModel(t)
	started, finished := make(chan struct{}), make(chan struct{})
	m.client.API = apiFunc(func(ctx context.Context, _ string, _, _ any) error {
		close(started)
		<-ctx.Done()
		close(finished)
		return ctx.Err()
	})
	cmd := m.refresh(Workers, false)
	go cmd()
	<-started
	if m.refresh(Workers, false) != nil {
		t.Fatal("overlapping refresh")
	}
	start := time.Now()
	key(m, "q")
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("quit blocked on IO")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("request not cancelled")
	}
}
