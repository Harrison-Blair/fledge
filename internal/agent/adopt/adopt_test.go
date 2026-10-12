package adopt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

type call = herdrscript.Call

// The fake process tree: this command (4242) runs under a tool shell (4000)
// started by the harness (3000), the pane's foreground process, whose login
// shell (2000) runs under init.
const self, harness, shell = 4242, 3000, 2000

// statLine is a /proc/<pid>/stat line for pid named comm, with parent ppid
// and start time start.
func statLine(pid int, comm string, ppid int, start uint64) string {
	return fmt.Sprintf("%d (%s) S %d %d %d 0 -1 4194560 100 0 0 0 1 1 0 0 20 0 1 0 %d 1000 100\n", pid, comm, ppid, pid, pid, start)
}

// tree is the default ancestry of self.
func tree() map[int]string {
	return map[int]string{
		self:    statLine(self, "fledge", 4000, 500),
		4000:    statLine(4000, "bash", harness, 400),
		harness: statLine(harness, "claude", shell, 300),
		shell:   statLine(shell, "bash", 1, 200),
		1:       statLine(1, "systemd", 0, 1),
	}
}

// fakeProcesses serves stats in place of /proc for the test, as Linux, with
// self as this command's PID. It returns the PIDs read, in order.
func fakeProcesses(t *testing.T, stats map[int]string) *[]int {
	t.Helper()
	var mu sync.Mutex
	read := &[]int{}
	restoreOpen, restoreSelf, restorePlatform := openStat, selfPID, platform
	t.Cleanup(func() { openStat, selfPID, platform = restoreOpen, restoreSelf, restorePlatform })
	platform = "linux"
	selfPID = func() int { return self }
	openStat = func(pid int) (io.ReadCloser, error) {
		mu.Lock()
		defer mu.Unlock()
		*read = append(*read, pid)
		s, ok := stats[pid]
		if !ok {
			return nil, &os.PathError{Op: "open", Path: fmt.Sprintf("/proc/%d/stat", pid), Err: os.ErrNotExist}
		}
		return io.NopCloser(strings.NewReader(s)), nil
	}
	return read
}

func client(t *testing.T, calls ...call) libagent.Client {
	t.Helper()
	fakeProcesses(t, tree())
	c := herdrscript.Client(t, calls...)
	c.CallerPane = "w1:p3"
	c.Cwd = identitytest.Repository(t)
	return c
}

// agent is the live agent in pane with the given name (nil for unnamed).
func agent(pane string, name *string) herdr.AgentResult {
	p := herdrscript.LiveAgent("idle")
	p.PaneID, p.Name = pane, name
	r := herdrscript.Info(p)
	r.Agent.TerminalID = "term_a"
	return r
}
func named(s string) *string { return &s }
func get(target string, r herdr.AgentResult) call {
	return call{Method: "agent.get", Params: map[string]any{"target": target}, Result: r}
}

// processInfo is a pane.process_info reply for pane with foreground pids.
func processInfo(pane string, pids ...any) map[string]any {
	procs := []any{}
	for _, pid := range pids {
		procs = append(procs, map[string]any{"pid": pid, "name": "claude", "cwd": "/repo"})
	}
	return map[string]any{"type": "pane_process_info", "process_info": map[string]any{
		"pane_id": pane, "shell_pid": shell, "foreground_process_group_id": harness, "foreground_processes": procs}}
}

// owned is the pane.process_info request for pane, answered with the harness
// in its foreground.
func owned(pane string) call {
	return call{Method: "pane.process_info", Params: map[string]any{"pane_id": pane}, Result: processInfo(pane, harness)}
}

// verified is the caller lookup and ownership check of the agent r in the
// caller's pane w1:p3.
func verified(r herdr.AgentResult) []call { return []call{get("w1:p3", r), owned(r.Agent.PaneID)} }

func calls(groups ...[]call) []call {
	var all []call
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

// labels are the requests that label pane, alone in tab, as helper.
func labels(pane, tab string) []call {
	one := 1
	return []call{
		{Method: "pane.rename", Params: map[string]any{"pane_id": pane, "label": "helper"}, Result: herdr.PaneResult{Type: "pane_info", Pane: herdrscript.Pane(pane, "w1", tab)}},
		{Method: "tab.get", Params: map[string]any{"tab_id": tab}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: tab, WorkspaceID: "w1", Label: "1", PaneCount: &one}}},
		{Method: "tab.rename", Params: map[string]any{"tab_id": tab, "label": "helper"}, Result: herdr.TabResult{Type: "tab_info", Tab: herdr.Tab{ID: tab, WorkspaceID: "w1", Label: "helper", PaneCount: &one}}},
	}
}
func notFound() error { return &herdr.Error{Code: "agent_not_found", Message: "no agent"} }

// renaming is the rename of the agent in pane to helper.
func renaming(pane string) call {
	return call{Method: "agent.rename", Params: map[string]any{"target": pane, "name": "helper"}, Result: agent(pane, named("helper"))}
}

