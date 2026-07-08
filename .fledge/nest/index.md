---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Context Index

## architecture.md
The two-layer design (deterministic `internal/cli`+domain packages vs the `internal/bootstrap` embed/adapter system), the 6-primitive/derived-tier contract, the init→skill→CLI dogfooding loop, and a ripple map of what must change together.
Read this when: you need the big picture before a cross-cutting change, are about to touch `internal/bootstrap` or add/change a harness adapter, or want to know what else a change forces you to update.

## modules.md
Repo map by `fledge scan` module (root, cmd, internal/cli, internal domain packages, internal/bootstrap, pluma, docs): purpose, key files, and a "look here for" pointer each.
Read this when: you know roughly what to change but not which files/package own it — orient here first.

## conventions.md
Observed idioms: command-registration + exit-code/`--json` discipline, CLI-owns-frontmatter rule, byte-idempotent writes, manifest-driven scaffolding + write policies, agent-neutral core prose, bird naming, and this repo's dogfooding install/refresh discipline.
Read this when: writing new code or prose you want to match existing style — especially before adding a CLI command, a scaffolded file, or touching frontmatter.

## data-model.md
The on-disk artifacts and their backing types: plumage, feather, brood/lock, molt/evidence, nest docs, manifest — plus the integrity relationships (orphan feather, dangling ref, evidence-for-criteria) between them.
Read this when: changing spec/frontmatter schema, lock/evidence handling, or anything about how plumage↔feather↔evidence link and how their integrity is checked.

## dependencies.md
Third-party libs (goccy/go-yaml, go-internal/testscript; lean, hand-rolled dispatch), the one-way internal package dependency direction, and the target agent harnesses with their detection markers and open discovery questions.
Read this when: adding a dependency, reasoning about package coupling, or working on harness detection/adapters.

## entry-points.md
Where execution and agents enter: process entry, the full CLI command surface grouped by role, the triage-relevant split between `preen` (errors) and `colony` (observer report), and the harness-loaded files (.claude adapter, skill symlink, CLAUDE.md).
Read this when: adding/altering a command, working on validation/orphan/evidence triage, or on how a coding agent detects and enters the workflow.

## testing.md
The two test styles (testscript/txtar acceptance under cmd/fledge/testdata, colocated unit tests), which fixtures break when embedded content changes, the test-first evidence discipline, and the standard build/vet/test checks.
Read this when: adding tests, changing scaffolded/embedded content (fixtures will break), or verifying a change.

## domain.md
The bird-themed glossary that is load-bearing across commands, packages, and agent roles: artifacts, lifecycle states, operations, the four agent roles, the brooder↔skua species-pairing scheme, and structural concepts (primitive, tier, oversight, tracer).
Read this when: you hit unfamiliar terminology, are writing user-facing prose, or working on agent-role/worker-pairing behavior or agent terminology comprehension.
