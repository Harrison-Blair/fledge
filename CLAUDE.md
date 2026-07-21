# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## cli flags
cli flags follow the same unified convention throughout the project:

--[wholeflag]
-[CAPITAL-LETTER]

example:
--version
-V


## Agent first design
when adding new commands, prompt the user if they want a --json output

## Commands

```bash
./scripts/build.sh              # go build -> bin/fledge
./scripts/install.sh            # copy to GOBIN (or GOPATH/bin); BINDIR= to override
go test ./...
go test -run TestGet ./internal/version/   # single test
gofmt -l . && go vet ./...
```

No Makefile, no CI, no dependencies — `go.mod` has zero requires, Go 1.26.

## `docs/` is a completed legacy experiment

Everything in `docs/` predates this project and documents a prior exploration
("Stage 0") that has been **run to completion**. Its roadmap, ground rules, and
repo layout are historical; the code it describes was deleted in `bf69715
teardown for re-write`. Do not treat it as the current plan or restore what it
describes — `internal/herdrclient/`, `cmd/exp*/`, and
`scripts/gen-herdr-types.sh` are listed throughout and none exist.

What carries forward is the verified findings. `docs/handoff-stage0.md` is a
finished commissioning brief — ignore it as instruction entirely.
`docs/reference/*` is a fixed 2026-07-17 snapshot, never edited (ADR-006).

## What the experiment established

- **EXP1** — `pane.report_agent --source custom:*` is accepted on a Claude pane
  but does **not** seize authority or suppress screen detection; native
  detection wins. Custom reports can't break `blocked`, but metadata-only stays
  the rule. Unverified: `clear_agent_authority` vs `release_agent` semantics.
- **EXP2** — `pane.send_input {text, keys:["enter"]}` submits reliably (3/3) to
  an interactive Claude TUI. Workers run in visible panes; no `-p` fallback.
- **EXP3** — no practical concurrency ceiling. Don't pre-cap concurrent panes;
  treat rate limits reactively via the `StopFailure`/`rate_limit` hook (ADR-014),
  which is authoritative — pane-output scraping is only a hint, and once
  false-positived on this repo's own text.

Raw observations: `docs/EXPERIMENTS.md`. Herdr protocol-16 wire facts: ADR-017.

## What fledge is

A **zero-inference Go orchestrator** for a multi-agent coding stack: Herdr (pane
bus), Pi, and Claude Code. Two invariants:

1. **The Go CLI is the state authority.** Herdr and agent events are *input
   signals*; fledge's own store is truth. Herdr loses token metadata across
   server restarts, so it is never durable state.
2. **Zero inference in the orchestrator.** It issues socket commands, consumes
   events, advances a deterministic FSM, and writes its log — never an LLM call.
   All inference happens inside visible, operator-interactable panes.

## Current state

HEAD is a skeleton: `cmd/fledge/main.go` (hand-rolled `switch`, no flag
package), `internal/scaffold` (creates the `.fledge/` tree), `internal/version`.
That is all the Go code. Git tags reach `v0.6.10` but predate the rewrite.

`internal/version/VERSION` is the single source of truth, `//go:embed`-ed by
`version.go` — bumping means editing that one file. No ldflags, no duplicated
constant. It sits inside the package because embed can't cross directory
boundaries; a future release workflow must watch that path, not a root
`VERSION`.

**Git history is not design input** (ADR-010) — don't resurrect designs from it.
That's about *design*, not forensics; reading history to see what was deleted is
fine.

## Re-verify before you rely

Herdr, Pi, and Claude Code are fast-moving pre-1.0 surfaces; the versions pinned
in `docs/INTEGRATION-CONTRACTS.md` are from 2026-07-17. Check live (`herdr api
schema --json`) before building on any version-specific claim. ADR-017 shows
what drift looks like: the client targeted protocol v15, the server was 16, and
five shapes differed.
