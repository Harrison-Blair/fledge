---
id: FTHR-003
title: "fledge unfledged command: list incomplete plumage and feathers with type filters"
plumage: PLM-002
status: fledged
priority: P2
depends_on: []
oversight: merge
authored: 2026-07-07T21:21:05Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.0
---

# FTHR-003: fledge unfledged command: list incomplete plumage and feathers with type filters

## Description
The whole of PLM-002 in one command: a working `fledge unfledged`, end to end. It lists every non-complete plumage and every non-complete feather (`status != fledged`) in two flat sections (FC-1, FC-2), ordered priority-then-ID (FC-3), status-only with no readiness computation (FC-4), scoped by `--plumage`/`--feathers` (FC-5), in both human text and `--json` from one computed value (FC-6). Unparseable spec files are surfaced in an issues section without aborting, exit 0 (FC-7); empty repos yield a valid empty report (FC-8); exit codes follow the 0/2/3 taxonomy with no exit-1 paths (FC-9). This feather does NOT touch any agent-facing docs — wiring `fledge unfledged` into the orchestration skill is FTHR-B's job.

## Affected Modules
- `internal/cli` — new file `internal/cli/unfledged.go`, plus adding `"unfledged"` to `commandOrder` in `internal/cli/cli.go` (see `.fledge/nest/modules.md` → internal/cli; `.fledge/nest/conventions.md` → one-command-per-file, dual output).
- `cmd/fledge` — new e2e suite `cmd/fledge/testdata/unfledged.txtar` (see `.fledge/nest/testing.md`).

## Approach
Follow the existing command pattern (models: `internal/cli/ready.go` for filtering/sorting/dual-output, `internal/cli/colony.go` for the compute-once report struct and `set.Errors` issues handling):
- `init() { register("unfledged", runUnfledged, "fledge unfledged [--plumage] [--feathers] [--json]") }`; add `"unfledged"` to `commandOrder` in `cli.go` (pins FC-9 usage listing + AC-4).
- Parse flags with `flag.ContinueOnError` (unknown flag → `ExitUsage`, pins FC-9 usage). Two bools `--plumage`/`--feathers`; the section-selection rule is `showPlumage := *plum || !*feath` and `showFeathers := *feath || !*plum` — so neither-or-both shows both (FC-5).
- Reuse `loadSet()` (`internal/cli/specload.go`) for repo detection + spec loading; its env/exit handling gives FC-9 (exit 3 outside a repo) for free. Like `colony` and UNLIKE `ready`, do NOT refuse on check errors — `unfledged` is an observer (PLM-002 FC-7).
- Define a compute-once report struct with JSON tags, rendered to text or `emitJSON` from the same value (FC-6). Shape, e.g.:
  - `type unfledgedItem struct { ID, Status, Priority, Title string; Plumage string `json:"plumage,omitempty"`; Oversight string `json:"oversight,omitempty"`; Path string `json:"path"` }`
  - `type unfledgedReport struct { Plumage []unfledgedItem `json:"plumage"`; Feathers []unfledgedItem `json:"feathers"`; Issues []string `json:"issues"` }` (nil slices → `[]` before emit, per `ready`'s convention).
- Populate: iterate `set.Reqs`, keep `r.Status != spec.ReqFledged` → Plumage items (no `Plumage` field); iterate `set.Tasks`, keep `t.Status != spec.TaskFledged` → Feathers items (set `Plumage`, `Oversight`). `Path` via `relPath(r.Root, …)`. Issues from `set.Errors` as `relPath: err` strings (matching colony's parse-error formatting), sorted.
- Sort each section by priority then ID, identical to `ready.go` (priority string compare, ID tiebreak).
- Text render: `Plumage` heading then lines `ID  status  priority  title`; `Feathers` heading then `ID  status  priority  title  (plumage PLM-###)`; only emit a section when selected by the flags. An `Issues:` section prints only when non-empty. Empty selected sections print a `(none)` placeholder line (FC-8).

## Tests
`cmd/fledge/testdata/unfledged.txtar`, testscript-driven like `ready.txtar`/colony's suite. Implementation order is fixed: (1) write the txtar; (2) run `go test ./cmd/fledge -run TestScript/unfledged` against unchanged code and confirm it FAILS with `unknown command "unfledged"`; (3) implement until green.
- populated repo: plumages and feathers in mixed statuses (incl. at least one `fledged` of each) → asserts both sections list only non-fledged items, fledged items absent, correct fields incl. a feather's `(plumage PLM-###)`, and priority-then-ID ordering (pins FC-1, FC-2, FC-3).
- `--plumage` → only the Plumage section present; `--feathers` → only the Feathers section present; both flags together → both sections (pins FC-5).
- `--json` → asserts the document shape: `plumage`/`feathers` arrays with `id/status/priority/title/path`, feather entries carrying `plumage`, and `[]` (not null) for an empty section (pins FC-6).
- degraded repo: an unparseable spec file present → the good specs still list and the bad file appears under issues, exit 0 (pins FC-7).
- empty repo (`.fledge/` present, no specs) → valid report with empty sections, exit 0 (pins FC-8).
- usage error (`fledge unfledged --bogus`) → exit 2; outside a fledge repo → exit 3 (pins FC-9).

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation (unknown-command failure captured) and pass after.
- [x] AC-2: `fledge unfledged` (text and `--json`) satisfies PLM-002 FC-1–FC-9 as pinned by the txtar assertions, including `--plumage`/`--feathers` scoping and the neither-or-both rule.
- [x] AC-3: `go test ./...` green; `go vet ./...` clean; full suite unaffected.
- [x] AC-4: `fledge unfledged` appears in the usage listing (`commandOrder`) and running it in this repo lists PLM-002 and its open feathers accurately (spot-checked against `fledge colony`).