func TestAdoptSelfRenamesUnnamedAgent(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{renaming("w1:p3")}, labels("w1:p3", "w1:t2"))...)
	out := Run(context.Background(), c, Options{Name: "helper"})
	if out.Status != "success" || out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	r := out.Result.(Result)
	if !r.Renamed || *r.Name != "helper" || r.Pane != "w1:p3" || r.TerminalID != "term_a" || r.RegisteredBy != "adopt" || r.Parent != nil || r.WorktreePath != nil || r.Profile != nil {
		t.Fatalf("%+v", r)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var stored identity.Record
	if err := s.Get(identity.Kind, r.ID, &stored); err != nil || !reflect.DeepEqual(stored, r.Record) {
		t.Fatalf("%+v %v", stored, err)
	}
	last := out.Effects[len(out.Effects)-4:]
	if !reflect.DeepEqual(last, []cli.Effect{{Action: "updated", Kind: "agent_name", ID: "w1:p3"}, {Action: "created", Kind: "agent_record", ID: r.ID}, {Action: "updated", Kind: "pane_label", ID: "w1:p3"}, {Action: "updated", Kind: "tab", ID: "w1:t2"}}) {
		t.Fatalf("%+v", out.Effects)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Adopted helper (w1:p3) as "+r.ID+".\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestAdoptNamedAgentWithMatchingNameDoesNotRename(t *testing.T) {
	for _, name := range []string{"worker", ""} {
		c := client(t, calls(verified(agent("w1:p3", named("worker"))), verified(agent("w1:p3", named("worker"))))...)
		out := Run(context.Background(), c, Options{Name: name})
		if out.Error != nil || out.Result.(Result).Renamed || *out.Result.(Result).Name != "worker" || out.Result.(Result).Parent != nil {
			t.Fatalf("%q: %+v %+v", name, out.Error, out.Result)
		}
	}
}

func TestAdoptRejections(t *testing.T) {
	for label, tc := range map[string]struct {
		o     Options
		calls []call
		code  string
	}{
		"different name":  {Options{Name: "other"}, []call{get("w1:p3", agent("w1:p3", named("worker")))}, "invalid_input"},
		"unnamed no name": {Options{}, []call{get("w1:p3", agent("w1:p3", nil))}, "invalid_input"},
		"bad name":        {Options{Name: "Bad Name"}, nil, "invalid_input"},
		"no agent":        {Options{Name: "x"}, []call{{Method: "agent.get", Err: notFound()}}, "agent_not_found"},
		"name taken": {Options{Name: "helper"}, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)),
			[]call{{Method: "agent.rename", Params: map[string]any{"target": "w1:p3", "name": "helper"}, Err: &herdr.Error{Code: "agent_name_taken", Message: "taken"}}}), "agent_name_taken"},
		"launch pending": {Options{Name: "x"}, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)),
			[]call{{Method: "agent.rename", Err: &herdr.Error{Code: "agent_launch_pending", Message: "pending"}}}), "agent_launch_pending"},
	} {
		t.Run(label, func(t *testing.T) {
			out := Run(context.Background(), client(t, tc.calls...), tc.o)
			if out.Error == nil || out.Error.Code != tc.code {
				t.Fatalf("%+v", out.Error)
			}
			if ids := records(t, out); len(ids) != 0 {
				t.Fatalf("records created: %v", ids)
			}
		})
	}
}

// Adopt needs the caller's pane; without it, it refuses before any request.
func TestAdoptWithoutCallerPaneIsInvalid(t *testing.T) {
	for _, pane := range []string{"", " ", "\t\n"} {
		c := client(t)
		c.CallerPane = pane
		read := fakeProcesses(t, tree())
		out := Run(context.Background(), c, Options{Name: "x"})
		if out.Status != "rejected" || out.Error == nil || out.Error.Code != "invalid_input" || out.Error.Phase != "validation" || out.ExitCode() != 2 || len(out.Effects) != 0 || len(*read) != 0 {
			t.Fatalf("%q: %+v %+v", pane, out, out.Error)
		}
		if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
			t.Fatalf("created .fledge: %v", err)
		}
	}
}

// with is tree with changes applied: a pid's stat replaced, or removed when
// its new stat is empty.
func with(changes map[int]string) map[int]string {
	stats := tree()
	for pid, s := range changes {
		if s == "" {
			delete(stats, pid)
		} else {
			stats[pid] = s
		}
	}
	return stats
}

// deep is an ancestry of self n processes long below init.
func deep(n int) map[int]string {
	stats := map[int]string{1: statLine(1, "init", 0, 1)}
	pid := self
	for i := range n {
		parent := 10000 + i
		if i == n-1 {
			parent = 1
		}
		stats[pid] = statLine(pid, "p", parent, uint64(100000-i))
		pid = parent
	}
	return stats
}

