---
generated: 2026-07-17T02:54:09Z
commit: e7a6d4969f861ed3f03af7833b750a7cd703a7a8
agent: fledge-forager
fledge_version: 0.5.8
---

# Context Index

## architecture.md
Explains fledge's two-layer design (deterministic `internal/cli` + domain packages vs. the `internal/bootstrap` scaffolding/adapter system), how the two layers are wired together (`init.go` ↔ `bootstrap`, `preen.go` ↔ `DriftReport`), and how the embedded core-skill prose relates to per-harness adapters and tiers.
Read this when: you need the big picture before touching either the CLI dispatch layer or the bootstrap/scaffold system, or need to understand why a change in one ripples into the other.

## modules.md
A repo map: every top-level Go package and doc/config module, its purpose, key files, and a "look here for" pointer, including the merged small packages (check/graph/ciconfig/doctest/hooktest and ledger/nest/repo/roster/scan).
Read this when: you know roughly what you want to change but need to find which package/file owns it.

## conventions.md
Reconciled conventions across the repo: spec/ID lifecycle rules, CLI exit-code/flag/registration conventions, Go build/lint gates, bootstrap file-write-policy conventions, test conventions, and code-style/scope-discipline rules from the orchestration prose.
Read this when: writing new code or specs and need to match existing patterns (naming, error handling, flag conventions, write policies) rather than reinvent them.

## data-model.md
Exact field-by-field data types: `Requirement`/`Task` structs and frontmatter field order (`internal/spec/types.go`), the full status lifecycle enums, the scaffold `Stamp`/`StampEntry`/`Manifest` types (`internal/bootstrap`), and the brood/roster/ledger/nest/scan record formats.
Read this when: authoring or validating anything that touches spec frontmatter, the scaffold stamp, or any on-disk record format — this is the most precision-critical doc for planning work on spec layout or lifecycle.

## dependencies.md
The full external dependency list (two Go modules: `goccy/go-yaml`, `rogpeppe/go-internal`/testscript), stdlib usage by area, runtime dependencies (git, GitHub Releases API), and internal package dependency direction.
Read this when: adding a new dependency (check if one already covers the need) or tracing what a package pulls in.

## entry-points.md
Build/install commands, the full 19-command CLI surface with flags, the nest-command detail (the process that generated this document set), key public package APIs, and CI/git-hook entry points.
Read this when: you need to invoke fledge (as a user or from a script), add/modify a CLI command, or understand what CI runs.

## testing.md
How to run tests at every level (unit, acceptance/txtar, meta-tests), the full breakdown of what each of the 27 acceptance-test files and each package's unit tests cover, and the test-first evidence discipline used during feather implementation.
Read this when: writing or debugging tests, deciding where a new test belongs, or verifying a change didn't break existing coverage.

## domain.md
Glossary of fledge's bird-themed vocabulary — spec artifacts (plumage/feather/AC/FC), CLI operations (preen/vee/brood/molt/colony), worker roles (incubator/forager/scout/brooder/skua), workflow phases, and the primitive/tier/adapter/harness model.
Read this when: unfamiliar terminology appears in a spec, prose doc, or conversation and you need the precise, source-grounded definition.
