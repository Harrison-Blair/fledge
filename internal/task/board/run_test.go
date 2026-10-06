package board

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
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
func TestRunResolvesRepositoryOnceAndSeesLaterState(t *testing.T) {
	cwd := identitytest.Repository(t)
	a := tasktest.Agent("w1:p1", "term", "live")
	c := tasktest.Client(t, cwd, "", herdrscript.Call{Method: "agent.list", Result: herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}})
	runner := func(m tea.Model, _ io.Reader, _ io.Writer, _ context.Context) error {
		mm := m.(*model)
		// State appears after startup; later loads must find it without Git.
		id := tasktest.Seed(t, cwd, task.Record{Title: "later", Status: task.Created})
		rec := tasktest.Register(t, cwd, a)
		t.Setenv("PATH", t.TempDir())
		tasks := mm.refresh(Tasks, false)().(Observation)
		if tasks.Err != nil || tasks.Snapshot.Records[id].Title != "later" {
			t.Fatalf("tasks: %+v", tasks)
		}
		workers := mm.refresh(Workers, false)().(Observation)
		if workers.Err != nil || workers.Workers[rec.ID].Name != "live" {
			t.Fatalf("workers: %+v", workers)
		}
		return nil
	}
	if out := run(context.Background(), c, strings.NewReader("q"), &bytes.Buffer{}, func(any) bool { return true }, runner); out.ExitCode() != 0 {
		t.Fatalf("%+v", out)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".fledge", "state", "tasks")); err != nil {
		t.Fatal(err)
	}
}