// Rejections decided before registration create no .fledge, no record, and
// no Herdr change: a script with no rename or label calls fails the test on
// any such request.
func TestAdoptRejectionBeforeRegistrationLeavesNoFiles(t *testing.T) {
	unnamed := agent("w1:p3", nil)
	process := func(r any) []call {
		return []call{get("w1:p3", unnamed), {Method: "pane.process_info", Params: map[string]any{"pane_id": "w1:p3"}, Result: r}}
	}
	foreground := func(pids ...any) []call { return process(processInfo("w1:p3", pids...)) }
	for label, tc := range map[string]struct {
		calls    []call
		stats    map[int]string
		platform string
		code     string
		phase    string
	}{
		"missing pane":           {calls: []call{{Method: "agent.get", Params: map[string]any{"target": "w1:p3"}, Err: notFound()}}, code: "agent_not_found", phase: "agent.get"},
		"transport":              {calls: []call{{Method: "agent.get", Err: &herdr.Error{Code: "connection_error", Message: "down"}}}, code: "connection_error", phase: "agent.get"},
		"different name":         {calls: []call{get("w1:p3", agent("w1:p3", named("worker")))}, code: "invalid_input", phase: "validation"},
		"process info transport": {calls: []call{get("w1:p3", unnamed), {Method: "pane.process_info", Err: &herdr.Error{Code: "timeout", Message: "slow", Uncertain: true}}}, code: "timeout", phase: "pane.process_info"},
		"process info pane gone": {calls: []call{get("w1:p3", unnamed), {Method: "pane.process_info", Err: &herdr.Error{Code: "pane_not_found", Message: "gone"}}}, code: "pane_not_found", phase: "pane.process_info"},
		// The reported shell_pid is this command's ancestor, which proves nothing.
		"foreign foreground": {calls: foreground(7777), stats: with(map[int]string{7777: statLine(7777, "claude", shell, 250)}), code: "caller_identity_unverified", phase: "identity"},
		"init only":          {calls: foreground(1), code: "caller_identity_unverified", phase: "identity"},
		"pid zero":           {calls: foreground(0, harness), code: "caller_identity_unverified", phase: "identity"},
		"negative pid":       {calls: foreground(-1, harness), code: "caller_identity_unverified", phase: "identity"},
		"pid too large":      {calls: foreground(1<<23, harness), code: "caller_identity_unverified", phase: "identity"},
		// The script surfaces the decoding error that the real client reports as
		// protocol_error.
		"pid not a number":     {calls: foreground("3000"), code: "operation_failed", phase: "pane.process_info"},
		"pid missing":          {calls: process(map[string]any{"type": "pane_process_info", "process_info": map[string]any{"pane_id": "w1:p3", "foreground_processes": []any{map[string]any{"name": "claude"}}}}), code: "caller_identity_unverified", phase: "identity"},
		"no foreground":        {calls: foreground(), code: "caller_identity_unverified", phase: "identity"},
		"foreground null":      {calls: process(map[string]any{"type": "pane_process_info", "process_info": map[string]any{"pane_id": "w1:p3", "foreground_processes": nil}}), code: "caller_identity_unverified", phase: "identity"},
		"no process info":      {calls: process(map[string]any{"type": "pane_process_info"}), code: "caller_identity_unverified", phase: "identity"},
		"wrong discriminator":  {calls: process(map[string]any{"type": "pane_info", "process_info": processInfo("w1:p3", harness)["process_info"]}), code: "caller_identity_unverified", phase: "identity"},
		"wrong pane":           {calls: process(processInfo("w1:p4", harness)), code: "caller_identity_unverified", phase: "identity"},
		"self stat unreadable": {calls: foreground(harness), stats: with(map[int]string{self: ""}), code: "caller_identity_unverified", phase: "identity"},
		"missing ancestry":     {calls: foreground(harness), stats: with(map[int]string{4000: ""}), code: "caller_identity_unverified", phase: "identity"},
		"disappearing harness": {calls: foreground(harness), stats: with(map[int]string{harness: ""}), code: "caller_identity_unverified", phase: "identity"},
		"reused parent":        {calls: foreground(harness), stats: with(map[int]string{harness: statLine(harness, "claude", shell, 600)}), code: "caller_identity_unverified", phase: "identity"},
		"stat of another pid":  {calls: foreground(harness), stats: with(map[int]string{4000: statLine(4001, "bash", harness, 400)}), code: "caller_identity_unverified", phase: "identity"},
		"malformed stat":       {calls: foreground(harness), stats: with(map[int]string{4000: "4000 (bash S 3000"}), code: "caller_identity_unverified", phase: "identity"},
		"short stat":           {calls: foreground(harness), stats: with(map[int]string{4000: "4000 (bash) S 3000 1 1"}), code: "caller_identity_unverified", phase: "identity"},
		"bad parent field":     {calls: foreground(harness), stats: with(map[int]string{4000: strings.Replace(statLine(4000, "bash", harness, 400), " 3000 ", " x ", 1)}), code: "caller_identity_unverified", phase: "identity"},
		"orphan without init":  {calls: foreground(harness), stats: with(map[int]string{4000: statLine(4000, "bash", 0, 400)}), code: "caller_identity_unverified", phase: "identity"},
		"cycle":                {calls: foreground(harness), stats: with(map[int]string{4000: statLine(4000, "bash", 4100, 400), 4100: statLine(4100, "x", 4000, 400)}), code: "caller_identity_unverified", phase: "identity"},
		"oversized stat":       {calls: foreground(harness), stats: with(map[int]string{4000: statLine(4000, "bash", harness, 400) + strings.Repeat(" 0", 2100)}), code: "caller_identity_unverified", phase: "identity"},
		"too deep":             {calls: foreground(1), stats: deep(130), code: "caller_identity_unverified", phase: "identity"},
		"unsupported platform": {calls: []call{get("w1:p3", unnamed)}, platform: "darwin", code: "caller_identity_unverified", phase: "identity"},
	} {
		t.Run(label, func(t *testing.T) {
			c := client(t, tc.calls...)
			if tc.stats != nil {
				fakeProcesses(t, tc.stats)
			}
			if tc.platform != "" {
				platform = tc.platform
			}
			out := Run(context.Background(), c, Options{Name: "x"})
			if out.Status != "rejected" || out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != tc.phase || len(out.Effects) != 0 {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if tc.code == "caller_identity_unverified" && out.ExitCode() != 1 {
				t.Fatalf("exit %d", out.ExitCode())
			}
			if _, err := os.Stat(filepath.Join(c.Cwd, ".fledge")); !os.IsNotExist(err) {
				t.Fatalf("created .fledge: %v", err)
			}
		})
	}
}

// The ancestry walk stops at init, but a match found before it, or at the
// last step allowed, still counts.
func TestOwnershipAncestryBounds(t *testing.T) {
	for label, tc := range map[string]struct {
		stats map[int]string
		fg    int
	}{
		"command itself":     {tree(), self},
		"direct parent":      {tree(), 4000},
		"just before init":   {tree(), shell},
		"128th ancestor":     {deep(128), 10126},
		"name with brackets": {with(map[int]string{4000: statLine(4000, "a) (b ) S 1 c", harness, 400)}), harness},
	} {
		t.Run(label, func(t *testing.T) {
			read := fakeProcesses(t, tc.stats)
			c := herdrscript.Client(t, call{Method: "pane.process_info", Result: processInfo("w1:p3", tc.fg)})
			if err := owns(context.Background(), c, agent("w1:p3", nil).Agent); err != nil {
				t.Fatal(err)
			}
			for _, pid := range *read {
				if pid == 1 {
					t.Fatalf("read init after a match: %v", *read)
				}
			}
		})
	}
}

// A stat of exactly the read bound is accepted; one byte more is not.
func TestStatReadBound(t *testing.T) {
	line := statLine(self, "fledge", 4000, 500)
	exact := line[:len(line)-1] + strings.Repeat(" ", statLimit-len(line)) + "\n"
	fakeProcesses(t, map[int]string{self: exact})
	if _, err := stat(self); err != nil {
		t.Fatal(err)
	}
	fakeProcesses(t, map[int]string{self: exact + "0"})
	if _, err := stat(self); err == nil {
		t.Fatal("read past the bound")
	}
	for _, pid := range []int{0, -1, maxPID + 1} {
		read := fakeProcesses(t, tree())
		if _, err := stat(pid); err == nil || len(*read) != 0 {
			t.Fatalf("pid %d: %v %v", pid, err, *read)
		}
	}
}

// The real reader finds this test's parent among its ancestors.
func TestOwnershipReadsProc(t *testing.T) {
	if platform != "linux" {
		t.Skip("needs Linux /proc")
	}
	c := herdrscript.Client(t, call{Method: "pane.process_info", Result: processInfo("w1:p3", os.Getppid())})
	if err := owns(context.Background(), c, agent("w1:p3", nil).Agent); err != nil {
		t.Fatal(err)
	}
	c = herdrscript.Client(t, call{Method: "pane.process_info", Result: processInfo("w1:p3", 1)})
	if err := owns(context.Background(), c, agent("w1:p3", nil).Agent); err == nil {
		t.Fatal("init alone verified the caller")
	}
}

// HERDR_PANE_ID may name a pane the caller's terminal has left; agent.get
// answers with the terminal's canonical pane, which ownership is checked
// against and the record stores. The recheck follows the terminal again.
func TestAdoptFollowsCanonicalPane(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	c := client(t, calls([]call{get("old:p1", agent("w1:p3", nil)), owned("w1:p3"),
		get("old:p1", agent("w1:p4", nil)), owned("w1:p4"), renaming("w1:p4")}, labels("w1:p4", "w1:t2"))...)
	c.CallerPane = "old:p1"
	out := Run(context.Background(), c, Options{Name: "helper"})
	if out.Error != nil || out.Result.(Result).Pane != "w1:p4" {
		t.Fatalf("%+v %+v", out.Error, out.Result)
	}
	// Ownership evidence for the pane the caller named, not the canonical
	// one, does not count.
	c = client(t, get("old:p1", agent("w1:p3", nil)), call{Method: "pane.process_info", Params: map[string]any{"pane_id": "w1:p3"}, Result: processInfo("old:p1", harness)})
	c.CallerPane = "old:p1"
	if out := Run(context.Background(), c, Options{Name: "helper"}); out.Error == nil || out.Error.Code != "caller_identity_unverified" || len(out.Effects) != 0 {
		t.Fatalf("%+v %+v", out, out.Error)
	}
}

func records(t *testing.T, out cli.Outcome) []string {
	for _, e := range out.Effects {
		if e.Kind == "agent_record" {
			return []string{e.ID}
		}
	}
	return nil
}

func TestAdoptRefusesRegisteredTerminal(t *testing.T) {
	w := agent("w1:p3", named("worker"))
	c := client(t, calls(verified(w), verified(w), verified(w), verified(w))...)
	first := Run(context.Background(), c, Options{})
	if first.Error != nil {
		t.Fatalf("%+v", first.Error)
	}
	id := first.Result.(Result).ID
	out := Run(context.Background(), c, Options{})
	if out.Error == nil || out.Error.Code != "agent_already_registered" || !strings.Contains(out.Error.Message, id) {
		t.Fatalf("%+v", out.Error)
	}
}

func TestAdoptOutsideRepositoryFailsBeforeRename(t *testing.T) {
	c := client(t, verified(agent("w1:p3", nil))...)
	c.Cwd = t.TempDir()
	out := Run(context.Background(), c, Options{Name: "x"})
	if out.Error == nil || out.Error.Phase != "state" || len(out.Effects) != 0 {
		t.Fatalf("%+v", out)
	}
}

// A change seen by the recheck, after the store opened, stops adoption before
// any Herdr change or record. Effects report only the setup the store made:
// a fresh repository reports .fledge as created (partial), an existing store
// reports nothing (rejected). Rechecking requests that fail keep their codes.
func TestAdoptRecheckFailures(t *testing.T) {
	for _, unnamed := range []bool{true, false} {
		var name *string
		if !unnamed {
			name = named("worker")
		}
		before := agent("w1:p3", name)
		otherTerminal := agent("w1:p3", name)
		otherTerminal.Agent.TerminalID = "term_b"
		otherHarness := agent("w1:p3", name)
		otherHarness.Agent.Agent = named("codex")
		noHarness := agent("w1:p3", name)
		noHarness.Agent.Agent = nil
		for label, tc := range map[string]struct {
			recheck []call
			code    string
			phase   string
		}{
			"terminal changed":    {[]call{get("w1:p3", otherTerminal)}, "caller_identity_unverified", "identity"},
			"harness changed":     {[]call{get("w1:p3", otherHarness)}, "caller_identity_unverified", "identity"},
			"harness gone":        {[]call{get("w1:p3", noHarness)}, "caller_identity_unverified", "identity"},
			"agent gone":          {[]call{{Method: "agent.get", Err: notFound()}}, "agent_not_found", "agent.get"},
			"foreground changed":  {[]call{get("w1:p3", before), {Method: "pane.process_info", Result: processInfo("w1:p3", 7777)}}, "caller_identity_unverified", "identity"},
			"process info failed": {[]call{get("w1:p3", before), {Method: "pane.process_info", Err: &herdr.Error{Code: "connection_error", Message: "down"}}}, "connection_error", "pane.process_info"},
		} {
			for _, fresh := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s unnamed=%v fresh=%v", label, unnamed, fresh), func(t *testing.T) {
					c := client(t, calls(verified(before), tc.recheck)...)
					if !fresh {
						if _, err := identity.OpenStore(context.Background(), c.Cwd, &cli.Outcome{}); err != nil {
							t.Fatal(err)
						}
					}
					o := Options{}
					if unnamed {
						o.Name = "helper"
					}
					out := Run(context.Background(), c, o)
					if out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != tc.phase {
						t.Fatalf("%+v %+v", out, out.Error)
					}
					for _, e := range out.Effects {
						if e.Kind != "directory" && e.Kind != "file" {
							t.Fatalf("effect beyond setup: %+v", out.Effects)
						}
					}
					if fresh && (out.Status != "partial" || len(out.Effects) == 0) || !fresh && (out.Status != "rejected" || len(out.Effects) != 0) {
						t.Fatalf("%s %+v", out.Status, out.Effects)
					}
					s, err := identity.Existing(context.Background(), c.Cwd)
					if err != nil {
						t.Fatal(err)
					}
					if ids, err := s.List(identity.Kind); err != nil || len(ids) != 0 {
						t.Fatalf("records %v %v", ids, err)
					}
				})
			}
		}
	}
}

