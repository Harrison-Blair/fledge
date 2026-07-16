---
generated: 2026-07-16T04:02:08Z
commit: 154510fc963e7071b2f09297ecfeba2b6710e85e
agent: fledge-forager
fledge_version: 0.5.8
---

# Context Index

## architecture.md
The two-layer design (deterministic `internal/cli` vs. agent-neutral `internal/bootstrap`), the 6-primitive contract and tier derivation (A/B/C), and how scaffolding/drift detection works end to end.
Read this when: you need to understand how the CLI and the bootstrap/adapter system relate, why tiers are derived not declared, or how `fledge init`/`--refresh` decides what to touch.

## modules.md
Repo map — one entry per `fledge scan` module (root, cmd, docs, .github+scripts, internal/cli, internal/bootstrap ×2, internal/spec, internal/nest, internal-misc ×9-packages), with key files and "look here for" pointers.
Read this when: you need to find which file/package owns a piece of behavior, or want a fast orientation before diving into one area.

## conventions.md
Naming/lifecycle/error-handling/testing conventions, and the full "Versioning & release" section naming all three files that must move together on a release (VERSION, internal/cli/version.go's binaryVersion, cmd/fledge/testdata/stamp_warning.txtar).
Read this when: making a release, adding a new command/spec-field, or needing to match existing patterns before writing new code.

## data-model.md
Core Go types (Requirement, Task, Set, Criterion, Manifest, Stamp, Drift, brood Record, roster Entry, scan Result, graph Graph, check Finding) with exact fields and file references.
Read this when: you need a struct's exact fields/JSON shape, or are writing code that consumes/produces one of these types.

## dependencies.md
The 2 direct + 2 indirect Go module dependencies (goccy/go-yaml, rogpeppe/go-internal/testscript, x/sys, x/tools) plus external tools (Go toolchain, GitHub Actions, gh CLI, git) with usage notes.
Read this when: adding a new dependency, auditing what's actually used, or tracing where YAML/testscript/git-shelling happens.

## entry-points.md
The binary entry point (`cmd/fledge/main.go` → `internal/cli.Run`), the exact 19-command list with one-line purposes (verified via `awk` over `commandOrder` in cli.go), build/install/run commands, and scaffolded entry points generated into consumer repos.
Read this when: you need the exact command list/count, how to build or install the binary, or what files `fledge init` puts into a target repo.

## testing.md
The two test layers (25 txtar acceptance fixtures under `cmd/fledge/testdata/`, and per-package unit tests), a fixture-by-fixture coverage table, notable unit-test suites, and how to run any of them.
Read this when: adding or modifying a command (update its txtar fixture), verifying a fix, or working out how CI enforcement (gofmt/vet/test) is wired.

## domain.md
Glossary of fledge's bird-themed vocabulary — spec hierarchy (plumage/feather/AC), repo artifacts (nest/brood/molt/roster/scaffold/colony/vee), process operations (preen/forage/scout/interrogate), orchestration roles (brooder/skua/incubator/forager), and the primitive/tier capability model.
Read this when: you hit unfamiliar terminology anywhere in this repo's specs, skills, or prose and need the precise definition.

## Open Questions (cross-cutting, not owned by one doc)

- Whether `docs/generalization-plan.md` (locked design, 23 resolved decisions) has been converted into a tracked plumage/feathers yet, or remains reference-only design prose.
- Whether `internal/nest/templates/*.md` and `internal/bootstrap/core/skills/fledge-orchestrate/templates/*.md` are kept in sync by convention or generation — not resolved from reading either in isolation.
