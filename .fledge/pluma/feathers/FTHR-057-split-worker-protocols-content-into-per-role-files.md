---
id: FTHR-057
title: Split worker-protocols content into per-role files
plumage: PLM-027
status: fledged
priority: P1
depends_on: []
authored: 2026-07-16T16:15:06Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-057: Split worker-protocols content into per-role files

## Description
Split `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` into `incubator.md`, `brooder.md`, and `skua.md` (verbatim content moves, no wording changes), shrink `worker-protocols.md` to a stub index, and replace the Go test file that asserts on the old combined structure with three tests matching the new files. This is the root feather (PLM-027 FC-1, FC-2, FC-3, FC-5): everything else in this plumage depends on the new files existing at their final paths. Bundling the test replacement into this same feather (rather than a separate one) is deliberate — it keeps `go test ./...` green at every merge point instead of leaving a window where main is red between "docs split" and "tests updated" landing separately.

## Affected Modules
- `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` (internal-bootstrap-core, per `.fledge/nest/modules.md`) — shrinks to stub.
- `internal/bootstrap/core/skills/fledge-orchestrate/incubator.md`, `brooder.md`, `skua.md` — new files.
- `internal/bootstrap/worker_protocols_test.go` — deleted.
- `internal/bootstrap/incubator_test.go`, `brooder_test.go`, `skua_test.go` — new files.

Note: this feather touches only `internal/bootstrap/core/...` (the embedded source of truth) and its test, not this repo's own scaffolded `.fledge/skills/fledge-orchestrate/*.md` copies — those are refreshed later, in FTHR-061 (`fledge init --refresh`), per `CLAUDE.md`'s "change source, then --refresh" convention.

## Approach
1. In the current `worker-protocols.md`: lines 1-6 are the shared spawn-prompt-contract intro (what a spawn prompt is, its fixed fields) — this stays, verbatim, in the stub. Lines 7-40 are the `## Incubator` section (through its `Relay envelope`/`Communication rules`/`Drafting`/`Lifecycle` subsections) — move verbatim into `incubator.md` as a new `# Incubator` H1 document (demote the section heading to a document title, keep every subsection heading level as-is beneath it). Lines 42-72 (`## Brooder` through its subsections) move verbatim into `brooder.md` as `# Brooder`. Lines 74-110 (`## Skua` through its subsections, including `### Reviewing a feather` and `### Verdict`) move verbatim into `skua.md` as `# Skua`.
2. Rewrite `worker-protocols.md` to keep only: the intro paragraph (adjusted only to describe the three files instead of sections — no protocol content changes) and a short links list to `incubator.md`, `brooder.md`, `skua.md` (one line each, naming what each covers, since a reader landing on the stub needs to pick the right file without opening all three).
3. Delete `internal/bootstrap/worker_protocols_test.go`. Create `incubator_test.go`, `brooder_test.go`, `skua_test.go`, each reading its own embedded file directly (`FS.ReadFile("core/skills/fledge-orchestrate/incubator.md")` etc. — no more section-extraction-by-string-search, since the whole file IS the section now). Preserve the same assertions the old tests made (e.g. the `skuaSection`/`reviewingSection`/`verdictSection` helpers' underlying checks — subsection presence, key phrases) against the new files, rewritten as direct content checks rather than extracted-substring checks.

## Tests
- `TestIncubatorDocSections` (`incubator_test.go`): asserts `incubator.md` exists, starts with `# Incubator`, and contains its four subsection headings (`Relay envelope`, `Communication rules`, `Drafting`, `Lifecycle`).
- `TestBrooderDocSections` (`brooder_test.go`): asserts `brooder.md` starts with `# Brooder` and contains `Communication rules`, `Protocol`, `When stuck`, `Lifecycle`.
- `TestSkuaDocSections` (`skua_test.go`): asserts `skua.md` starts with `# Skua` and contains `Communication rules`, `Reviewing a feather`, `Verdict`, `Lifecycle`, preserving the old tests' key-phrase checks (e.g. the third-rejection and pass-signal phrasing) now read directly from `skua.md`.
- `TestWorkerProtocolsStub` (in whichever of the three files is simplest to extend, or a small new `worker_protocols_stub_test.go`): asserts `worker-protocols.md` no longer contains `## Incubator`/`## Brooder`/`## Skua` headings and does link to all three new files.
- Implementation order: write these four tests against the *unchanged* repo first — they fail (missing files / old structure) for the expected reason — then perform the split and confirm they pass.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: `incubator.md`, `brooder.md`, `skua.md` exist under `internal/bootstrap/core/skills/fledge-orchestrate/`, and a diff of each against the corresponding pre-split section of `worker-protocols.md` shows no content changes beyond the heading-level demotion (satisfies PLM-027 AC-1, FC-2).
- [x] AC-3: `worker-protocols.md` contains only the intro paragraph and links to the three new files — no `## Incubator`/`## Brooder`/`## Skua` headings remain (satisfies PLM-027 AC-2, FC-3).
- [x] AC-4: `internal/bootstrap/worker_protocols_test.go` no longer exists; `incubator_test.go`/`brooder_test.go`/`skua_test.go` exist and pass (satisfies PLM-027 AC-4, FC-5).
- [x] AC-5: `go test ./internal/bootstrap/...` passes.
