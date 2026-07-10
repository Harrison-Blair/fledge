---
id: FTHR-008
title: scaffold CLAUDE.md fledge-detection pointer for the Claude adapter
plumage: PLM-004
status: pipping
priority: P1
depends_on: []
oversight: merge
authored: 2026-07-08T06:44:22Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-008: scaffold CLAUDE.md fledge-detection pointer for the Claude adapter

## Description
Makes `fledge init` for the Claude harness emit a top-level detection pointer in
`CLAUDE.md` — the project-memory file Claude Code auto-loads on entry — so a
freshly initialized Claude repo tells the agent it is fledge-managed and routes
work through the orchestration workflow (satisfies both PLM-004 user stories).
The whole plumage: no new Go code. The proven `append_if_missing` write policy
(`registry.go:392`, `ensureLine`) already creates the file when absent (AC-1) and
is idempotent when the line is present (AC-2) — it ships today for the Codex
`AGENTS.md` pointer. This feather adds the equivalent manifest entry for Claude
and the fixtures that prove it.

## Affected Modules
- `internal/bootstrap/adapters/claude/manifest.yaml` — add one `files` entry:
  `append_if_missing` targeting `CLAUDE.md` with the pointer line. Mirrors the
  Codex entry in `internal/bootstrap/adapters/codex/manifest.yaml` (see
  `.fledge/nest/architecture.md` → manifest-driven adapters; `.fledge/nest/modules.md`
  → internal/bootstrap).
- `cmd/fledge/testdata/init_agents.txtar` — new Claude-pointer assertions
  (see Tests).
- `cmd/fledge/testdata/init.txtar` — update only if the added `created CLAUDE.md`
  output line shifts what the Claude-init scenario asserts.
- `internal/bootstrap/registry_test.go` — update only if a coverage/neutrality
  assertion enumerates the Claude file list; the entry already satisfies the
  "src, append_if_missing, or symlink" guard at `registry_test.go:36`.
- This repo's own `CLAUDE.md` — appended by `fledge init --refresh` during the
  dogfooding regeneration the ripple map requires (visible in the merge diff).

## Approach
- Add to `adapters/claude/manifest.yaml` `files`, mirroring Codex exactly except
  the adapter path:
  ```yaml
  - dst: CLAUDE.md
    append_if_missing: "> fledge: load and follow .fledge/skills/fledge-orchestrate/SKILL.md — primitive map at .claude/fledge-adapter.md"
  ```
- No Go changes: `writeManifestFile` already routes `append_if_missing` through
  `ensureLine` (create-if-absent + idempotent). Confirm by test, don't add code.
- Rebuild + reinstall (`go install ./cmd/fledge`, `hash -r`) then regenerate this
  repo's scaffold with `fledge init --refresh`; review the appended line in this
  repo's `CLAUDE.md`.

## Tests
Test-first (txtar acceptance, mirroring the existing Codex block in
`init_agents.txtar` ~line 49–55). Written and observed FAILING against the
unchanged manifest (no CLAUDE.md is created → the `exists`/`grep` lines fail),
then the manifest entry makes them pass.
- **AC-1 (create-when-absent):** in a repo with `.claude/` and no `CLAUDE.md`,
  `fledge init` → `exists CLAUDE.md` and `grep 'fledge-orchestrate/SKILL.md' CLAUDE.md`.
- **AC-2 (additive + idempotent):** seed `CLAUDE.md` with a sentinel line of
  existing prose, `fledge init` → `grep '<sentinel>' CLAUDE.md` (preserved) and
  `grep 'fledge-orchestrate/SKILL.md' CLAUDE.md`; re-run `fledge init` →
  `grep -count=1 'fledge-orchestrate/SKILL.md' CLAUDE.md`.
- **AC-3 (wording matches Codex):** `grep` the exact full pointer line, differing
  from the Codex line only in `.claude/` vs `.codex/`.
- **Suite:** `go test ./...` green; `go build ./...` and `go vet ./...` clean.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after (the AC-1/AC-2/AC-3 txtar cases failed against the unchanged manifest — no CLAUDE.md created — and pass once the entry is added).
- [ ] AC-2: In a Claude repo with no pre-existing CLAUDE.md, `fledge init` creates CLAUDE.md containing the pointer line (satisfies PLM-004 FC-1, AC-1).
- [ ] AC-3: In a Claude repo with an existing CLAUDE.md, `fledge init` appends the pointer without altering existing content, and a repeated init leaves exactly one copy (`grep -count=1`) — satisfies PLM-004 FC-2, AC-2.
- [ ] AC-4: The scaffolded line matches the Codex pointer verbatim except for the adapter path (`.claude/fledge-adapter.md`), asserted by an exact-line `grep` — satisfies PLM-004 FC-3, AC-3.
- [ ] AC-5: `go build ./...`, `go vet ./...`, and the full `go test ./...` suite pass; no Go source under `internal/` or `cmd/` changed except test fixtures (satisfies PLM-004 AC-4).
