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
	register("brood", runLock, "fledge brood FTHR-### --owner <name> [--branch <b>] [--worktree <path>] [--json]")
	register("abandon", runUnlock, "fledge abandon FTHR-### [--fledged] [--force] [--json]")
	register("broods", runLocks, "fledge broods [--stale] [--json]")
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
	fs := flag.NewFlagSet("brood", flag.ContinueOnError)
	owner := fs.String("owner", "", "brood holder name (required)")
	branch := fs.String("branch", "", "feather branch (default: current git branch)")
	worktree := fs.String("worktree", "", "feather worktree path (default: empty)")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge brood FTHR-### --owner <name>")
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
	if task.Status == spec.TaskFledged {
		return fail("%s is already fledged", id)
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
		Worktree: *worktree,
	}
	if err := lock.Acquire(r.LocksDir(), rec); err != nil {
		var held *lock.HeldError
		if errors.As(err, &held) {
			return fail("%s: %v", id, held)
		}
		return fail("%v", err)
	}
	if _, err := setTaskStatus(task, spec.TaskHatching); err != nil {
		// Roll the lock back so lock and status never desync on this path.
		lock.Release(r.LocksDir(), id)
		return fail("brooding %s: status write failed, brood rolled back: %v", id, err)
	}
	if *jsonOut {
		return emitJSON(rec)
	}
	fmt.Printf("brooding %s for %s (branch %s); status hatching\n", id, *owner, rec.Branch)
	return ExitOK
}

func runUnlock(args []string) int {
	fs := flag.NewFlagSet("abandon", flag.ContinueOnError)
	done := fs.Bool("fledged", false, "also set feather status to fledged")
	force := fs.Bool("force", false, "release even if not held; skip status changes")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) != 1 {
		return usageErr("usage: fledge abandon FTHR-### [--fledged] [--force]")
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
		// Flip to fledged BEFORE removing the brood: a crash in between leaves
		// fledged + stale brood, which `fledge preen` reports.
		if _, err := setTaskStatus(task, spec.TaskFledged); err != nil {
			return fail("%v", err)
		}
		status = spec.TaskFledged
	}
	if err := lock.Release(r.LocksDir(), id); err != nil {
		if *force {
			err = nil
		} else {
			return fail("%v", err)
		}
	}
	if !*done && !*force && task != nil && task.Status == spec.TaskHatching {
		fmt.Fprintf(os.Stderr, "note: %s is still hatching; set it explicitly with `fledge status`\n", id)
	}
	if *jsonOut {
		out := map[string]any{"feather": id, "released": true}
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
	fs := flag.NewFlagSet("broods", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	staleOnly := fs.Bool("stale", false, "only broods whose worktree is gone")
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
	recs, skipped, err := lock.List(r.LocksDir())
	if err != nil {
		return fail("%v", err)
	}
	for _, s := range skipped {
		fmt.Fprintf(os.Stderr, "warning: skipping corrupt brood file %s\n", s)
	}
	type lockOut struct {
		lock.Record
		PIDAlive       bool `json:"pid_alive"`
		WorktreeExists bool `json:"worktree_exists"`
	}
	out := make([]lockOut, 0, len(recs))
	for _, rec := range recs {
		if *staleOnly && worktreeExists(rec.Worktree) {
			continue
		}
		out = append(out, lockOut{rec, pidAlive(rec.PID), worktreeExists(rec.Worktree)})
	}
	if *jsonOut {
		return emitJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("no broods held")
		return ExitOK
	}
	for _, l := range out {
		annot := ""
		if !l.PIDAlive {
			annot += "  (pid not alive)"
		}
		if !l.WorktreeExists && !*staleOnly {
			annot += "  (worktree gone)"
		}
		fmt.Printf("%s  %s  since %s  branch %s%s\n", l.Task, l.Owner, l.Created, l.Branch, annot)
	}
	return ExitOK
}

// worktreeExists reports whether the stored worktree path still resolves to a
// directory on disk. An empty path (legacy, pre-FTHR-050 record) reports false.
// Informational only, like pidAlive: no git-registry cross-check.
func worktreeExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pidAlive is informational only: pids recycle.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil || errors.Is(syscall.Kill(pid, 0), syscall.EPERM)
}
