# Claude Code Fable — implementation review

Read-only review by the existing `fable-review` agent in Herdr pane `wQ:p5`.
Session: `f0dad138-bf66-46a6-8dc6-ddf54ebfb9d6`.
Source: final assistant text in that session transcript; thinking and tool blocks are excluded.
Timestamp: `2026-09-18T01:19:54.788Z`.
Assistant message UUID: `d3f3b963-5772-4093-8f14-06c34b86b9f0`.
The reviewer reported no blocking findings. The triage below records subsequent
repairs and remaining discussion items; the original review text follows unchanged.

## Triage after independent verification

- **D2 and T1 — repaired:** exact branch, ancestor, and descendant ref conflicts
  are rejected before filesystem effects. Regression coverage passed.
- **D3 — repaired:** a stale optional caller does not block an explicit new
  workspace. Implicit placement still requires a valid caller, and cancellation
  remains an error. Regression coverage passed.
- **D4 — repaired:** managed ignore updates append without truncating existing
  bytes. Effect reporting covers creation, zero-byte failed writes, partial
  writes, and close failures. Regression coverage passed.
- **T3/T4 — covered:** tests verify a newly opened worktree's requested initial
  tab name and explicit-pane label/focus ordering.
- **D1/T2 — repaired, independently verified, and live confirmed:** direct
  socket probes on 2026-09-18 UTC returned `pane_info` with a `pane` object.
  The reference and implementation now use that observed shape. Regression
  tests and independent verification passed. The rebuilt, installed Fledge
  successfully created, labeled, focused, and launched Claude with this path.
- **D5 — discussion:** human failures currently use stdout; choosing stderr is
  an output-contract decision. JSON remains on stdout.
- **D6 — optional:** disabling JSON HTML escaping would improve readability
  without changing decoded values.
- **S1 — optional:** review whether removing the open-mode preflight is worth
  changing the current validation sequence.
- **S2 — optional:** passing the spawn result explicitly could replace internal
  type assertions whose invariant currently holds.

The repaired implementation passed independent review, Git-aware formatting,
`go vet ./...`, `go test -race ./...`, build, and whitespace checks. The focus
repair received renewed independent verification; rebuild and installation
passed. Remaining optional changes above are for discussion.

## Live smoke checks — 2026-09-18 UTC

- Claude in `wQ:p6` / `wQ:t5` replied `FLEDGE_CLAUDE_OK` after a message sent
  using `--name` and `--body`.
- Codex in `wQ:p7` / `wQ:t5`, launched with native `--sandbox read-only`, replied
  `FLEDGE_CODEX_OK` after a multiline message sent using `--pane` and `--file -`.
- Installed Fledge launched `fledge-smoke-focus` in newly created `wQ:t6` /
  `wQ:p8` with `--label 'Focus regression test' --focus --timeout 60s`.
  The result reported success, label/focus effects, and actual argv
  `claude --model fable --permission-mode plan`. A subsequent message was
  acknowledged and received the reply `FLEDGE_FOCUS_OK`; human-readable list
  output included all three agents.

**Open follow-up:** an immediate `pane.split` → `agent.start` sequence sometimes
returned `agent_pane_busy`. Fledge correctly reported a partial outcome and
preserved the created pane. After inspecting the shell prompt, the operator
manually relaunched into that same pane successfully. Shell readiness remains
a follow-up; no automatic retry or readiness policy was added.

These checks establish these Claude and Codex launch/message paths, not live
worktree behavior or native acceptance for every supported harness/model mapping.

---

I've re-read every changed file after the in-flight repair, re-run the checks, and probed the git edge cases. Writing the verdict now.

## Verdict

No blocking findings. Every wire call matches the schema and the reference for the methods used, preflight runs before any mutation, native tokens survive exactly (confirmed end to end by the socket-backed integration test), and the partial/unknown/rejected classification is correct in every path I traced. Formatting, `go vet`, and `go test -race` all pass on the current tree. The items below are real but small, plus a few simplifications.

The known-issue list you gave me is already reflected in the current files: empty-ID `invalid_request` acceptance (`internal/herdr/client.go:68`), exact API phases via `phaseError` wrapping on malformed reads (`internal/agent/placement.go:84`, `internal/agent/worktree.go:47`), runtime exit 1 for git execution failures (`internal/agent/worktree.go:132-140`), cwd and worktree in human output, and orchestration tests for new-workspace, selector failures, malformed mutation results, and worktree create/open. I did not re-report those.

## Findings

**D1. Medium, correctness risk. The `pane.focus` result shape is unverified in either direction, and a wrong guess blocks every `--focus` spawn.**
`internal/agent/service.go:247-251` requires `type: "agent_info"` and an `agent` object. That matches `reference/herdr/api/pane.md` (pane.focus section), but that section is explicitly marked "inferred" and "Constructed from schema; not live-validated", and `pane.focus` has no CLI subcommand, so the doc probes never exercised it. Your repair list says this call "uses agent_info not pane_info", which would move away from the reference on equally unverified grounds. Trigger: any `spawn --focus`. If the server returns the other shape, the code raises a `protocol_error` with `Uncertain: true` after a mutating call, the outcome becomes `unknown`, and `agent.start` never runs. Smallest correction: decode into a struct with both optional `agent` and `pane` fields, accept either discriminator, and validate only that the returned pane ID matches. Confirm once against a scratch server before release.

