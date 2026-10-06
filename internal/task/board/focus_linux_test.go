//go:build linux

package board

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/tasktest"
	"golang.org/x/sys/unix"
)

func TestFocusOwnerRejectsReassignmentAfterSelectionRead(t *testing.T) {
	cwd := identitytest.Repository(t)
	a := tasktest.Agent("w1:p1", "term", "worker")
	rec := tasktest.Register(t, cwd, a)
	id := tasktest.Seed(t, cwd, task.Record{Status: task.Assigned, Owner: &rec.ID})
	path := filepath.Join(cwd, ".fledge/state/tasks", id+".json")
	old, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := tasktest.Load(t, cwd, id)
	changed.Owner = tasktest.Ptr("ffffffff")
	data, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(cwd, "reassigned.json")
	if err := os.WriteFile(replacement, data, 0600); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	focused := false
	c := libagent.Client{Cwd: cwd, API: apiFunc(func(_ context.Context, method string, _, result any) error {
		switch method {
		case "agent.list":
			// selector reads ownership after agent.list. A FIFO holds that read
			// open with the old record until its path contains the new owner.
			// This synchronizes a concurrent assignment without a production seam.
			if err := os.Remove(path); err != nil {
				return err
			}
			if err := unix.Mkfifo(path, 0600); err != nil {
				return err
			}
			pipe, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			t.Cleanup(func() { pipe.Close() })
			if _, err := pipe.Write(old); err != nil {
				return err
			}
			watch, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
			if err != nil {
				return err
			}
			if _, err := unix.InotifyAddWatch(watch, path, unix.IN_OPEN); err != nil {
				unix.Close(watch)
				return err
			}
			go func() {
				defer unix.Close(watch)
				// Poll is bounded even if selection never opens the task. Always
				// replace the path and close the writer to release any pending read.
				events := []unix.PollFd{{Fd: int32(watch), Events: unix.POLLIN}}
				n, waitErr := unix.Poll(events, 2000)
				if waitErr == nil && (n != 1 || events[0].Revents&unix.POLLIN == 0) {
					waitErr = fmt.Errorf("selector did not open the controlled ownership read")
				}
				if err := os.Rename(replacement, path); err != nil {
					// Avoid leaving a FIFO for a subsequent read if setup fails.
					os.Remove(path)
					waitErr = err
				}
				pipe.Close()
				finished <- waitErr
			}()
			*(result.(*herdr.AgentListResult)) = herdr.AgentListResult{Type: "agent_list", Agents: []herdr.AgentDetails{a.Agent}}
			return nil
		case "agent.focus":
			focused = true
			*(result.(*herdr.AgentResult)) = a
			return nil
		default:
			return fmt.Errorf("unexpected request %s", method)
		}
	})}
	err = FocusOwner(context.Background(), c, id, rec.ID)
	select {
	case fixtureErr := <-finished:
		if fixtureErr != nil {
			t.Fatal(fixtureErr)
		}
	case <-time.After(time.Second):
		t.Fatalf("controlled ownership read did not finish: %v", err)
	}
	if focused || err == nil || !strings.Contains(err.Error(), "owner changed during navigation") {
		t.Fatalf("reassigned task reached focus or lost its diagnostic: focused=%v err=%v", focused, err)
	}
	if got := tasktest.Load(t, cwd, id); got.Owner == nil || *got.Owner != "ffffffff" {
		t.Fatal("navigation overwrote the concurrent reassignment")
	}
}
