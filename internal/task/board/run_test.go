package board

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestRunChecksTTYBeforeLoading(t *testing.T) {
	out := Run(context.Background(), libagent.Client{Cwd: "/does/not/exist"}, strings.NewReader("q"), &bytes.Buffer{})
	if out.ExitCode() != 2 || !strings.Contains(out.Error.Message, "terminal") {
		t.Fatalf("%+v", out)
	}
}
func TestRunStartupAndRuntimeOutcomes(t *testing.T) {
	for _, mode := range []string{"startup", "quit", "cancel", "failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cwd := identitytest.Repository(t)
			if mode == "startup" {
				cwd = "/does/not/exist"
			}
			ran := false
			runner := func(m tea.Model, _ io.Reader, _ io.Writer, _ context.Context) error {
				ran = true
				if mode == "cancel" {
					cancel()
					return tea.ErrProgramKilled
				}
				if mode == "failure" {
					return errors.New("runtime failed")
				}
				cmd := m.(*model).key("q")
				if cmd == nil {
					t.Fatal("quit command missing")
				}
				return nil
			}
			out := run(ctx, libagent.Client{Cwd: cwd}, strings.NewReader("q"), &bytes.Buffer{}, func(any) bool { return true }, runner)
			want := 0
			if mode == "startup" || mode == "failure" {
				want = 1
			}
			if out.ExitCode() != want || ran == (mode == "startup") {
				t.Fatalf("ran %v outcome %+v", ran, out)
			}
		})
	}
}
