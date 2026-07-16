---
id: FTHR-059
title: Repoint Claude adapter agent definitions to per-role files
plumage: PLM-027
status: egg
priority: P1
depends_on: [FTHR-057]
authored: 2026-07-16T16:17:22Z
agent: fledge-orchestrate/planning
fledge_version: 0.5.8
---

# FTHR-059: Repoint Claude adapter agent definitions to per-role files

## Description
Repoint the Claude adapter's three worker-role agent definitions — each of which currently tells its worker to "read the '<Role>' section of `worker-protocols.md`" — to their own new per-role file created by FTHR-057 (satisfies PLM-027 FC-4, the Claude-adapter half).

## Affected Modules
Per `.fledge/nest/modules.md` (internal-bootstrap-adapters):
- `internal/bootstrap/adapters/claude/agents/fledge-incubator.md` (line 13)
- `internal/bootstrap/adapters/claude/agents/fledge-brooder.md` (line 9)
- `internal/bootstrap/adapters/claude/agents/fledge-skua.md` (line 10)

These are the source files the repo's own `.claude/agents/*.md` symlink to (per `.fledge/nest/architecture.md`) — edited here, then picked up automatically via the existing symlinks (no separate repointing needed for the repo's own `.claude/agents/`).

## Approach
- `fledge-incubator.md:13` — currently: "**`.fledge/skills/fledge-orchestrate/worker-protocols.md`, "Incubator" section** — your relay envelope..." → repoint to "**`.fledge/skills/fledge-orchestrate/incubator.md`** — your relay envelope...", dropping the now-meaningless "section" qualifier since the whole file is the protocol.
- `fledge-brooder.md:9` — currently: "**Read the "Brooder" section of `.fledge/skills/fledge-orchestrate/worker-protocols.md` and follow it exactly.**" → "**Read `.fledge/skills/fledge-orchestrate/brooder.md` and follow it exactly.**"
- `fledge-skua.md:10` — currently: "**Read the "Skua" section of `.fledge/skills/fledge-orchestrate/worker-protocols.md` and follow it exactly.**" → "**Read `.fledge/skills/fledge-orchestrate/skua.md` and follow it exactly.**"

Do not touch any other line in these three files (spawn-prompt field lists, communication rules summaries, etc. are out of scope — only the worker-protocols.md citation changes).

## Tests
- `TestClaudeAgentDefsRepointToRoleFiles` (new, in `internal/bootstrap` alongside the existing agent-definition assertions, or `internal/bootstrap/adapters` if that's where such tests currently live — confirm via `grep -rl "fledge-incubator.md" internal/bootstrap/**/*_test.go` before placing): reads the three embedded agent files and asserts each references its own new per-role file (`incubator.md` in `fledge-incubator.md`, `brooder.md` in `fledge-brooder.md`, `skua.md` in `fledge-skua.md`) and none contain the stale `worker-protocols.md` "section" phrasing.
- Implementation order: write the test against the unchanged repo (fails — old phrasing still present), then make the edits, confirm it passes.

## Acceptance Criteria
- [ ] AC-1: The test listed above was observed failing before implementation and passes after.
- [ ] AC-2: `fledge-incubator.md`, `fledge-brooder.md`, `fledge-skua.md` each reference their own new per-role file instead of a `worker-protocols.md` section (satisfies PLM-027 FC-4, AC-3 — Claude-adapter half).
- [ ] AC-3: `go test ./internal/bootstrap/...` passes.
