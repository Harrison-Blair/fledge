---
id: PLM-018
title: Adversarial skua review
status: fledged
priority: P2
authored: 2026-07-15T18:37:27Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.4
---

# PLM-018: Adversarial skua review

## Context
The fledge implementation loop pairs each brooder with a skua that reviews its work before merge (`worker-protocols.md` § Skua). Today the skua's review posture is too easy to talk out of a real finding: it withdraws a finding whenever the brooder "pushes back... with a fact verified to be correct" (no requirement that the skua itself re-check that fact), it treats claims as innocent-until-shown-otherwise rather than requiring affirmative proof, and it never actively hunts for edge cases the spec's Tests section fails to pin — it only audits what the brooder already claims to have covered. This lets a confident but wrong brooder argue its way past review, and lets gaps the spec-author never thought to name slip through untested. This plumage hardens the skua's review posture on all three axes so that findings only die on evidence the skua has personally verified, evidence is guilty until proven, and the skua actively tries to break the feather rather than passively checking boxes. The brooder's role and behavior are explicitly unchanged — it stays a pure implementer; only the skua's review protocol changes.

## User Stories
- As the fledge maintainer, I want the skua to hold a finding until the brooder's disproof is independently re-verified, so that a brooder can't argue its way past a real bug with a confident but unverified counter-claim.
- As the fledge maintainer, I want ambiguous or incomplete AC evidence treated as insufficient by default, so that a feather can't merge on the benefit of the doubt.
- As the fledge maintainer, I want the skua to actively probe for edge cases the spec's Tests section doesn't name, so that gaps the spec-author never thought to test still get caught before merge.

## Functional Criteria
1. FC-1: A brooder's pushback on a finding withdraws that finding only when the brooder supplies concrete, independently checkable disproof (a specific test run, line reference, or spec citation that directly contradicts the finding) **and** the skua itself re-verifies that disproof (re-runs the cited command, reads the cited line/spec text) before withdrawing. A bare counter-assertion, re-explanation, or unverified "that's intentional" never withdraws a finding.
2. FC-2: When a brooder's pushback does not meet FC-1's bar, the finding either stands (skua still holds it after checking) or, if it is a genuine judgment call unresolved after one round, escalates to the orchestrator — never a second silent revise cycle on an unverified claim.
3. FC-3: An `## AC-N` evidence-file section that is ambiguous, incomplete, or backed only by a terse/summarized log (no visible assertions, diffs, or output substantiating the claim) is treated as NOT proof — the skua leaves that criterion's box unchecked and files a finding, regardless of whether re-running the command would be cheap.
4. FC-4: For any command the skua chooses not to re-run ("where cheap" audits, unchanged), the recorded evidence-file output must itself be sufficient to independently confirm the claim; otherwise FC-3 applies.
5. FC-5: Every review cycle (not only the first), the skua performs an explicit red-team pass: reads the implementation for branches/inputs the spec's named tests never exercise, and runs throwaway, never-committed probes (ad hoc invocations with uncovered inputs, or a scratch test file outside the tracked worktree) to surface edge cases the Tests section fails to pin. Any gap found is reported as a numbered finding (a missing case), never fixed or written as a real test by the skua itself.
6. FC-6: The skua's hard constraints are otherwise unchanged: it never modifies tracked/committed code, never merges, and its only permitted write remains checking/unchecking AC boxes via `fledge criteria check|uncheck`. Red-team probes under FC-5 are throwaway and never committed to the branch.
7. FC-7: The 3-rejection escalation threshold and its counting are unchanged — 3 review-cycle rejections (from any combination of standard findings and FC-5 red-team findings) trigger escalation to the orchestrator; the skua does not start a 4th cycle on its own.
8. FC-8: The brooder's protocol and behavior are unchanged by this plumage — it remains a pure implementer with the same "do not argue past one round" rule on its side of the fix loop.

## Acceptance Criteria
- [x] AC-1: `worker-protocols.md` § Skua § Verdict states the hardened concession rule (FC-1, FC-2): a finding withdraws only on independently re-verified disproof, never on a bare counter-assertion.
- [x] AC-2: `worker-protocols.md` § Skua's evidence-audit check states the guilty-until-proven default (FC-3, FC-4): ambiguous/incomplete/terse evidence is not proof and leaves the box unchecked with a finding filed.
- [x] AC-3: `worker-protocols.md` § Skua § Reviewing a feather gains an explicit red-team checklist item (FC-5, FC-6) that runs every cycle and produces findings only, never fixes.
- [x] AC-4: The 3-rejection escalation rule (FC-7) and the brooder's protocol (FC-8) are verified unchanged (or, if any incidental wording needed adjustment for consistency with the above, it stays a wording-only change with no behavioral shift).
- [x] AC-5: `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` (source of truth) and its regenerated copy at `.fledge/skills/fledge-orchestrate/worker-protocols.md` are both updated and identical, via rebuild (`go install ./cmd/fledge`) + `fledge init --refresh`, with `.fledge/scaffold.json` reflecting the new content hash.
- [x] AC-6: `cmd/fledge/testdata/*.txtar` fixtures (`init.txtar`, `init_agents.txtar`, `agents.txtar`) and any other assertions on the changed prose are confirmed passing (updated if the new wording trips an existing assertion) — `go test ./...` is green.

## Out of Scope
- Any change to the brooder's protocol, behavior, or hard constraints (it remains a pure implementer; FC-8).
- Any change to review checks unrelated to concession strictness, evidence sufficiency, or red-teaming (e.g. the existing "tests pass now," "diff vs. spec," "scope and simplicity" checks are unchanged except where FC-3/FC-4 tighten the evidence-audit check specifically).
- Any change to the 3-rejection threshold count or its escalation mechanics (FC-7).
- Any change to `fledge-skua.md`'s one-line `description` frontmatter — left to feather-level judgment; it currently just summarizes the role and delegates to `worker-protocols.md`, so it likely needs no edit, but this is not decided at the plumage level.
- Tmux/team-loop auto-defaulting — that is PLM-(sibling), a separate plumage.

## Open Questions
None — all interrogation branches resolved with the user.
