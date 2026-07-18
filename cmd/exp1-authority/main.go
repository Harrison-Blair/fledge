// exp1-authority — EXP1: does pane.report_agent --source custom:* suppress
// Herdr's screen-manifest detection on a Claude Code pane, and does
// pane.clear_agent_authority restore it?
//
// Supervised harness: run only in the throwaway session `fledge-exp`, with
// the operator watching, after in-session approval (docs/EXPERIMENTS.md §EXP1
// for the full protocol and flip threshold; results feed docs/DECISIONS.md).
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
		source      = flag.String("source", "custom:test", "authority source to report with (must be custom:*)")
		doReport    = flag.Bool("report", false, "append structured results to the experiments file")
		experiments = flag.String("experiments", "docs/EXPERIMENTS.md", "experiments file for --report")
		timeout     = flag.Duration("timeout", 30*time.Second, "per-call timeout")
	)
	flag.Parse()

	requireExpSession()
	if *paneID == "" && !*spawn {
		fmt.Fprintln(os.Stderr, "need --pane <id> or --spawn (see docs/EXPERIMENTS.md §EXP1)")
		os.Exit(2)
	}

	ctx := context.Background()
	c := dial(ctx)
	defer c.Close()

	r := &report{Exp: "EXP1", Harness: "exp1-authority", Started: time.Now()}
	{
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		snapshotVersions(cctx, c, r)
		cancel()
	}

	explain := func(label string) {
		// Pivotal ADR-012 signal: protocol 16 exposes screen-detection
		// suppression as the boolean screen_detection_skipped on the agent
		// record (agent.get), replacing v15's screen_detection_skip_reason.
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		if info, _, err := c.AgentGet(cctx, *paneID); err != nil {
			r.obs("%s: agent.get FAILED: %v", label, err)
		} else {
			r.obs("%s: agent=%q agent_status=%q screen_detection_skipped=%v",
				label, info.Agent, info.AgentStatus, info.ScreenDetectionSkipped)
		}
		cancel()

		// Full authority explanation, recorded raw for the operator/report
		// (the v16 explain payload is server-defined and untyped).
		cctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
		if _, resp, err := c.AgentExplainPane(cctx, *paneID); err != nil {
			r.obs("%s: agent.explain FAILED: %v", label, err)
		} else {
			r.obs("%s: raw explain: %s", label, resp.Raw)
		}
	}

	if *spawn && gate("spawn Claude pane in fledge-exp: agent.start exp1-claude -- "+*claudeCmd) {
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		res, resp, err := c.AgentStart(cctx, herdrclient.AgentStartParams{
			Name: "exp1-claude", Cwd: *cwd, Split: "right", Argv: []string{*claudeCmd},
		})
		cancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "agent.start failed: %v\n", err)
			os.Exit(1)
		}
		*paneID = res.PaneID
		r.obs("spawned Claude pane %q (raw: %s)", res.PaneID, resp.Raw)
	}
	if *paneID == "" {
		fmt.Fprintln(os.Stderr, "no pane to test against; aborting")
		os.Exit(1)
	}

	// (a) Baseline: screen-manifest blocked detection.
	fmt.Println("\n(a) In the fledge-exp UI, give the Claude pane a task that triggers a")
	fmt.Println("    permission prompt, and wait until the dialog is visible.")
	if gate("record baseline state via agent.explain (expect blocked, via screen manifest)") {
		explain("baseline")
	}

	// (b) Seize authority and check whether screen detection is suppressed.
	if gate(fmt.Sprintf("pane.report_agent {source:%q, state:working, seq:1} on pane %s", *source, *paneID)) {
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		err := c.PaneReportAgent(cctx, herdrclient.ReportAgentParams{
			PaneID: *paneID, Source: *source, Agent: "exp1", State: "working", Seq: 1,
		})
		cancel()
		if err != nil {
			r.obs("pane.report_agent FAILED: %v", err)
		} else {
			r.obs("pane.report_agent accepted (source=%s state=working seq=1)", *source)
		}
		explain("after report_agent")
		r.obs("operator answer — sidebar still shows blocked for the visible dialog?: %s",
			ask("Does the sidebar still show blocked? (y/n/notes):"))
	}

	// (c) Hand authority back; screen detection should resume.
	if gate(fmt.Sprintf("pane.clear_agent_authority {source:%q} on pane %s", *source, *paneID)) {
		cctx, cancel := context.WithTimeout(ctx, *timeout)
		err := c.PaneClearAgentAuthority(cctx, herdrclient.ClearAgentAuthorityParams{
			PaneID: *paneID, Source: *source,
		})
		cancel()
		if err != nil {
			r.obs("pane.clear_agent_authority FAILED: %v", err)
		} else {
			r.obs("pane.clear_agent_authority accepted (source=%s)", *source)
		}
		explain("after clear_agent_authority")
		r.obs("operator answer — screen detection resumed?: %s",
			ask("Did screen detection resume (blocked shown again)? (y/n/notes):"))
	}

	r.finish(*doReport, *experiments)
}