type adopterKey struct{}

// barrierHerdr serves concurrent adopters of the named agent in w1:p3. Each
// adopter's second pane.process_info, the last Herdr call before
// registration, waits until every adopter has reached it (or fails after a
// bound, so an adopter that stops early cannot hang the test).
type barrierHerdr struct {
	arrived sync.WaitGroup
	mu      sync.Mutex
	checks  map[any]int
}

func (b *barrierHerdr) Call(ctx context.Context, method string, params any, result any) error {
	var reply any
	switch method {
	case "agent.get":
		reply = agent("w1:p3", named("worker"))
	case "pane.process_info":
		b.mu.Lock()
		adopter := ctx.Value(adopterKey{})
		b.checks[adopter]++
		second := b.checks[adopter] == 2
		b.mu.Unlock()
		if second {
			b.arrived.Done()
			all := make(chan struct{})
			go func() { b.arrived.Wait(); close(all) }()
			select {
			case <-all:
			case <-time.After(5 * time.Second):
				return fmt.Errorf("not every adopter reached registration")
			}
		}
		reply = processInfo("w1:p3", harness)
	default:
		return fmt.Errorf("unexpected %s", method)
	}
	data, _ := json.Marshal(reply)
	return json.Unmarshal(data, result)
}

func TestConcurrentAdoptsRegisterOnce(t *testing.T) {
	const n = 2
	fake := &barrierHerdr{checks: map[any]int{}}
	fake.arrived.Add(n)
	c := client(t)
	c.API = fake
	// Create .fledge up front: concurrent first-time creation is not under test.
	if _, err := identity.OpenStore(context.Background(), c.Cwd, &cli.Outcome{}); err != nil {
		t.Fatal(err)
	}
	outs := make([]cli.Outcome, n)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Go(func() { outs[i] = Run(context.WithValue(context.Background(), adopterKey{}, i), c, Options{}) })
	}
	wg.Wait()
	var winner string
	refused := 0
	for _, out := range outs {
		switch {
		case out.Error == nil:
			if winner != "" {
				t.Fatalf("two adopts succeeded: %s and %s", winner, out.Result.(Result).ID)
			}
			winner = out.Result.(Result).ID
		case out.Error.Code == "agent_already_registered":
			refused++
		default:
			t.Fatalf("%+v", out.Error)
		}
	}
	if winner == "" || refused != n-1 {
		t.Fatalf("winner %q, %d refused", winner, refused)
	}
	for _, out := range outs {
		if out.Error != nil && !strings.Contains(out.Error.Message, winner) {
			t.Fatalf("refusal does not name the winner %s: %s", winner, out.Error.Message)
		}
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := s.List(identity.Kind); err != nil || len(ids) != 1 || ids[0] != winner {
		t.Fatalf("records %v, %v", ids, err)
	}
	if rec, err := identity.Live(s, "term_a"); err != nil || rec == nil || rec.ID != winner {
		t.Fatalf("%+v %v", rec, err)
	}
}

