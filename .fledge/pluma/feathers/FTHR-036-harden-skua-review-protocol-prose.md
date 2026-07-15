---
id: FTHR-036
title: Harden skua review protocol prose
plumage: PLM-018
status: hatching
priority: P2
depends_on: []
authored: 2026-07-15T18:47:12Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# FTHR-036: Harden skua review protocol prose

## Description
Rewrite the `## Skua` section of the agent-neutral `worker-protocols.md` (specifically `### Reviewing a feather` and `### Verdict`) so the skua reviews more adversarially on three axes: (1) a brooder's pushback only withdraws a finding on disproof the skua independently re-verifies — never on a bare counter-assertion; (2) evidence is guilty until proven — ambiguous, incomplete, or terse-log-only `## AC-N` sections are treated as NOT proof; (3) the skua runs an explicit, every-cycle red-team pass hunting for edge cases the spec's Tests section fails to pin, reporting gaps as findings only (never fixing or writing tests itself). The 3-rejection escalation threshold and the entire `## Brooder` section are explicitly left untouched. Satisfies PLM-018 FC-1 through FC-8.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` — the single agent-neutral source; only the `## Skua` section changes (`### Reviewing a feather` checklist and `### Verdict` concession paragraph). See `.fledge/nest/architecture.md` → "Skua review protocol" and "The orchestration workflow (agent-neutral prose)".
- Do NOT touch `## Brooder`, `## Incubator`, the `### Lifecycle`/`### Communication rules` subsections of `## Skua`, or any harness adapter file (`internal/bootstrap/adapters/...`) — those are out of scope for this feather. Do NOT run `go install`/`fledge init --refresh` in this feather — that resync is FTHR-B, which depends on this one.

## Approach
Edit only within `## Skua` in `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md`:

1. In `### Reviewing a feather`, replace item 5 ("Criteria audit") with hardened text making the guilty-until-proven default explicit: an `## AC-N` section that is ambiguous, incomplete, or backed only by a terse/summarized log (e.g. an exit code or one-line summary, no visible assertions/diffs/output) is NOT proof — the skua leaves that box unchecked and files a finding. Keep "re-run commands where cheap," but add: for any command not re-run, the recorded output itself must be sufficient to independently confirm the claim. The rest of item 5 (checking boxes, committing `review: verify FTHR-### AC-1..N`, uncheck-on-invalidation) is unchanged.
2. Insert a new checklist item **after** item 3 ("Diff vs. spec") and **before** the item that becomes "Scope and simplicity" (renumbering that and the criteria-audit item by one): a "Red-team pass" item, run every review cycle (not only the first), directing the skua to read the implementation for branches/inputs/interactions the spec's Tests section never names, and probe them using only throwaway, never-committed means (ad hoc invocations with uncovered inputs, or a scratch test file kept outside the tracked worktree). Any gap found is reported as a numbered finding — the skua never writes or commits the missing test itself.
3. In `### Verdict`, replace the closing paragraph ("If a brooder pushes back on a finding with a fact verified to be correct, withdraw the finding; if the disagreement is a judgment call that can't be resolved in one round, escalate to the orchestrator rather than looping.") with the hardened concession rule: a finding withdraws only when the brooder supplies concrete, independently checkable disproof (a specific test run, line reference, or spec citation that directly contradicts the finding) **and** the skua itself re-verifies that disproof (re-runs the cited command, reads the cited line/spec text) before withdrawing; a bare counter-assertion, re-explanation, or unverified "that's intentional" never withdraws a finding — if the disproof doesn't meet this bar, the finding stands; a genuine judgment call unresolved after one round still escalates to the orchestrator as before.
4. Leave the "Third rejection" bullet in `### Verdict` and the entire `## Brooder` section byte-for-byte unchanged (PLM-018 FC-7, FC-8) — these are verified, not rewritten.

No Go code changes; no adapter/manifest changes; no rebuild/refresh (that's FTHR-B).

## Tests
New file `internal/bootstrap/worker_protocols_test.go`, package `bootstrap`, reading `core/skills/fledge-orchestrate/worker-protocols.md` via the package's embedded `FS` (same pattern as `TestCorePrimitivesReferenced`/`TestCoreNeutral` in `registry_test.go`):

- `TestSkuaConcessionHardened` — asserts the old lenient sentence ("If a brooder pushes back on a finding with a fact verified to be correct, withdraw the finding") is **absent**, and that replacement language is **present**: the disproof must be independently checkable and the skua "re-verifies" it before withdrawing, and a bare/unverified counter-assertion "never withdraws a finding".
- `TestSkuaEvidenceGuiltyUntilProven` — asserts the `### Reviewing a feather` criteria-audit item states ambiguous/incomplete/terse-log evidence "is NOT proof" (or equivalent asserted phrase) and that the box is left unchecked with a finding filed, plus the "where cheap" carve-out is present alongside the new "must be sufficient to independently confirm" requirement.
- `TestSkuaRedTeamPass` — asserts a "Red-team pass" (or equivalent named) checklist item exists in `### Reviewing a feather`, that it appears after the "Diff vs. spec" item and before the item(s) covering "Scope and simplicity" (via `strings.Index` ordering on the section text), and that its text says findings are reported (never fixed/committed by the skua).
- `TestSkuaUnchangedInvariants` — asserts the "Third rejection" sentence ("if a feather fails review 3 times, do NOT start a fourth cycle") is present verbatim, and a stable sentence from `## Brooder`'s fix-loop rule ("Do not argue a finding with the skua past one round of clarification") is present verbatim — guarding FC-7/FC-8 against incidental changes.

Implementation order: write all four tests first, run `go test ./internal/bootstrap -run TestSkua`, capture them **failing** against the unmodified file (the "absent" assertions in `TestSkuaConcessionHardened` will actually pass, but the "present" assertions in all four will fail — expected reason: new language doesn't exist yet), then rewrite the section per Approach until all four pass.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation (new-language assertions) and pass after.
- [ ] AC-2: `### Reviewing a feather`'s criteria-audit item states the guilty-until-proven default (satisfies PLM-018 FC-3, FC-4) — `TestSkuaEvidenceGuiltyUntilProven` passes.
- [ ] AC-3: `### Reviewing a feather` has a red-team checklist item positioned after "Diff vs. spec" and before "Scope and simplicity," running every cycle, findings-only (satisfies PLM-018 FC-5, FC-6) — `TestSkuaRedTeamPass` passes.
- [ ] AC-4: `### Verdict`'s concession paragraph requires independently re-verified disproof to withdraw a finding (satisfies PLM-018 FC-1, FC-2) — `TestSkuaConcessionHardened` passes.
- [ ] AC-5: The 3-rejection sentence and the cited `## Brooder` sentence are unchanged verbatim (satisfies PLM-018 FC-7, FC-8) — `TestSkuaUnchangedInvariants` passes.
- [ ] AC-6: `go vet ./...` and `go test ./internal/bootstrap/...` pass with the new tests included.
