---
id: FTHR-009
title: "Scaffold stamp module + init stamping (tracer)"
plumage: PLM-009
status: pipping
priority: P1
depends_on: []
authored: 2026-07-10T14:57:51Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-009: Scaffold stamp module + init stamping (tracer)

## Description
The thin end-to-end tracer for PLM-009: introduce the scaffold stamp —
`internal/bootstrap/stamp.go` with the `Stamp`/`StampEntry` types, deterministic
serialization, `LoadStamp`/`Write`, a `renderEntry` helper factored out of
`writeFileEntry`, and `ExpectedFiles` (the rendered path→{policy, bytes|target|lines}
map shared later by preen and refresh) — and have `fledge init` write
`.fledge/scaffold.json` as its last act. After this feather a real `fledge init`
produces a git-trackable, byte-idempotent stamp recording the fledge version and a
sha256 manifest of every file it scaffolded. Later feathers consume the stamp:
FTHR-010 (warning), FTHR-011 (preen drift), FTHR-012 (refresh preserve/prune).

## Affected Modules
- **`internal/bootstrap`** (new `stamp.go`, edit `registry.go`) — Stamp types;
  hash = sha256 hex of rendered bytes as shipped (templates hashed post-render);
  symlink entries record `target`, append entries record `lines`; the stamp
  excludes itself. `renderEntry(m, f, ctx) ([]byte, error)` factored from the
  template-render body of `writeFileEntry` (registry.go ~412–425) and reused by
  both the write path and `ExpectedFiles`. See `.fledge/nest/architecture.md`
  → bootstrap layer; `.fledge/nest/conventions.md` → byte-idempotent writes.
- **`internal/cli/init.go`** — a helper assembles the base-scaffold extras
  (`baseFiles`, `.fledgeignore` bytes, `.gitignore` append lines) into
  ExpectedFiles input so init (and later preen) compute the identical set;
  `runInit` builds the stamp from the expected tree after all writes succeed and
  writes it via `writeIfChanged`, reported like any other file. Agents recorded =
  resolved agents ∪ old stamp's agents.
- **`cmd/fledge/testdata/`** — `init.txtar`, `init_agents.txtar`, `agents.txtar`
  gain stamp assertions; the existing second-run "quiet" block in init.txtar
  proves stamp idempotence.

## Approach
- Stamp schema (`.fledge/scaffold.json`, schema 1): `fledgeVersion`, `agents`,
  `files: {path: {policy, sha256|target|lines}}`. `json.MarshalIndent` + trailing
  newline (sorted keys are free from Go maps) → deterministic bytes; written with
  `writeIfChanged` so repeat runs are no-ops. No timestamps or commit shas.
- Policy labels in entries: `core`, `default`, `generate`, `primitive_map`,
  `overwrite`, `symlink`, `append`.
- For skip-if-exists files that were preserved on disk, the recorded hash is still
  the shipped content ("content as shipped at this version") — a user-preserved
  file simply reads as *modified* later, which is what prune safety needs.
- `LoadStamp(root)` returns `(nil, nil)` when absent — the no-stamp path stays
  cheap and non-erroring.
- This stamp is distinct from `repo.Repo.Version()` (the target repo's own
  VERSION file); do not conflate the two.

## Tests
Written test-first: (1) write; (2) observe FAIL against unchanged code for the
expected reason; (3) implement until green.
- **`internal/bootstrap/stamp_test.go`**
  - `TestStampRoundTrip` — Write then LoadStamp reproduces the struct.
  - `TestStampDeterministic` — marshaling twice yields identical bytes; trailing
    newline present.
  - `TestExpectedFilesCoversAllPolicies` — for the claude manifest + core tree,
    every write policy appears with the right entry shape (hash vs target vs
    lines), and `.fledge/scaffold.json` itself is absent.
  - `TestRenderEntryMatchesWritePath` — bytes from `renderEntry` equal the bytes
    `writeFileEntry` writes for generate/primitive_map/overwrite/default files.
- **txtar**: `init.txtar` — `exists .fledge/scaffold.json`,
  `grep '"fledgeVersion"'`, grep one core path and one adapter path in the
  manifest; second-run block stays fully quiet (idempotence).
  `init_agents.txtar`/`agents.txtar` — agents list assertions (e.g.
  `grep '"codex"'`).
- Whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: `fledge init` writes `.fledge/scaffold.json` containing the binary version, the scaffolded agents, and a manifest entry (policy + sha256/target/lines) for every file it wrote, excluding the stamp itself (satisfies PLM-009 FC-1).
- [ ] AC-3: A second `fledge init` (and `--refresh` with no changes) leaves the stamp byte-identical — the init.txtar quiet block passes unchanged in spirit with the stamp present.
- [ ] AC-4: `ExpectedFiles` output matches the write path byte-for-byte for rendered files, providing the shared surface FTHR-011/012 build on; `go test ./...` and `go vet ./...` pass.