**D2. Low to medium, plan deviation. Branch ref-namespace conflicts pass preflight and fail only after local filesystem effects.**
`internal/agent/worktree.go:144-152` checks only `refs/heads/<branch>` itself. Verified on a scratch repo: with a branch named `feature` present, `show-ref --verify refs/heads/feature/topic` exits 1, so `prepareWorktree` proceeds, creates `.fledge/worktrees/feature/`, writes the ignore file, and then Herdr's `git worktree add` fails with "cannot lock ref". The reverse case (`feature/topic` exists, `--branch feature` requested) behaves the same. The plan at lines 165-169 promises rejection of an existing branch before filesystem artifacts. The outcome is still a correct `partial` with preserved effects, so this is not data-losing. Smallest correction: before the existence check, also run `git show-ref --verify --quiet refs/heads/<each ancestor prefix>` and `git for-each-ref --count=1 "refs/heads/<branch>/"`, rejecting on any hit.

**D3. Low. A failing caller lookup blocks an explicit new-workspace target.**
`internal/agent/placement.go:127-138` calls `pane.current` whenever `HERDR_PANE_ID` is set, even when the destination is `--workspace NAME` for a name that does not exist. The caller is only needed there to supply `source_workspace_id`. Trigger: `HERDR_PANE_ID` exported into a shell outside its pane, or otherwise unresolvable, plus `--workspace newname`. The plan at line 277 says missing caller context must not prevent fully explicit targets. Smallest correction: when `createWorkspace` is true, treat a caller failure as "no source" rather than an error.

**D4. Low. The managed ignore file is rewritten with `O_TRUNC`, so a failed write loses the user's existing rules.**
`internal/agent/worktree.go:183-204` reads the file, rebuilds it, truncates, then writes. A write error after truncation (disk full, interrupted) leaves an empty or partial file while `effects` reports "updated". The plan promises preserved content. Smallest correction, which is also simpler than the current code: open with `O_APPEND|O_WRONLY|O_CREATE` and append only the missing newline and `*`. The read is still needed for `lastRule`, but the rebuild and truncate go away.

**D5. Low, decision. Human-mode failures go to stdout.**
`cmd/agent/*/…` pass `cmd.OutOrStdout()` to `Finish`, so in non-JSON mode "rejected: … (phase)" prints on stdout, while the root's own errors go to stderr (`cmd/cmd.go:41`). JSON on stdout is the plan's contract; human errors on stdout are a choice the plan never made. Either is defensible. If you keep it, document it; if not, route human failures to `cmd.ErrOrStderr()`.

**D6. Low, optional. JSON output HTML-escapes `<`, `>`, and `&`.**
`internal/agent/output.go:127` uses a default encoder, so a native argument like `--prompt "<task>"` appears as `\u003ctask\u003e` in `argv`. Valid JSON, harder to read. Call `SetEscapeHTML(false)` on the encoder.

## Simplifications and test gaps

- **S1. Redundant open-mode pre-check.** `internal/agent/worktree.go:73-87` resolves `--tab` conflicts against the listing before `worktree.open`. Opening an already-open worktree is a no-op that returns `already_open: true`, and `placeInWorkspace` then performs the same `tab` and `anchor` checks and rejects with only a `reused` effect. The pre-check adds nothing, and `worktree.list` (lines 44-51) is only consulted for it in open mode. Both can go for `--worktree PATH`, saving a round trip and fourteen lines.
- **S2. Hidden type assertions.** `out.Result.(*SpawnResult)` at `placement.go:173,236,245` and `worktree.go:88,107` will panic if a caller ever passes an outcome without a spawn result (the worktree tests already construct bare `Outcome{}` values for `prepareWorktree`). Passing `*SpawnResult` explicitly removes the latent panic and the coupling.
- **T1.** No test for the namespace-conflict branch in D2.
- **T2.** No test that `pane.focus` tolerates the alternate result shape (D1).
- **T3.** No test for a newly opened worktree (`already_open: false`) with `--tab`, which is the only path where `initialTab` renames after `worktree.open`.
- **T4.** `--label` and `--focus` ordering in `--pane` mode is untested; the split test covers the ordering, but not the explicit-pane path where nothing is created first.

`★ Insight ─────────────────────────────────────`
The outcome classifier in `Outcome.fail` is small because it derives status from evidence already collected: the effects list says whether a mutation is confirmed, and `herdr.Error.Uncertain` says whether the last response was trustworthy. `unknown` wins only when both "we were mutating" and "the answer is untrustworthy" hold. That is why the local-filesystem failures in `prepareWorktree` classify correctly without any special casing: they add effects but never carry `Uncertain`, so they land on `partial`, never on `unknown`.
`─────────────────────────────────────────────────`

## Residual verification limits

Nothing here was run against a live Herdr. Unverified by design: the `pane.focus` result shape (D1), whether `agent.start` accepts a pane immediately after `tab.create` or `pane.split` returns, `worktree.create` honoring an explicit `path` inside the primary checkout, and every per-harness `--model` translation (the tests establish generated argv only, as the plan states). Wire params and result discriminators for the other thirteen methods were checked against `raw/schema.json` and match.

FABLE_CODE_REVIEW_COMPLETE
