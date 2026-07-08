---
id: FTHR-006
title: "nest scaffold, scout, and stamp verbs"
plumage: PLM-003
status: egg
priority: P2
depends_on: [FTHR-005]
authored: 2026-07-08T01:52:52Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-006: nest scaffold, scout, and stamp verbs

## Description
Widen the `fledge nest` command from the tracer's single `new` verb to the full four-verb
surface by adding `scaffold`, `scout`, and `stamp` on the spine FTHR-005 established (the
`internal/nest` schemas, renderer, registry, and embedded templates already exist). No new
package; this feather is `internal/cli/nest.go` logic + `internal/nest` helpers + txtar.

## Affected Modules
- **`internal/cli/nest.go`** — three new dispatch branches. See `.fledge/nest/conventions.md`
  → Command dispatch.
- **`internal/nest`** — small additions: a nest-clear helper for `scaffold`, and a
  stamp/refresh helper that parses an existing file, rewrites derived frontmatter, and
  preserves the body (reuses `spec.SplitFrontmatter`). See `.fledge/nest/modules.md` →
  internal/spec for the reused primitives.
- **`cmd/fledge/testdata/nest.txtar`** — extended with cases for the three verbs.

## Approach
- **`nest scaffold [--agent] [--json]`**: remove `.fledge/nest/*.md` and everything under
  `.fledge/nest/raw/` (recreate the dirs), then create all nine concern docs (the FTHR-005
  `new` path, looped over `ConcernDocs`) with fresh stamped frontmatter. Overwrites by
  default (no `--force` needed — it owns the clean slate). `--agent` default `fledge-forager`.
  Emits the created path list (`{"created":[...]}` / one `created <rel>` line each).
- **`nest scout --module <m> [--agent] [--force] [--json]`**: build a `Scout`-kind `Doc`
  (`Module=m`, `Authored=now`, `Agent=--agent|default fledge-context-scout`, VERSION),
  `Body=` embedded scout-report template; write `.fledge/nest/raw/<m>.md` with `O_EXCL`
  unless `--force`. Missing `--module` → `usageErr` (exit 2).
- **`nest stamp <file> [--kind concern|scout] [--agent] [--json]`**: resolve `<file>`,
  reject a path outside `.fledge/nest/` (`usageErr`). Detect kind by path (`raw/` → scout,
  else concern) unless `--kind` overrides. `spec.SplitFrontmatter` the file; parse the
  existing frontmatter to recover `agent`/`module`; rebuild a `Doc` with refreshed derived
  fields (`Generated`/`Authored`=now, `Commit`=`r.Head()`, `FledgeVersion`=`r.Version`),
  preserved `Agent` (or `--agent` override) and `Module`, dropping any unknown keys; write
  `Render()` (canonical FM + original body bytes) via `spec.WriteFileAtomic`. Text prints
  `stamped <rel>`.

## Tests
Test-first (write → observe FAIL → implement):
- **`nest.txtar`** additions:
  - `scaffold` into a nest pre-seeded with a stale file + a stray `raw/x.md`: asserts the
    stale/stray files are gone, all nine concern docs exist with stamped frontmatter.
  - `scout --module cli` creates `raw/cli.md` with scout-schema frontmatter + template body;
    missing `--module` → exit 2; existing report without `--force` → exit 1.
  - `stamp` on a doc with an outdated `commit`/`fledge_version` and an extra unknown key:
    asserts derived fields refreshed, unknown key dropped, body bytes unchanged, `agent`
    preserved; `--agent` overrides; a path outside `.fledge/nest/` → exit 2; a `raw/*.md`
    stamps with the scout schema (kind-by-path).
- **`internal/nest/nest_test.go`**: `TestStampPreservesBodyAndDropsUnknownKeys` on the
  stamp/refresh helper directly.

## Acceptance Criteria
- [x] AC-1: The tests listed above were observed failing before implementation and pass
  after.
- [x] AC-2: `nest scaffold` clears the whole nest (incl. `raw/`) and recreates nine docs with
  stamped frontmatter, overwriting by default (satisfies PLM-003 FC-3).
- [x] AC-3: `nest scout --module <m>` creates `raw/<m>.md` from the scout template with the
  scout schema; refuses overwrite without `--force`; missing `--module` is a usage error
  (satisfies PLM-003 FC-5).
- [x] AC-4: `nest stamp <file>` refreshes derived fields, preserves `agent`/`module` and the
  body byte-for-byte, drops unknown keys, detects kind by path with `--kind` override, and
  rejects out-of-nest paths (satisfies PLM-003 FC-6).
- [x] AC-5: All verbs honor `--json` and the exit-code taxonomy; `go test ./...` and
  `go vet ./...` pass (satisfies PLM-003 FC-8).
