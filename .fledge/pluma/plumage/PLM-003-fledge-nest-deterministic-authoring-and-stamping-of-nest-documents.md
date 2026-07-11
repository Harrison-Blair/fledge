---
id: PLM-003
title: "fledge nest: deterministic authoring and stamping of nest documents"
status: fledged
priority: P2
authored: 2026-07-08T01:26:58Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-003: fledge nest: deterministic authoring and stamping of nest documents

## Context
The forager and its scouts produce the `.fledge/nest/` document set — eight concern docs
plus `index.md` (forager), and `raw/<module>.md` reports (scouts). Today every one of those
files' frontmatter is hand-assembled by an agent shelling out to `date -u`, `git rev-parse
HEAD`, and reading `VERSION`, then hand-writing YAML. That is exactly the nondeterministic,
drift-prone busywork the rest of fledge refuses to tolerate: spec frontmatter is CLI-owned
and never hand-edited (`internal/spec/frontmatter.go` renders a canonical fixed-key block),
yet nest frontmatter — structurally identical in spirit — is hand-built. The cost is real:
the nest docs were recently found 13 commits stale with `fledge_version: 0.1.0` and a
retired `agent:` value, because refreshing them is a manual chore.

This plumage gives the nest the same deterministic substrate specs already have: a `fledge
nest` command that creates nest documents (with correct frontmatter + a titled body stub)
and stamps/refreshes an existing nest doc's frontmatter canonically — preserving the
markdown body byte-for-byte. The CLI becomes the single source of truth for the nest's two
frontmatter schemas and its body templates; the foraging protocol and agent prose migrate
from "hand-write this YAML" to "run `fledge nest …`."

## User Stories
- As a forager, I want one command that scaffolds the whole concern-doc set with correct,
  fresh frontmatter, so that I stop hand-writing YAML and hand-deleting stale docs.
- As a scout, I want to create my `raw/<module>.md` report from the canonical template with
  correct frontmatter in one command, so that every scout report is structurally identical.
- As an orchestrator gathering context in-session (no `spawn-worker`), I want the same
  create/stamp commands, so that Tier-A/B harnesses produce byte-identical nest output.
- As a maintainer, I want a `stamp` command that deterministically refreshes a nest doc's
  `generated`/`commit`/`fledge_version` while preserving its prose, so that keeping context
  fresh is a mechanical, reviewable operation.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: A single `fledge nest` command dispatches four sub-verbs on a positional first arg
   (mirroring `fledge new plumage|feather`): `scaffold`, `new <doc>`, `scout`, `stamp`. It
   registers once in `commandOrder` and its Bash allow-list entry is generated.
2. FC-2: The CLI owns the nest's two canonical frontmatter schemas, rendered fixed-key like
   spec frontmatter — concern docs: `generated / commit / agent / fledge_version`; scout
   reports: `module / authored / agent / fledge_version`.
3. FC-3: `fledge nest scaffold` clears the whole nest (`.fledge/nest/*.md` and
   `.fledge/nest/raw/*`) and recreates the eight concern docs + `index.md`, each with
   correct frontmatter and a generic titled body stub. It overwrites by default. Default
   `agent: fledge-forager`, overridable via `--agent`.
4. FC-4: `fledge nest new <doc>` creates one concern doc from the closed known set (the
   eight concern names + `index`); an unknown `<doc>` is a usage error. It refuses to
   overwrite an existing file without `--force`. Default `agent: fledge-forager`.
5. FC-5: `fledge nest scout --module <m>` creates one `raw/<m>.md` from the canonical
   scout-report template with correct frontmatter. It refuses to overwrite without
   `--force`. Default `agent: fledge-context-scout`. Missing `--module` is a usage error.
6. FC-6: `fledge nest stamp <file>` rewrites the file's frontmatter canonically — refreshing
   the derived fields (`generated`/`authored` → now UTC, `commit` → git HEAD,
   `fledge_version` → repo VERSION), preserving `agent`/`module`, dropping unknown keys, and
   preserving the markdown body byte-for-byte. Doc kind is detected by path (`raw/` → scout
   report, else concern doc), overridable with `--kind`. An optional `--agent` sets a
   corrected agent value. A path outside `.fledge/nest/` is a usage error.
7. FC-7: All body templates (generic concern-doc stub, `index.md` stub, scout-report body)
   are embedded in the Go binary as the single source of truth. The skill's
   `templates/scout-report.md` and the frontmatter blocks in `foraging.md` /
   `context-doc.md` are reduced to pointers at the CLI.
8. FC-8: Every verb supports `--json` (emitting created/stamped path(s)) and text output
   (`created <rel>` / `stamped <rel>`), consistent with existing commands. Exit codes follow
   the taxonomy: 0 success, 2 usage error, 3 not a fledge repo (`RequireFledge`), 1 for
   I/O/git failure.
9. FC-9: The foraging protocol and forager/scout agent prose are migrated to call `fledge
   nest …` instead of hand-writing frontmatter or hand-deleting the nest, keeping the
   workflow self-consistent with the new commands.

## Acceptance Criteria
Checkbox list of verifiable conditions under which this plumage is considered fledged, one `- [ ] AC-N: …` line each. Authored unchecked; checked only via `fledge criteria check` at plumage closeout.
- [x] AC-1: Tests written first and observed FAILING against unchanged code for the expected
  reason, then passing after implementation (per-repo test-first convention).
- [x] AC-2: A txtar e2e suite covers each verb: `scaffold` (nine files created, correct
  frontmatter, prior nest cleared), `new <doc>` (known-name success, unknown-name usage
  error, `--force` overwrite behavior), `scout --module` (report created, missing-module
  usage error), and `stamp` (derived fields refreshed, body byte-preserved, unknown keys
  dropped, out-of-nest path rejected, `raw/` vs concern kind detection). `--json` shapes and
  exit codes asserted.
- [x] AC-3: Go unit tests for the `internal/nest` canonical frontmatter renderer prove
  fixed-key order, canonical scalar quoting, and round-trip body preservation.
- [x] AC-4: The `init`/`agents` txtar fixtures and generated Claude allow-list are updated
  for the new `nest` command; `go test ./...` and `go vet ./...` pass.
- [x] AC-5: Running the commands in this repo reproduces the current nest output form
  (frontmatter byte-identical to what the forager now hand-writes), human-verified.
- [x] AC-6: `fledge preen` reports no findings for the spec set after authoring.

## Out of Scope
- Nest *validation* (a `preen`-style rule checking nest completeness or frontmatter
  freshness) — the CLI knowing the nest contract enables it, but it's a separate plumage.
- Free-form concern-doc names — `new <doc>` is restricted to the closed known set.
- Changing the *content/shape* of the eight concern docs or the scout-report sections — only
  their creation and frontmatter become CLI-owned; the body stubs stay generic.
- Auto-running `stamp` or `scaffold` from other commands (e.g. a git hook) — invocation
  stays explicit.
- Regenerating context (the forager pipeline itself) — unchanged except for which primitive
  writes the files.

## Open Questions
None — command surface resolved via interrogation (2026-07-08).