// A different harness started in an adopted terminal can be adopted afresh,
// and the new record has no parent: the caller's own ended record is not one.
func TestAdoptAfterHarnessChange(t *testing.T) {
	codex := agent("w1:p3", named("worker"))
	codex.Agent.Agent = named("codex")
	claude := agent("w1:p3", named("worker"))
	c := client(t, calls(verified(codex), verified(codex), verified(claude), verified(claude))...)
	first := Run(context.Background(), c, Options{})
	if first.Error != nil {
		t.Fatalf("%+v", first.Error)
	}
	out := Run(context.Background(), c, Options{})
	if out.Error != nil || out.Result.(Result).ID == first.Result.(Result).ID || *out.Result.(Result).Harness != "claude" || out.Result.(Result).Parent != nil {
		t.Fatalf("%+v", out)
	}
}

// A fresh self adoption has no parent even when another live record exists,
// and registration makes no caller lookup of its own.
func TestAdoptFreshSelfHasNullParent(t *testing.T) {
	c := client(t, calls(verified(agent("w1:p3", named("worker"))), verified(agent("w1:p3", named("worker"))))...)
	s, err := identity.OpenStore(context.Background(), c.Cwd, &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	other := agent("w1:p7", named("other")).Agent
	other.TerminalID = "term_other"
	if _, err := identity.Register(context.Background(), s, libagent.Client{}, other, "spawn", nil, nil); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), c, Options{})
	if out.Error != nil || out.Result.(Result).Parent != nil {
		t.Fatalf("%+v %+v", out.Error, out.Result)
	}
	if got := stored(t, c, out.Result.(Result).ID, 2); got.Parent != nil {
		t.Fatalf("%+v", got)
	}
}

