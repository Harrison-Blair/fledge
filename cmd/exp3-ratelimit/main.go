// exp3-ratelimit — EXP3: how many concurrent Claude Code panes can run
// representative work before sustained subscription throttling appears?
//
// *** NEVER RUN BY AN AGENT. HUMAN OPERATOR ONLY. ***
// This experiment burns real Claude subscription quota (pooled across ALL
// Claude usage on the account) over hours of wall clock. Stage 0 delivers
// the harness and the written protocol only; execution is exclusively the
// operator's, at a time they choose (docs/EXPERIMENTS.md §EXP3).
//
// The harness spawns N concurrent Claude panes in the throwaway session
// `fledge-exp`, each fed one task from --tasks, then watches for throttle
// signals in pane output and in an optional StopFailure/rate_limit hook
// capture file, logging time-to-first-throttle. It makes no LLM calls
// itself; all inference happens inside the spawned panes.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdrclient"
)

// throttleRe matches an ACTIVE Claude throttle in pane output — a corroborator
// only; the hook capture path (--hook-capture, StopFailure/rate_limit) is the
// authoritative signal. Patterns intentionally require a "reached"/error form:
// the earlier bare substrings (rate.?limit, usage limit, weekly limit, 429,
// overloaded) false-positived on (a) Claude's ever-present "N% of weekly limit"
// status banner and (b) any repo whose own text discusses rate limiting — which
// invalidated the first n=2 run when panes ran with cwd inside this repo. Run
// panes in a neutral cwd (see exp3-tasks.txt) so a match means a real throttle.
var throttleRe = regexp.MustCompile(`(?i)\blimit reached\b|rate limit exceeded|rate_limit_error|overloaded_error`)

