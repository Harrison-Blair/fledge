---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Architecture

Covers fledge's two-layer design (deterministic CLI + agent-neutral bootstrap/adapter system), how the binary, CLI, and bootstrap embed relate, and how this repo dogfoods its own scaffold.

## Two layers, deliberately separated

1. **The CLI** (`internal/cli`, 26 files, plus domain packages under `internal/`) — deterministic, agent-agnostic spec operations. `cmd/fledge/main.go` is a 1-line dispatcher: it calls `internal/cli.Run(os.Args[1:])` and exits with the returned code. `internal/cli/cli.go` holds `Run`, command registration (`register(name, run, usage)` called from each command file's `init()`), the `commandOrder` slice (19 commands, exact order: `init, agents, scan, new, nest, preen, ready, vee, colony, unfledged, status, set, criteria, brood, abandon, broods, roster, version, update` — verified via `awk '/commandOrder = /,/^}/' internal/cli/cli.go`), and the shared exit codes (`ExitOK=0, ExitFail=1, ExitUsage=2, ExitEnv=3`).
2. **The bootstrap/adapter system** (`internal/bootstrap`, 36 files) — what `fledge init` scaffolds into a target repo. `bootstrap.go` embeds two trees via `//go:embed core adapters`: `core/` is the single agent-neutral source (the `fledge-orchestrate` and `fledge-interrogate` skills — planning.md, implementation.md, foraging.md, worker-protocols.md, templates/), and `adapters/<harness>/` is a thin, manifest-driven, format-only mapping per harness (claude, codex, pi). Adding a harness means writing a manifest, not Go code.

## The 6-primitive contract

`internal/bootstrap/primitives.go` defines `PrimitiveOrder` — 6 primitives in fixed order: `confirm-gate, read-only-shell, write-file, run-fledge, spawn-worker, message-peer`. Each adapter's manifest declares which concrete mechanism realizes each primitive (e.g. Claude Code: `AskUserQuestion`, `Bash`, `Write`, `Bash(fledge ...)`, teammate-spawn, `SendMessage`). An adapter's **tier** (A/B/C) is *derived*, never declared, by `DeriveTier()` (`primitives.go:46`) from which primitives it provides: Tier A = base 4 (confirm-gate, read-only-shell, write-file, run-fledge) → solo work only; Tier B = A + spawn-worker → can fan out foraging; Tier C = B + message-peer → full team loop (brooder/skua pairs, incubator delegation). Per the `internal-bootstrap-adapters` scout: Claude Code provides all 6 (Tier C); Codex and pi each provide 4 — missing spawn-worker and message-peer, so they run planning/implementation solo, in-session (`registry_test.go`'s `TestPrimitiveCoverage` asserts derived tiers `claude:C, codex:A, pi:A`).

## Scaffolding mechanics

`internal/bootstrap/registry.go` defines `Manifest` (name, detector, `tier_primitives` map, `Files []ManifestFile`, optional `PipingFile`) and `ManifestFile` write policies: `generate`/`primitive_map` (render a Go `text/template`), `overwrite` (always rewritten), `append_if_missing` (additive one-liner into CLAUDE.md/AGENTS.md), `symlink` (e.g. `.claude/skills/...` → `.fledge/skills/...`), and the default (copy, skip-if-exists so user edits survive `fledge init`; `--refresh` re-syncs). `WriteCore()`/`WriteAdapter()` perform the writes; `internal/bootstrap/stamp.go` computes `ExpectedFiles()` (rendered content + sha256 per file) and writes `.fledge/scaffold.json` (the `Stamp`: fledge version, agent list, per-file policy+hash/target/lines) that `fledge preen` and `fledge init --refresh` use for drift detection. `internal/bootstrap/drift.go` classifies every scaffolded file into one of 5 `DriftStatus` values (up-to-date, stale, modified, missing, obsolete) by comparing disk bytes against both the old stamp and the newly-expected content — this is what lets `--refresh` safely reset fledge-owned files while a plain re-run leaves user edits alone.

## Cross-module relationships

- `cmd/fledge/main.go` → `internal/cli.Run()` → per-command handler → `internal/repo` (repo root discovery), `internal/spec` (load/parse/mutate PLM/FTHR files), `internal/check` (preen validation), `internal/graph` (dependency graph for `vee`/`ready`), `internal/lock` (brood claim files), `internal/scan` (module discovery), `internal/roster` (agent species allocation), `internal/nest` (concern-doc scaffolding/status), `internal/bootstrap` (adapter/scaffold I/O for `init`/`agents`/`preen` drift check).
- This repo (`fledge` itself) is fledge-managed: it has its own `.fledge/pluma/` specs, `.fledge/nest/` (this document set), `.fledge/broods/` claims, and a scaffolded `.claude/` adapter (agents under `.claude/agents/*.md` are symlinks into `internal/bootstrap/adapters/claude/agents/`). Changes to workflow behavior are made in `internal/bootstrap/...` (source of truth), then `fledge init --refresh` regenerates this repo's own scaffolded copies — never hand-edit the scaffolded `.fledge/skills/` or `.claude/` files directly.
- CI (`.github/workflows/pr-check.yml`, `release.yml`) and the optional local `scripts/hooks/pre-commit` hook both gate on `gofmt -l .` + `go vet ./...` (+ `go test ./...` for CI); `release.yml` triggers a 4-platform build only when `VERSION` changes on `main`.

## Open Questions

- Exact go/template rendering context (`{{.Tier}}`, `{{.Provided}}`, `{{.Rows}}`) for `generate`/`primitive_map` files — the struct lives in `registry.go` (`renderContext`, `primitiveRow`) but full template execution flow wasn't traced end-to-end by scouts.
- Whether the pi adapter's prompts (`fledge-plan.md`, `fledge-implement.md`) referencing "Tier A" solo implementation are fully consistent everywhere with the primitive-coverage-derived tier (flagged by the `internal-bootstrap-adapters` scout as worth double-checking).