// countPrechecks counts adopt's scans of the agent records outside Register,
// which itself scans once.
func countPrechecks(t *testing.T) *int {
	t.Helper()
	n, real := 0, registered
	registered = func(s *state.Store, a herdr.AgentDetails) (*identity.Record, error) { n++; return real(s, a) }
	t.Cleanup(func() { registered = real })
	return &n
}

// Adopting a named agent scans the agent records once, in Register. Renaming
// an unnamed agent is a Herdr call that cannot run under the store lock, so
// adopt scans once more beforehand to find a record of the terminal to name
// instead of registering anew: two scans, a deliberate exception to one scan
// per adopt.
func TestAdoptScansOnceUnlessItMustRenameFirst(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	n := countPrechecks(t)
	c := client(t, calls(verified(agent("w1:p3", named("worker"))), verified(agent("w1:p3", named("worker"))))...)
	if out := Run(context.Background(), c, Options{}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if *n != 0 {
		t.Fatalf("adopting a named agent scanned %d extra times, want 0", *n)
	}
	*n = 0
	c = client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{renaming("w1:p3")}, labels("w1:p3", "w1:t2"))...)
	if out := Run(context.Background(), c, Options{Name: "helper"}); out.Error != nil {
		t.Fatalf("%+v", out.Error)
	}
	if *n != 1 {
		t.Fatalf("adopting with a rename scanned %d extra times, want 1", *n)
	}
}

