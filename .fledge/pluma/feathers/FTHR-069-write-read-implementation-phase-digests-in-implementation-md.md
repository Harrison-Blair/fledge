---
id: FTHR-069
title: Write/read implementation-phase digests in implementation.md
plumage: PLM-029
status: egg
priority: P2
depends_on: [FTHR-058]
authored: 2026-07-16T16:38:10Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-069: Write/read implementation-phase digests in implementation.md

## Description
Add digest write/read instructions to `implementation.md`: step 5 ("End of run") writes `digest-implementation.md`, and step 1 ("Resolve scope") best-effort-reads a prior `digest-planning.md` if present (satisfies PLM-029 FC-1, FC-2, FC-3, FC-4, FC-5(implementation half), AC-1).

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` — step 1 ("Resolve scope", line 31) and step 5 ("End of run", line 118). Per `.fledge/nest/modules.md` → internal-bootstrap-core.

## Approach
- **Step 5 (End of run):** after the existing report bullets (feathers completed vs. blocked, merges performed, newly-unblocked feathers), add: write `.fledge/scratch/digest-implementation.md` (overwriting any prior one) containing — the outcome (which feathers merged, current suite status on main), key decisions made during escalation triage (step 4) if any, and pointers to the merged feathers' spec files.
- **Step 1 (Resolve scope):** add a note that if `.fledge/scratch/digest-planning.md` exists, read it as grounding context (best-effort — a missing file means proceed without it) before resolving which feathers are in scope for this run.

## Tests
- `TestImplementationDocDescribesDigestWrite` (new, `internal/bootstrap`): reads embedded `implementation.md`, asserts step 5's prose mentions writing `digest-implementation.md`.
- `TestImplementationDocDescribesDigestRead` (new, `internal/bootstrap`): asserts step 1's prose mentions best-effort reading `digest-planning.md` if present.
- Implementation order: write both tests against `implementation.md` as FTHR-058 leaves it (fail — no digest language yet), add the two additions, confirm both pass.

## Acceptance Criteria
- [ ] AC-1: Both tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: `implementation.md` step 5 documents writing `digest-implementation.md` with the FC-3 content shape (satisfies PLM-029 AC-1).
- [ ] AC-3: `implementation.md` step 1 documents best-effort reading `digest-planning.md` if present (satisfies PLM-029 AC-2).
- [ ] AC-4: `go test ./internal/bootstrap/...` passes.
