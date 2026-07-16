---
generated: 2026-07-16T21:27:15Z
commit: a1ed62a38540df7ab1cbdc4c486176a64a762018
agent: fledge-forager
fledge_version: 0.5.8
---

# Context Index

## architecture.md
Explains fledge's two-layer split (deterministic `internal/cli` vs. the manifest-driven `internal/bootstrap` scaffold/adapter system), the 6-primitive/tier-derivation model, scaffold write policies and drift detection, and how the three orchestration phases (planning/foraging/implementation) chain together — including that this repo's own `.fledge/skills/` is scaffolded output of `internal/bootstrap/core/...`, never hand-edited.
Read this when: orienting to the codebase for the first time, changing anything in `internal/bootstrap`, or needing to understand how a harness's tier is determined.

## modules.md
One entry per top-level module `fledge scan` reports (with `internal/bootstrap` and `internal` further split by subdirectory to match how it was actually scouted): purpose, key files, and a "Look here for" pointer for each.
Read this when: deciding which files to open for a task, or scoping a feather's "Affected Modules" section.

## conventions.md
Naming/structural/process idioms actually observed in the code and prose: command-registry pattern, exit codes, byte-preservation and atomic-I/O guarantees, manifest-driven adapter design, spec-lifecycle rules (frontmatter ownership, AC-1 test-first, no commit trailers), and worker-naming/communication-topology rules.
Read this when: writing new CLI code, editing spec templates, or authoring/reviewing a feather and need to match existing idiom.

## data-model.md
Every core struct across `internal/spec`, `internal/lock`, `internal/check`, `internal/graph`, `internal/cli` JSON shapes, `internal/bootstrap` (Manifest/Stamp/Drift), and `internal/nest` (Doc/StatusResult), plus the prose-level shapes (evidence file, relay envelope) that aren't Go types.
Read this when: touching frontmatter/JSON serialization, adding a CLI output field, or needing the exact schema of a brood record, scaffold stamp, or nest status result.

## dependencies.md
The two direct Go module deps (`goccy/go-yaml`, `rogpeppe/go-internal`/testscript), load-bearing stdlib usage (flock, sha256, embed, text/template, os/exec-to-git), and external CI/release tooling (GitHub Actions, `gh` CLI).
Read this when: adding a new dependency, touching the release workflow, or tracing where YAML/git-shelling/hashing happens.

## entry-points.md
Build/install/run commands, the full `fledge` CLI command surface (19+ subcommands with flags), the binary entry point chain (`main.go`→`cli.Run`), and the agent-facing entry points into the orchestration workflow (`SKILL.md` routing, per-harness prompt files, worker spawn prompts).
Read this when: needing the exact invocation for any `fledge <cmd>`, or tracing how an agent first enters the orchestration workflow on a given harness.

## testing.md
How to run tests at every scope (`go test ./...` down to one txtar fixture), what each of the ~10 test-bearing packages actually covers (with notable specifics like the 16-way lock-contention test and the ~15 embedded-prose "guard" tests pinning core skill doc sentences), and the test-first/evidence-capture process convention.
Read this when: writing or running tests, deciding which fixture to update after a scaffold/CLI change, or verifying a feather's AC-1 evidence expectations.

## domain.md
Glossary of fledge's bird-themed vocabulary grouped by theme: spec artifacts and lifecycle (plumage, feather, brooding, molting, fledging), repo structures (nest, scaffold, stamp, roster), orchestration roles (forager, scout, incubator, brooder, skua), and orchestration mechanics (primitive, tier, adapter, manifest, relay envelope, green teardown).
Read this when: unsure what a term means anywhere else in this doc set, or onboarding to the project's vocabulary before writing prose that uses it.

## Open Questions carried forward

- `docs/generalization-plan.md` milestones M0–M5 status vs. current code is unverified (architecture.md).
- `piping_file` manifest field appears Claude-only — unclear if Codex/pi team loops are unsupported or handled differently (architecture.md).
- `docs/google_ai_mode_response.md`'s external AI-infrastructure proposal has unclear relevance to current fledge scope (dependencies.md).
- Skua's "traceable to spec" scope-creep threshold and commit-message content conventions beyond "no trailers" are unspecified in prose (conventions.md).