// seed stores a live record of the adopt fixture's terminal as registered
// from pane w1:p9 in workspace w9 with the given name, a parent, a profile,
// and a worktree, and returns it.
func seed(t *testing.T, c libagent.Client, name *string) identity.Record {
	t.Helper()
	s, err := identity.OpenStore(context.Background(), c.Cwd, &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	d := agent("w1:p9", name).Agent
	d.WorkspaceID = "w9"
	tree := "/repo/.fledge/worktrees/w"
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, d, "spawn", &identity.Checkout{Path: tree}, named("implementer"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(identity.Kind, rec.ID, &rec, func() error { rec.Parent = named("0000beef"); return nil }); err != nil {
		t.Fatal(err)
	}
	return rec
}

// stored is the record id, the only one of n stored.
func stored(t *testing.T, c libagent.Client, id string, n ...int) identity.Record {
	t.Helper()
	want := 1
	if len(n) > 0 {
		want = n[0]
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	if ids, err := s.List(identity.Kind); err != nil || len(ids) != want {
		t.Fatalf("records %v %v", ids, err)
	}
	var rec identity.Record
	if err := s.Get(identity.Kind, id, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// A registered terminal whose live agent is unnamed is named through Herdr and
// keeps its record: the same id, parent, profile, registration, and worktree,
// with the new name and the pane where it now runs. Whether the record kept a
// former name does not matter; Herdr's live name decides.
func TestAdoptNamesRegisteredUnnamedAgentKeepingRecord(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	for label, former := range map[string]*string{"lost name": named("worker"), "never named": nil} {
		t.Run(label, func(t *testing.T) {
			c := client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{renaming("w1:p3")}, labels("w1:p3", "w1:t2"))...)
			existing := seed(t, c, former)
			out := Run(context.Background(), c, Options{Name: "helper"})
			if out.Status != "success" || out.Error != nil {
				t.Fatalf("%+v", out.Error)
			}
			want := existing
			want.Name, want.Pane, want.WorkspaceID = named("helper"), "w1:p3", "w1"
			r := out.Result.(Result)
			if !r.Renamed || !reflect.DeepEqual(r.Record, want) || *r.Parent != "0000beef" || *r.Profile != "implementer" {
				t.Fatalf("%+v", r)
			}
			if got := stored(t, c, existing.ID); !reflect.DeepEqual(got, want) {
				t.Fatalf("%+v", got)
			}
			if !reflect.DeepEqual(out.Effects, []cli.Effect{{Action: "updated", Kind: "agent_name", ID: "w1:p3"}, {Action: "updated", Kind: "agent_record", ID: existing.ID}, {Action: "updated", Kind: "pane_label", ID: "w1:p3"}, {Action: "updated", Kind: "tab", ID: "w1:t2"}}) {
				t.Fatalf("%+v", out.Effects)
			}
			var b bytes.Buffer
			if err := out.Write(&b, false, Render); err != nil || b.String() != "Adopted helper (w1:p3) as "+existing.ID+".\n" {
				t.Fatalf("%q %v", b.String(), err)
			}
		})
	}
}

// When naming a registered agent fails, or its outcome is uncertain, the record
// is left as it was.
func TestAdoptNamingRegisteredAgentFailures(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	other := agent("w1:p3", named("helper"))
	other.Agent.TerminalID = "term_b"
	for label, tc := range map[string]struct {
		rename call
		status string
		code   string
	}{
		"name taken":       {call{Method: "agent.rename", Err: &herdr.Error{Code: "agent_name_taken", Message: "taken"}}, "rejected", "agent_name_taken"},
		"launch pending":   {call{Method: "agent.rename", Err: &herdr.Error{Code: "agent_launch_pending", Message: "pending"}}, "rejected", "agent_launch_pending"},
		"mismatched reply": {call{Method: "agent.rename", Result: other}, "unknown", "protocol_error"},
		"uncertain":        {call{Method: "agent.rename", Err: &herdr.Error{Code: "timeout", Message: "slow", Uncertain: true}}, "unknown", "timeout"},
	} {
		t.Run(label, func(t *testing.T) {
			c := client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{tc.rename})...)
			existing := seed(t, c, named("worker"))
			out := Run(context.Background(), c, Options{Name: "helper"})
			if out.Status != tc.status || out.Error == nil || out.Error.Code != tc.code || out.Error.Phase != "agent.rename" || len(out.Effects) != 0 {
				t.Fatalf("%+v %+v", out, out.Error)
			}
			if got := stored(t, c, existing.ID); !reflect.DeepEqual(got, existing) {
				t.Fatalf("%+v", got)
			}
		})
	}
}

// A record that ends after adopt looked it up but before it stores the new
// name leaves a partial outcome: Herdr renamed the agent, the record did not
// change.
func TestAdoptRenamedButRecordEndedIsPartial(t *testing.T) {
	t.Setenv("HERDR_SESSION", "dev")
	var existing identity.Record
	var c libagent.Client
	rename := renaming("w1:p3")
	rename.Before = func() {
		s, err := identity.Existing(context.Background(), c.Cwd)
		if err == nil {
			err = identity.End(s, existing.ID)
		}
		if err != nil {
			t.Error(err)
		}
	}
	c = client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{rename})...)
	existing = seed(t, c, named("worker"))
	out := Run(context.Background(), c, Options{Name: "helper"})
	if out.Status != "partial" || out.Error == nil || out.Error.Code != "agent_identity_stale" || out.Error.Phase != "state" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if !reflect.DeepEqual(out.Effects, []cli.Effect{{Action: "updated", Kind: "agent_name", ID: "w1:p3"}}) {
		t.Fatalf("%+v", out.Effects)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var got identity.Record
	if err := s.Get(identity.Kind, existing.ID, &got); err != nil || got.EndedAt == nil || *got.Name != "worker" || got.Pane != "w1:p9" {
		t.Fatalf("%+v %v", got, err)
	}
}

