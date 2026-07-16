---
id: FTHR-067
title: Write/read planning-phase digests in planning.md
plumage: PLM-029
status: egg
priority: P2
depends_on: [FTHR-065]
authored: 2026-07-16T16:38:10Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-067: Write/read planning-phase digests in planning.md

## Description
Add digest write/read instructions to `planning.md`: its phase-close step (4.7) writes `digest-planning.md`, and its opening (step 1) best-effort-reads a prior `digest-implementation.md` if present (satisfies PLM-029 FC-1, FC-2, FC-3, FC-4, FC-5(planning half), AC-1).

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/planning.md` — step 1 (freshness gate, the phase's opening) and step 4.7 (closing report). Per `.fledge/nest/modules.md` → internal-bootstrap-core.

## Approach
- **Step 4.7 (closing report):** after listing created files/dependency waves/ready set/remaining slate, add: write `.fledge/scratch/digest-planning.md` (overwriting any prior one) containing — the phase's outcome (hatched plumage/feather IDs and file paths), the key user decisions made during interrogation (not the full Q&A — just resolved decisions and rationale), and pointers to the created spec files. This is part of what becomes the `PHASE-CLOSE` message when delegated (worker-protocols.md's incubator relay envelope already covers what to *say*; this adds what to *write to disk* before saying it).
- **Step 1 (freshness gate / phase opening):** add a note that if `.fledge/scratch/digest-implementation.md` exists, read it as grounding context (best-effort — a missing file, e.g. on a repo's first-ever planning phase, means proceed without it) before continuing.

## Tests
- `TestPlanningDocDescribesDigestWrite` (new, `internal/bootstrap`): reads embedded `planning.md`, asserts step 4.7's prose mentions writing `digest-planning.md` with outcome/decisions/spec-pointer content.
- `TestPlanningDocDescribesDigestRead` (new, `internal/bootstrap`): asserts step 1's prose mentions best-effort reading `digest-implementation.md` if present.
- Implementation order: write both tests against `planning.md` as FTHR-065 leaves it (fail — no digest language yet), add the two additions, confirm both pass.

## Acceptance Criteria
- [x] AC-1: Both tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `planning.md` step 4.7 documents writing `digest-planning.md` with the FC-3 content shape (satisfies PLM-029 AC-1).
- [x] AC-3: `planning.md` step 1 documents best-effort reading `digest-implementation.md` if present (satisfies PLM-029 AC-2).
- [x] AC-4: `go test ./internal/bootstrap/...` passes.