func main() {
	var (
		operatorAck = flag.Bool("i-am-the-operator", false, "REQUIRED: assert a human operator is running this, per the EXP3 protocol")
		n           = flag.Int("n", 2, "number of concurrent Claude panes (protocol: 2, then 3, then 4)")
		tasksPath   = flag.String("tasks", "", "REQUIRED: file with one representative task prompt per line")
		claudeCmd   = flag.String("claude-cmd", "claude", "Claude command for spawned panes")
		claudeFlags = flag.String("claude-flags", "--permission-mode bypassPermissions", "space-separated flags inserted before the task prompt so panes run unattended (no approval prompts); set \"\" for interactive approval")
		cwd         = flag.String("cwd", "", "cwd for spawned panes")
		sustain     = flag.Bool("sustain", false, "keep panes under CONTINUOUS load: when a pane goes idle it is re-fed the next task (cycling --tasks) via pane.send_input instead of stopping. Required to probe the true ceiling — finite tasks only give a lower bound. Without this, the run stops as soon as tasks finish.")
		hookCapture = flag.String("hook-capture", "", "file a StopFailure/rate_limit hook appends to (see EXP3 protocol); watched when set")
		poll        = flag.Duration("poll", 30*time.Second, "pane output poll interval")
		duration    = flag.Duration("duration", 6*time.Hour, "maximum watch duration")
		doReport    = flag.Bool("report", false, "append structured results to the experiments file")
		experiments = flag.String("experiments", "docs/EXPERIMENTS.md", "experiments file for --report")
		timeout     = flag.Duration("timeout", 30*time.Second, "per-call timeout")
	)
	flag.Parse()

	requireExpSession()
	if !*operatorAck {
		fmt.Fprintln(os.Stderr, "refusing to run: EXP3 is human-executed only — it burns real Claude")
		fmt.Fprintln(os.Stderr, "subscription quota (pooled account-wide) for hours. If you are the")
		fmt.Fprintln(os.Stderr, "human operator and this is a deliberately chosen low-stakes time,")
		fmt.Fprintln(os.Stderr, "re-run with --i-am-the-operator. See docs/EXPERIMENTS.md §EXP3.")
		os.Exit(2)
	}
	if *tasksPath == "" {
		fmt.Fprintln(os.Stderr, "need --tasks <file> (one representative task prompt per line)")
		os.Exit(2)
	}
	tasks, err := readTasks(*tasksPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read tasks: %v\n", err)
		os.Exit(1)
	}
	if len(tasks) < *n {
		fmt.Fprintf(os.Stderr, "need at least %d tasks in %s, have %d\n", *n, *tasksPath, len(tasks))
		os.Exit(1)
	}

	ctx := context.Background()
	c := dial(ctx)
	defer c.Close()

	r := &report{Exp: "EXP3", Harness: "exp3-ratelimit", Started: time.Now()}
	{
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		snapshotVersions(cctx, c, r)
		cancel()
	}
	r.obs("config: n=%d poll=%s max duration=%s hook-capture=%q sustain=%v", *n, *poll, *duration, *hookCapture, *sustain)

	// Spawn N panes, each running Claude with its task as the initial prompt.
	start := time.Now()
	panes := make(map[string]int) // pane id -> worker index
	for i := 0; i < *n; i++ {
		name := fmt.Sprintf("exp3-w%d", i+1)
		argv := append([]string{*claudeCmd}, strings.Fields(*claudeFlags)...)
		argv = append(argv, tasks[i])
		if !gate(fmt.Sprintf("spawn %s: agent.start -- %s", name, strings.Join(argv, " "))) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		res, _, err := c.AgentStart(cctx, herdrclient.AgentStartParams{
			Name: name, Cwd: *cwd, Split: "right",
			Argv: argv,
		})
		cancel()
		if err != nil {
			r.obs("%s: agent.start FAILED: %v", name, err)
			continue
		}
		panes[res.PaneID] = i + 1
		r.obs("%s: spawned pane %s at t=%s", name, res.PaneID, time.Since(start).Round(time.Second))
	}
	if len(panes) == 0 {
		fmt.Fprintln(os.Stderr, "no panes spawned; aborting")
		os.Exit(1)
	}

	// Watch loop: poll pane output for throttle markers; tail the hook
	// capture file when configured. Records time-to-first-throttle per pane.
	fmt.Printf("\nwatching %d pane(s) for throttle signals (Ctrl+C to stop early)...\n", len(panes))
	throttled := make(map[string]time.Duration)
	var hookOffset int64
	idlePolls := 0   // consecutive polls with no pane actively working
	refeedIdx := *n  // next task to re-feed (cycles --tasks); first n already spawned
	deadline := time.Now().Add(*duration)
	for time.Now().Before(deadline) && len(throttled) < len(panes) {
		time.Sleep(*poll)

		working := 0
		for paneID, w := range panes {
			if _, done := throttled[paneID]; done {
				continue
			}
			cctx, cancel := context.WithTimeout(ctx, *timeout)
			res, resp, err := c.PaneRead(cctx, herdrclient.ReadParams{
				PaneID: paneID, Source: "recent_unwrapped", Lines: 60,
			})
			cancel()
			if err != nil {
				r.obs("w%d: pane.read error at t=%s: %v", w, time.Since(start).Round(time.Second), err)
				continue
			}
			text := res.Text
			if text == "" {
				text = string(resp.Raw)
			}
			if m := throttleRe.FindString(text); m != "" {
				elapsed := time.Since(start)
				throttled[paneID] = elapsed
				r.obs("w%d: FIRST THROTTLE SIGNAL %q in pane output at t=%s", w, m, elapsed.Round(time.Second))
				continue
			}
			// Liveness: is this pane still generating load? A finished task
			// leaves the Claude pane idle, which produces no throttle pressure.
			gctx, gcancel := context.WithTimeout(ctx, *timeout)
			info, _, gerr := c.AgentGet(gctx, paneID)
			gcancel()
			busy := gerr == nil && info.AgentStatus == "working"
			if busy {
				working++
			} else if *sustain {
				// Re-feed the idle pane the next task to sustain continuous
				// load (EXP2 proved text+enter submits to a Claude pane). This
				// is what turns a burst into the sustained pressure the ceiling
				// question needs.
				task := tasks[refeedIdx%len(tasks)]
				refeedIdx++
				sctx, scancel := context.WithTimeout(ctx, *timeout)
				serr := c.PaneSendInput(sctx, herdrclient.SendInputParams{
					PaneID: paneID, Text: task, Keys: []string{"enter"},
				})
				scancel()
				if serr != nil {
					r.obs("w%d: re-feed FAILED at t=%s: %v", w, time.Since(start).Round(time.Second), serr)
				} else {
					r.obs("w%d: re-fed task #%d at t=%s (sustaining load)", w, refeedIdx, time.Since(start).Round(time.Second))
					working++ // now under load again
				}
			}
		}

		if *hookCapture != "" {
			lines, newOffset, err := tailFile(*hookCapture, hookOffset)
			if err == nil {
				hookOffset = newOffset
				for _, l := range lines {
					r.obs("hook capture at t=%s: %s", time.Since(start).Round(time.Second), l)
				}
			}
		}

		// Heartbeat so the watch is visibly alive (an idle wait looked hung).
		elapsed := time.Since(start).Round(time.Second)
		fmt.Printf("  [t=%s] %d/%d throttled, %d working (deadline %s)\n",
			elapsed, len(throttled), len(panes), working, deadline.Sub(start).Round(time.Second))

		// Burst mode (no --sustain): stop early once the load is gone. All panes
		// idle for two consecutive polls means the tasks completed without
		// throttling — a lower bound only. Ceiling probing uses --sustain, which
		// re-feeds idle panes above and so never reaches working==0 until throttle
		// or deadline.
		if !*sustain {
			if working == 0 {
				idlePolls++
				if idlePolls >= 2 {
					r.obs("all panes idle for %d polls at t=%s — tasks completed, no throttle under this load (n=%d)",
						idlePolls, elapsed, len(panes))
					fmt.Println("  all panes idle — tasks finished; stopping (no throttle under this load). Use --sustain to probe the ceiling.")
					break
				}
			} else {
				idlePolls = 0
			}
		}
	}

	if len(throttled) == 0 {
		r.obs("no throttle signal observed within %s at n=%d", *duration, *n)
	} else {
		for paneID, d := range throttled {
			r.obs("summary: w%d (pane %s) first throttle at t=%s", panes[paneID], paneID, d.Round(time.Second))
		}
	}
	r.finish(*doReport, *experiments)
}

func readTasks(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tasks []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			tasks = append(tasks, line)
		}
	}
	return tasks, sc.Err()
}

// tailFile returns complete new lines appended since offset.
func tailFile(path string, offset int64) ([]string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset, err
	}
	var lines []string
	read := offset
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		read += int64(len(sc.Bytes())) + 1
	}
	return lines, read, sc.Err()
}
