---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Conventions

Cross-cutting conventions observed across the repository, reconciled from module-level reports.

## Command dispatch

Every CLI command lives in its own `internal/cli/<name>.go` file, registers itself via `register(name, runFunc, usage)` inside a file-level `init()`, and is added to the global `commandOrder` slice — which in turn drives both `fledge --help`/usage text and the generated Claude `settings.local.json` Bash allow-list. Mixed positional/flag argument parsing goes through a shared `parseMixed(fs, args)` helper. Exit codes (`ExitOK/Fail/Usage/Env` = 0/1/2/3) are shared and semantically meaningful across every command; helpers `fail()`, `usageErr()`, `envErr()` print to stderr and return the appropriate code. Output is either human-readable text (`fmt.Printf`) or JSON via `emitJSON()`, selected by a `--json` flag present on every command.

## Spec files are CLI-owned, never hand-edited

IDs (`PLM-###`, `FTHR-###`) and frontmatter are allocated and mutated exclusively through the `fledge` CLI (`new`, `status`, `set`, `criteria`, `brood`). Spec *bodies* (prose after frontmatter) are byte-exact preserved on every write — `internal/spec/frontmatter.go:SplitFrontmatter` never re-serializes the body. Acceptance-criteria checkboxes (`- [ ] AC-N: text`) are toggled by exact byte offset (`internal/spec/criteria.go:SetCriterion`), touching exactly one byte, and only via `fledge criteria check` — never hand-edited. All spec/frontmatter writes are atomic (temp file in the same directory + rename).

## Manifest-driven scaffolding, not code-driven

Every adapter's behavior (which files are written, by which policy, and which primitives it provides) is declared entirely in that adapter's `manifest.yaml`. Adding or changing a target harness requires editing a manifest, not Go code (`internal/bootstrap/registry.go:17-20`). File write policies (`generate`, `primitive_map`, `overwrite`, `append_if_missing`, `symlink`, default skip-if-exists) are enumerated once in `ManifestFile` (`registry.go:37-44`) and apply uniformly across harnesses.

## Byte-idempotent, additive writes

`writeIfChanged` compares on-disk bytes before rewriting scaffolded files, so re-running `fledge init` (without `--refresh`) is a no-op on unchanged files and `fledge init --refresh` only rewrites files whose shipped bytes actually changed. `fledge init` never destroys existing files — reruns only add missing agents/adapters (the "additive invariant", `docs/generalization-plan.md` Q9). Skill prose under `.fledge/skills/` uses skip-if-exists so user edits survive normal reruns; only `--refresh` restores embedded bytes.

## Agent-neutral core prose

Workflow prose in `internal/bootstrap/core/skills/` (`foraging.md`, `planning.md`, `implementation.md`, `worker-protocols.md`) never names a harness-specific path (`.claude/`, `.pi/`, `.codex/`, `.cursor/`) — enforced by `TestCoreNeutral` (`internal/bootstrap/registry_test.go:140-159`). Cross-references to other skill files use relative, self-referential phrasing ("the template at `templates/scout-report.md` in this skill's directory"), not adapter-specific paths, so the same prose renders correctly regardless of which harness scaffolded it.

## Primitive/tier discipline

Every primitive named in core prose must be one of the canonical 7 (`TestCorePrimitivesReferenced`, `registry_test.go:116-137`) and must be declared by at least one adapter — an unreferenced primitive is treated as a dead contract. Tiers (A/B/C) are always *derived* from an adapter's declared primitive coverage (`DeriveTier()`), never hand-declared in a manifest — this lets an adapter with an unusual primitive profile (e.g. fan-out but no team loop) self-label the correct tier rather than being miscategorized.

## Terminology: bird-themed throughout

`nest`, `plumage`, `feather`, `brood`, `preen`, `molt`, `forager`, `scout`, `vee`, `skua`, `colony`, `brooder`, `fledged`/`unfledged`. `README.md` decodes the metaphor; match it in new code, commands, and prose rather than introducing generic naming.

## Worker naming (team loop / Tier C)

Spawned workers get unique names from a fixed penguin-species list (emperor, king, adelie, ... macaroni — 18 base names, numeric suffix once exhausted), per `internal/bootstrap/core/skills/fledge-orchestrate/implementation.md` §3.1. The orchestrator itself is never given a species name — it uses whatever identity the harness assigns (e.g. `team-lead` on Claude Code).

## Testing conventions

Acceptance tests use the testscript/txtar format under `cmd/fledge/testdata/*.txtar`; unit tests sit beside their package (`internal/<pkg>/<pkg>_test.go`) using standard `testing.T`, table-driven where practical, with `t.TempDir()` for filesystem isolation and `exec.Command("git", "init", ...)` where a real git repo is needed (scan, lock tests). See `testing.md` for the full breakdown.

## Open Questions
- How are `commandOrder` and each adapter's generated allow-list kept in sync as new commands are added — is there a check that fails if a command is missing from an allow-list? (unresolved from internal-domain, internal-bootstrap reports)
