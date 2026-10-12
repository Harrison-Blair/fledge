package adopt

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// statLimit bounds one /proc/<pid>/stat read, maxAncestry the parents read
// from this command up, and maxPID the PIDs accepted as evidence (Linux's
// PID_MAX_LIMIT).
const statLimit, maxAncestry, maxPID = 4096, 128, 1 << 22

// platform, selfPID, and openStat are replaceable so tests can serve a
// process tree; every process read goes through stat.
var (
	platform = runtime.GOOS
	selfPID  = os.Getpid
	openStat = func(pid int) (io.ReadCloser, error) { return os.Open("/proc/" + strconv.Itoa(pid) + "/stat") }
)

// paneProcesses is the pane.process_info result subset adopt checks. PIDs
// decode as any number so a negative or oversized one is rejected as
// evidence, not as a malformed reply.
type paneProcesses struct {
	Type        string `json:"type"`
	ProcessInfo *struct {
		PaneID     string `json:"pane_id"`
		Foreground []struct {
			PID *float64 `json:"pid"`
		} `json:"foreground_processes"`
	} `json:"process_info"`
}

// process is the subset of one /proc/<pid>/stat line adopt reads.
type process struct {
	pid, ppid int
	start     uint64
}

func unverified(format string, args ...any) error {
	return &cli.Error{Code: "caller_identity_unverified", Message: fmt.Sprintf(format, args...)}
}

// owns verifies that this command runs inside a, the caller's agent: one of
// the foreground processes Herdr reports for a's pane must be this command
// or one of its ancestors. A shell, a working directory, a name, or the
// environment proves nothing, and missing evidence fails closed.
func owns(ctx context.Context, c libagent.Client, a herdr.AgentDetails) error {
	if platform != "linux" {
		return unverified("adopt verifies the caller through Linux /proc; %s is not supported", platform)
	}
	var r paneProcesses
	if err := c.Call(ctx, "pane.process_info", map[string]any{"pane_id": a.PaneID}, &r); err != nil {
		return err
	}
	if r.Type != "pane_process_info" || r.ProcessInfo == nil || r.ProcessInfo.PaneID != a.PaneID {
		return unverified("pane.process_info did not describe %s", a.PaneID)
	}
	foreground := map[int]bool{}
	for _, p := range r.ProcessInfo.Foreground {
		if p.PID == nil || *p.PID < 1 || *p.PID > maxPID || *p.PID != float64(int(*p.PID)) {
			return unverified("pane.process_info reported an invalid process for %s", a.PaneID)
		}
		if *p.PID > 1 {
			foreground[int(*p.PID)] = true
		}
	}
	if len(foreground) == 0 {
		return unverified("pane.process_info reported no foreground process for %s", a.PaneID)
	}
	cur, err := stat(selfPID())
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	for steps := 0; ; steps++ {
		if foreground[cur.pid] {
			return nil
		}
		if cur.pid == 1 {
			break
		}
		if steps == maxAncestry {
			return unverified("this command has more than %d ancestors", maxAncestry)
		}
		seen[cur.pid] = true
		if cur.ppid < 1 || seen[cur.ppid] {
			return unverified("process %d has no valid parent", cur.pid)
		}
		parent, err := stat(cur.ppid)
		if err != nil {
			return err
		}
		// A parent cannot start after its child; one that did reuses the PID.
		if parent.start > cur.start {
			return unverified("process %d was replaced", parent.pid)
		}
		cur = parent
	}
	return unverified("no foreground process of %s runs this command", a.PaneID)
}

// stat reads and parses /proc/<pid>/stat, reading at most statLimit bytes.
// The command name sits in parentheses and may itself hold spaces and
// parentheses, so the fields start after the last ')'.
func stat(pid int) (process, error) {
	if pid < 1 || pid > maxPID {
		return process{}, unverified("process %d is not a valid PID", pid)
	}
	f, err := openStat(pid)
	if err != nil {
		return process{}, unverified("cannot read process %d: %v", pid, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, statLimit+1))
	if err != nil {
		return process{}, unverified("cannot read process %d: %v", pid, err)
	}
	if len(data) > statLimit {
		return process{}, unverified("process %d stat is longer than %d bytes", pid, statLimit)
	}
	s := string(data)
	name, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if name < 0 || end < name || s[:name] != strconv.Itoa(pid)+" " || !strings.HasPrefix(s[end+1:], " ") {
		return process{}, unverified("process %d stat is malformed", pid)
	}
	// Fields after the name: state, ppid, ..., starttime (the 20th).
	fields := strings.Fields(s[end+1:])
	if len(fields) < 20 {
		return process{}, unverified("process %d stat is malformed", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	start, err2 := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || err2 != nil || ppid < 0 {
		return process{}, unverified("process %d stat is malformed", pid)
	}
	return process{pid: pid, ppid: ppid, start: start}, nil
}
