---
id: FTHR-068
title: Write foraging-phase digests in foraging.md
plumage: PLM-029
status: egg
priority: P2
depends_on: [FTHR-058]
authored: 2026-07-16T16:38:10Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-068: Write foraging-phase digests in foraging.md

## Description
Add a digest-writing instruction to `foraging.md`'s close-out step ("On the final message, verify and release", ~line 25): write `digest-foraging.md` as part of confirming/relaying the forager's coverage summary (satisfies PLM-029 FC-1, FC-2, FC-3, FC-4, FC-5(foraging half), AC-1).

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/foraging.md` — the "**On the final message, verify and release.**" paragraph (~line 25). Per `.fledge/nest/modules.md` → internal-bootstrap-core.

## Approach
Immediately after "...confirm the result with `fledge nest status`... relay the forager's coverage notes, and request the forager's graceful shutdown by name", add: before or alongside relaying, write `.fledge/scratch/digest-foraging.md` (overwriting any prior one) with the coverage outcome (which concern docs were written/updated, commit stamped), any open questions the forager flagged, and pointers to `.fledge/nest/index.md`. This is written by whoever is acting as commissioner (orchestrator or incubator) at that point, not the forager itself — the forager's job ends at its final message; the commissioner does the digest write as part of verify-and-release.

## Tests
- `TestForagingDocDescribesDigestWrite` (new, `internal/bootstrap`): reads embedded `foraging.md`, asserts the verify-and-release paragraph mentions writing `digest-foraging.md`.
- Implementation order: write the test against `foraging.md` as FTHR-058 leaves it (fails — no digest language yet), add the instruction, confirm it passes.

## Acceptance Criteria
- [ ] AC-1: The test listed above was observed failing before implementation and passes after.
- [ ] AC-2: `foraging.md`'s verify-and-release step documents writing `digest-foraging.md`, attributed to the commissioner, with the FC-3 content shape (satisfies PLM-029 AC-1).
- [ ] AC-3: `go test ./internal/bootstrap/...` passes.
