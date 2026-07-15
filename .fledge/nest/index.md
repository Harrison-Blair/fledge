---
generated: 2026-07-15T18:14:39Z
commit: 5728c29953a7c218c923ce20333dbffebb00623f
agent: fledge-forager
fledge_version: 0.5.4
---

# Context Index

## architecture.md
The two-layer split (deterministic CLI vs. bootstrap/adapter scaffolding), the 6-primitive contract and tier derivation, scaffold write policies/drift, and how the agent-neutral orchestration workflow (planning/implementation phases, worker-protocols.md's Skua review checklist/verdict rules, Claude's team-loop.md tmux precondition/fallback) fits together end to end.
Read this when: you need the big picture before touching `internal/bootstrap/`, planning changes to the Skua review protocol or the tmux teammate-display fallback, or adding/changing a harness adapter.

## modules.md
Repo map from `fledge scan`'s module list: root/CI/scripts, cmd (CLI entry + acceptance tests), docs (superseded design doc + unrelated research briefs), internal/bootstrap split into core-skills and adapters, internal/cli, and internal/domain (spec/check/graph/lock/nest/repo/scan + test-only packages).
Read this when: you know *what* you need to change but not *which file* — start here to find the right module, then drill into its scout report or the relevant concern doc.

## conventions.md
Go coding conventions (command registration, byte-preservation, atomic writes, concurrency locking), scaffold/bootstrap write-policy conventions, spec lifecycle and CLI-only mutation rules, worker naming/communication topology, and version-sync discipline at release time.
Read this when: writing new Go code in this repo, adding a CLI command, or deciding how a new worker role should be named/communicate.

## data-model.md
Concrete Go struct definitions for spec types (Requirement/Task/Criterion), lock records, nest Doc/Stamp/Drift types, scan Module/Result, bootstrap Manifest/ManifestFile/StampEntry, and every CLI `--json` output shape.
Read this when: you need exact field names/types before writing code that parses or produces fledge's JSON, frontmatter, or scaffold-stamp data.

## dependencies.md
Go module dependencies (goccy/go-yaml, rogpeppe/go-internal/testscript) with usage sites, external subprocess tools (git, gofmt, go vet, gh), GitHub Actions dependencies, and per-harness runtime capabilities each adapter assumes (Claude's tmux/SendMessage/TaskStop, Codex's AGENTS.md, Pi's fledge_gate).
Read this when: auditing what fledge links against or shells out to, or checking what a specific harness (Claude/Codex/Pi) must actually provide to realize a given primitive.

## entry-points.md
Build/install/test commands, the full CLI dispatch table with the commands most relevant to context/spec work highlighted, the agent-facing SKILL.md entry point, and the internal Go API surface consumed by `cmd/fledge`.
Read this when: you need the exact command to build, reinstall, or invoke fledge, or you're tracing which Go function a CLI verb calls into.

## testing.md
Testscript (txtar) and Go-unit-test frameworks, how to run each, the full acceptance-test file list mapped to what each covers (the authoritative behavioral spec — update alongside any core/adapters change), and notable patterns like the concurrent-allocation race tests and enforced test-first discipline.
Read this when: adding or changing behavior that needs a txtar fixture update, writing a new unit test, or verifying CI/pre-commit gate coverage.

## domain.md
Full bird-themed glossary: spec artifacts (plumage/feather/AC/molt), repository infrastructure (nest/brood/colony/preen/scaffold/stamp), orchestration primitives and tiers, spawned worker roles (incubator/forager/scout/brooder/skua) with the Skua's exact review-checklist and verdict terms, and process gates (confirm-gate, create-then-gate, escalation).
Read this when: any prose you're reading or writing uses fledge's bird terminology and you need the precise definition — especially before editing worker-protocols.md's Skua section or team-loop.md's shutdown/species rules.

## Notes for the two upcoming planning efforts

- **Adversarial fledge-skua**: see architecture.md's "Skua review protocol" subsection and domain.md's Skua entry for the current `### Reviewing a feather` checklist and `### Verdict` behavior (findings, third-rejection escalation, pass-to-orchestrator) before designing changes — source is `internal/bootstrap/core/skills/fledge-orchestrate/worker-protocols.md` § Skua and `internal/bootstrap/adapters/claude/agents/fledge-skua.md`.
- **tmux auto-default**: see architecture.md's "Team-loop mechanics" subsection for the current `test -n "$TMUX"` precondition and its confirm-gated in-process fallback — source is `internal/bootstrap/adapters/claude/team-loop.md` § Teammate display (tmux) and `implementation.md` §1. Remember: any change to this prose requires `go install ./cmd/fledge && fledge init --refresh` in this repo plus updating `cmd/fledge/testdata/init.txtar`/`init_agents.txtar`/`agents.txtar`.