// withSession is the live agent in pane carrying a claude session ref.
func withSession(pane string, name *string) herdr.AgentResult {
	r := agent(pane, name)
	r.Agent = identitytest.WithSession(r.Agent, "s-1")
	return r
}

func TestAdoptRecordsNativeSession(t *testing.T) {
	c := client(t, calls(verified(withSession("w1:p3", named("worker"))), verified(withSession("w1:p3", named("worker"))))...)
	out := Run(context.Background(), c, Options{})
	r := out.Result.(Result)
	if out.Error != nil || r.NativeSession == nil || r.NativeSession.Kind != "id" || r.NativeSession.Value != "s-1" || r.NativeSession.Harness != "claude" {
		t.Fatalf("%+v %+v", out.Error, r.NativeSession)
	}
	s, err := identity.Existing(context.Background(), c.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	var stored identity.Record
	if err := s.Get(identity.Kind, r.ID, &stored); err != nil || !reflect.DeepEqual(stored, r.Record) {
		t.Fatalf("%+v %v", stored, err)
	}
	if last := out.Effects[len(out.Effects)-1]; last != (cli.Effect{Action: "updated", Kind: "native_session", ID: r.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}

func TestAdoptNamingRegisteredAgentRecordsNativeSession(t *testing.T) {
	rename := renaming("w1:p3")
	rename.Result = withSession("w1:p3", named("helper"))
	c := client(t, calls(verified(agent("w1:p3", nil)), verified(agent("w1:p3", nil)), []call{rename}, labels("w1:p3", "w1:t2"))...)
	s, err := identity.OpenStore(context.Background(), c.Cwd, &cli.Outcome{})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := identity.Register(context.Background(), s, libagent.Client{}, agent("w1:p3", nil).Agent, "spawn", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), c, Options{Name: "helper"})
	r := out.Result.(Result)
	if out.Error != nil || r.ID != rec.ID || r.NativeSession == nil || r.NativeSession.Value != "s-1" {
		t.Fatalf("%+v %+v", out.Error, r)
	}
}

func TestAdoptSessionWriteFailureIsWarning(t *testing.T) {
	restore := observeSession
	t.Cleanup(func() { observeSession = restore })
	observeSession = func(*state.Store, string, herdr.AgentSession, time.Time) (identity.Record, bool, error) {
		return identity.Record{}, false, errors.New("disk full")
	}
	c := client(t, calls(verified(withSession("w1:p3", named("worker"))), verified(withSession("w1:p3", named("worker"))))...)
	out := Run(context.Background(), c, Options{})
	r := out.Result.(Result)
	if out.Status != "success" || out.Error != nil || out.ExitCode() != 0 || r.NativeSession != nil {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	if last := out.Effects[len(out.Effects)-1]; last != (cli.Effect{Action: "warning", Kind: "native_session", ID: r.ID}) {
		t.Fatalf("%+v", out.Effects)
	}
}

// An agent whose harness Herdr does not report has the same, unknown harness
// at the recheck.
func TestAdoptAgentWithoutHarness(t *testing.T) {
	unknown := agent("w1:p3", named("worker"))
	unknown.Agent.Agent = nil
	c := client(t, calls(verified(unknown), verified(unknown))...)
	out := Run(context.Background(), c, Options{})
	if out.Error != nil || out.Result.(Result).Harness != nil {
		t.Fatalf("%+v %+v", out.Error, out.Result)
	}
}
