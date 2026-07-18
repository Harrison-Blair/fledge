// exp2-input — EXP2: does pane.send_input {text, keys:["enter"]} reliably
// submit a prompt to an interactive Claude Code pane (the Ink TUI does not
// treat programmatic \r as submit), and do trust/permission dialogs need
// Down+Enter?
//
// Supervised harness: run only in the throwaway session `fledge-exp`, with
// the operator watching, after in-session approval. Every send is behind an
// operator gate — a human triggers each injection step, keeping clear of
// Claude Code's v2.1.200+ self-permission-change guard (docs/EXPERIMENTS.md
// §EXP2 for the full protocol and flip threshold).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdrclient"
)

func main() {
	var (
		paneID      = flag.String("pane", "", "existing Claude pane id to test against")
		spawn       = flag.Bool("spawn", false, "spawn a fresh Claude pane via agent.start instead of --pane")
		claudeCmd   = flag.String("claude-cmd", "claude", "command for the spawned Claude pane (with --spawn)")
		cwd         = flag.String("cwd", "", "cwd for the spawned pane (with --spawn)")
		text        = flag.String("text", "Reply with exactly the word pong and nothing else.", "prompt text to submit each round")
		rounds      = flag.Int("rounds", 3, "number of submit rounds (reliability sample)")
		trustDialog = flag.Bool("trust-dialog", false, "also run the Down+Enter trust/permission dialog phase")
		doReport    = flag.Bool("report", false, "append structured results to the experiments file")
		experiments = flag.String("experiments", "docs/EXPERIMENTS.md", "experiments file for --report")
		timeout     = flag.Duration("timeout", 30*time.Second, "per-call timeout")
		settle      = flag.Duration("settle", 5*time.Second, "wait after each send before reading the pane")
	)
	flag.Parse()

	requireExpSession()
	if *paneID == "" && !*spawn {
		fmt.Fprintln(os.Stderr, "need --pane <id> or --spawn (see docs/EXPERIMENTS.md §EXP2)")
		os.Exit(2)
	}

	ctx := context.Background()
	c := dial(ctx)
	defer c.Close()

	r := &report{Exp: "EXP2", Harness: "exp2-input", Started: time.Now()}
	{
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		snapshotVersions(cctx, c, r)
		cancel()
	}

	readTail := func(label string) {
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		res, resp, err := c.PaneRead(cctx, herdrclient.ReadParams{
			PaneID: *paneID, Source: "recent-unwrapped", Lines: 15,
		})
		if err != nil {
			r.obs("%s: pane.read FAILED: %v", label, err)
			return
		}
		if res.Text != "" {
			r.obs("%s: pane tail: %q", label, res.Text)
		} else {
			r.obs("%s: pane tail (raw): %s", label, resp.Raw)
		}
	}

	if *spawn && gate("spawn Claude pane in fledge-exp: agent.start exp2-claude -- "+*claudeCmd) {
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		res, resp, err := c.AgentStart(cctx, herdrclient.AgentStartParams{
			Name: "exp2-claude", Cwd: *cwd, Split: "right", Command: []string{*claudeCmd},
		})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent.start failed: %v\n", err)
			os.Exit(1)
		}
		*paneID = res.PaneID
		r.obs("spawned Claude pane %q (raw: %s)", res.PaneID, resp.Raw)
		fmt.Println("wait for Claude to finish starting (and pass any trust prompt) before proceeding.")
	}
	if *paneID == "" {
		fmt.Fprintln(os.Stderr, "no pane to test against; aborting")
		os.Exit(1)
	}

	// Phase 1: text + real Enter submits, sampled over --rounds.
	submitted := 0
	attempted := 0
	for i := 1; i <= *rounds; i++ {
		step := fmt.Sprintf("round %d/%d: pane.send_input {text:%q, keys:[\"enter\"]}", i, *rounds, *text)
		if !gate(step) {
			continue
		}
		attempted++
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		err := c.PaneSendInput(cctx, herdrclient.SendInputParams{
			PaneID: *paneID, Text: *text, Keys: []string{"enter"},
		})
		cancel()
		if err != nil {
			r.obs("round %d: pane.send_input FAILED: %v", i, err)
			continue
		}
		time.Sleep(*settle)
		readTail(fmt.Sprintf("round %d", i))
		ans := ask(fmt.Sprintf("round %d: did the prompt SUBMIT (Claude started responding)? (y/n/notes):", i))
		r.obs("round %d: operator answer: %s", i, ans)
		if ans == "y" || ans == "Y" {
			submitted++
		}
	}
	r.obs("submit reliability: %d/%d gated sends submitted", submitted, attempted)

	// Phase 2 (optional): trust/permission dialog needs Down+Enter.
	if *trustDialog {
		fmt.Println("\nSet up a trust or --dangerously-skip-permissions dialog in the pane")
		fmt.Println("(e.g. restart Claude in a fresh directory), then continue.")
		if gate("send keys [\"down\"] then [\"enter\"] to accept the visible dialog") {
			for _, key := range []string{"down", "enter"} {
				if !gate("send key \"" + key + "\"") {
					continue
				}
				cctx, cancel := context.WithTimeout(ctx, *timeout)
				err := c.PaneSendKeys(cctx, *paneID, key)
				cancel()
				if err != nil {
					r.obs("trust dialog: send_keys %q FAILED: %v", key, err)
				} else {
					r.obs("trust dialog: sent key %q", key)
				}
				time.Sleep(*settle)
			}
			readTail("trust dialog")
			r.obs("trust dialog: operator answer — dialog accepted?: %s",
				ask("Was the dialog accepted by Down+Enter? (y/n/notes):"))
		}
	}

	r.finish(*doReport, *experiments)
}
