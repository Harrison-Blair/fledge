package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lock"
	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("lock", runLock, "fledge lock TASK-### --owner <name> [--branch <b>] [--json]")
	register("unlock", runUnlock, "fledge unlock TASK-### [--done] [--force] [--json]")
	register("locks", runLocks, "fledge locks [--json]")
}

// parseMixed parses args where positionals may precede flags: leading
// positionals are collected, the rest goes to fs.Parse, and any trailing
// non-flag args are appended to the positionals.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		positional = append(positional, args[i])
		i++
	}
	if err := fs.Parse(args[i:]); err != nil {
		return nil, err
	}
	return append(positional, fs.Args()...), nil
}

func runLock(args []string) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	owner := fs.String("owner", "", "lock holder name (required)")
	branch := fs.String("branch", "", "task branch (default: current git branch)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge lock TASK-### --owner <name>")
	}
	if *owner == "" {
		return usageErr("--owner is required")
	}
	id := positional[0]

	r, set, _, code, ok := loadSet()
	if !ok {
		return code
	}
	task := set.Task(id)
	if task == nil {
		return fail("%s not found", id)
	}
	if task.Status == spec.TaskDone {
		return fail("%s is already done", id)
	}
	if *branch == "" {
		out, err := exec.Command("git", "-C", r.Root, "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err == nil {
			*branch = strings.TrimSpace(string(out))
		}
	}
	rec := lock.Record{
		Task: id, Owner: *owner, PID: os.Getpid(),
		Created: time.Now().UTC().Format(time.RFC3339), Branch: *branch,
	}
	if err := lock.Acquire(r.LocksDir(), rec); err != nil {
		var held *lock.HeldError
		if errors.As(err, &held) {
			return fail("%s: %v", id, held)
		}
		return fail("%v", err)
	}
	if _, err := setTaskStatus(task, spec.TaskInProgress); err != nil {
		// Roll the lock back so lock and status never desync on this path.
		lock.Release(r.LocksDir(), id)
		return fail("locking %s: status write failed, lock rolled back: %v", id, err)
	}
	if *jsonOut {
		return emitJSON(rec)
	}
	fmt.Printf("locked %s for %s (branch %s); status in-progress\n", id, *owner, rec.Branch)
	return ExitOK
}

func runUnlock(args []string) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	done := fs.Bool("done", false, "also set task status to done")
	force := fs.Bool("force", false, "release even if not held; skip status changes")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge unlock TASK-### [--done] [--force]")
	}
	id := positional[0]

	r, set, _, code, ok := loadSet()
	if !ok {
		return code
	}
	task := set.Task(id)
	if task == nil && !*force {
		return fail("%s not found", id)
	}

	status := ""
	if *done && task != nil {
		if unchecked := uncheckedCriteria(task.Body); len(unchecked) > 0 && !*force {
			return fail("%s: acceptance criteria unchecked: %s (use --force to override)", id, strings.Join(unchecked, ", "))
		}
		// Flip to done BEFORE removing the lock: a crash in between leaves
		// done + stale lock, which `fledge check` reports.
		if _, err := setTaskStatus(task, spec.TaskDone); err != nil {
			return fail("%v", err)
		}
		status = spec.TaskDone
	}
	if err := lock.Release(r.LocksDir(), id); err != nil {
		if *force {
			err = nil
		} else {
			return fail("%v", err)
		}
	}
	if !*done && !*force && task != nil && task.Status == spec.TaskInProgress {
		fmt.Fprintf(os.Stderr, "note: %s is still in-progress; set it explicitly with `fledge status`\n", id)
	}
	if *jsonOut {
		out := map[string]any{"task": id, "released": true}
		if status != "" {
			out["status"] = status
		} else {
			out["status"] = nil
		}
		return emitJSON(out)
	}
	fmt.Printf("released %s\n", id)
	return ExitOK
}

func runLocks(args []string) int {
	fs := flag.NewFlagSet("locks", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}
	if err := r.RequireFledge(); err != nil {
		return envErr("%v", err)
	}
	recs, err := lock.List(r.LocksDir())
	if err != nil {
		return fail("%v", err)
	}
	type lockOut struct {
		lock.Record
		PIDAlive bool `json:"pid_alive"`
	}
	out := make([]lockOut, 0, len(recs))
	for _, rec := range recs {
		out = append(out, lockOut{rec, pidAlive(rec.PID)})
	}
	if *jsonOut {
		return emitJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("no locks held")
		return ExitOK
	}
	for _, l := range out {
		stale := ""
		if !l.PIDAlive {
			stale = "  (pid not alive)"
		}
		fmt.Printf("%s  %s  since %s  branch %s%s\n", l.Task, l.Owner, l.Created, l.Branch, stale)
	}
	return ExitOK
}

// pidAlive is informational only: pids recycle.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil || errors.Is(syscall.Kill(pid, 0), syscall.EPERM)
}
