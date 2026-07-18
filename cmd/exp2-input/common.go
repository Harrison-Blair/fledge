package main

// Shared harness plumbing. Deliberately duplicated across cmd/exp*/ instead
// of adding a package outside the commissioned Stage 0 layout
// (docs/handoff-stage0.md §4). Keep the three copies in sync.
//
// Ground rules enforced here:
//   - Experiments run ONLY against the throwaway session `fledge-exp`
//     (HERDR_SESSION), never a session where real work lives.
//   - Harnesses issue socket commands, read events, and record observations;
//     they make no LLM calls.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdrclient"
)

const expSession = "fledge-exp"

// requireExpSession refuses to run outside the throwaway session.
func requireExpSession() {
	if got := os.Getenv("HERDR_SESSION"); got != expSession {
		fmt.Fprintf(os.Stderr,
			"refusing to run: HERDR_SESSION=%q, want %q.\n"+
				"Experiments deliberately churn pane authority and must never touch a\n"+
				"session where real work lives. Start the throwaway session with\n"+
				"scripts/exp-session-up.sh and rerun with HERDR_SESSION=%s.\n",
			got, expSession, expSession)
		os.Exit(2)
	}
}

func dial(ctx context.Context) *herdrclient.Client {
	c, err := herdrclient.Dial(ctx, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot reach the %s Herdr session: %v\n", expSession, err)
		fmt.Fprintln(os.Stderr, "is it up? scripts/exp-session-up.sh")
		os.Exit(1)
	}
	return c
}

var stdin = bufio.NewReader(os.Stdin)

// ask prints a prompt and returns one trimmed line from the operator.
func ask(prompt string) string {
	fmt.Printf("%s ", prompt)
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// gate pauses until the operator explicitly proceeds; returns false on skip.
// Every state-changing step in a supervised experiment sits behind one of
// these so the human drives the run.
func gate(step string) bool {
	for {
		switch strings.ToLower(ask(fmt.Sprintf("\n[step] %s\n  Enter=proceed, s=skip, q=abort:", step))) {
		case "":
			return true
		case "s":
			return false
		case "q":
			fmt.Println("aborted by operator")
			os.Exit(1)
		}
	}
}

// report accumulates structured observations and appends them into the
// experiment's Results section in docs/EXPERIMENTS.md (--report mode).
type report struct {
	Exp      string // "EXP1" | "EXP2" | "EXP3"
	Harness  string
	Started  time.Time
	Versions []string // "Herdr: ...", "Claude Code: ...", "Pi: ..."
	Obs      []string
	Verdict  string
}

// obs records one observation and echoes it to the terminal.
func (r *report) obs(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	r.Obs = append(r.Obs, line)
	fmt.Println("  [obs] " + line)
}

// appendTo inserts a run block just before the experiment's end marker,
// e.g. <!-- END RESULTS EXP1 -->, in the experiments file.
func (r *report) appendTo(path string) error {
	marker := fmt.Sprintf("<!-- END RESULTS %s -->", r.Exp)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	idx := strings.Index(string(data), marker)
	if idx < 0 {
		return fmt.Errorf("marker %q not found in %s", marker, path)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "#### Run %s — %s\n\n", r.Started.UTC().Format("2006-01-02 15:04 MST"), r.Harness)
	for _, v := range r.Versions {
		fmt.Fprintf(&b, "- Version — %s\n", v)
	}
	b.WriteString("- Raw observations:\n")
	if len(r.Obs) == 0 {
		b.WriteString("  - (none recorded)\n")
	}
	for _, o := range r.Obs {
		fmt.Fprintf(&b, "  - %s\n", o)
	}
	verdict := r.Verdict
	if verdict == "" {
		verdict = "(not recorded)"
	}
	fmt.Fprintf(&b, "- Verdict: %s\n\n", verdict)

	out := string(data[:idx]) + b.String() + string(data[idx:])
	return os.WriteFile(path, []byte(out), 0o644)
}

// finish prompts for the verdict and, when reporting is enabled, appends the
// run block.
func (r *report) finish(doReport bool, experimentsPath string) {
	r.Verdict = ask("\nOperator verdict for " + r.Exp + " (free text, one line):")
	if !doReport {
		fmt.Println("(--report not set; results NOT written to", experimentsPath+")")
		return
	}
	if err := r.appendTo(experimentsPath); err != nil {
		fmt.Fprintf(os.Stderr, "failed to append results: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("results appended to", experimentsPath)
}

// snapshotVersions records Herdr version/protocol from session.snapshot and
// asks the operator for the agent versions (the harness does not exec agent
// binaries).
func snapshotVersions(ctx context.Context, c *herdrclient.Client, r *report) {
	if s, err := c.SessionSnapshot(ctx); err == nil {
		r.Versions = append(r.Versions,
			fmt.Sprintf("Herdr: %s (protocol v%d)", s.Version, s.ProtocolVersion))
	} else {
		r.Versions = append(r.Versions, "Herdr: unknown (session.snapshot failed: "+err.Error()+")")
	}
	if v := ask("Claude Code version (`claude --version`, blank to skip):"); v != "" {
		r.Versions = append(r.Versions, "Claude Code: "+v)
	}
	if v := ask("Pi version (`pi --version`, blank to skip):"); v != "" {
		r.Versions = append(r.Versions, "Pi: "+v)
	}
}
