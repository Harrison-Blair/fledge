---
generated: 2026-07-16T02:20:48Z
commit: 407b91e70b53764944447dae5829d2076fb852c5
agent: fledge-forager
fledge_version: 0.5.5
---

# Context Index

## architecture.md
Explains the two-layer split (deterministic `internal/cli` command layer vs. the embedded `internal/bootstrap` scaffold/adapter system), the 6-primitive/tier-derivation model, the 6 manifest write policies, drift/stamp mechanics, and how this repo's own dogfooding loop (specs, nest, broods) fits together.
Read this when: you need the big-picture map before touching `internal/bootstrap`, adding a harness adapter, or understanding why CLI and bootstrap are separated.

## modules.md
Repo map organized by `fledge scan` module: `.github`+`scripts`, `<root>`, `cmd`, `docs`, and the four `internal/*` groupings (bootstrap, cli, spec, and a merged misc group of nine small packages) — each with purpose, key files, and "look here for" pointers.
Read this when: you know roughly what you want to change but not which file/package owns it.

## conventions.md
Reconciled naming/error-handling/testing/documentation idioms observed across the codebase: exit-code semantics, atomic/byte-preserving writes, fixed frontmatter key order, manifest-driven extension, and the "assert on docs/CI as data" testing pattern.
Read this when: writing new code or specs and you want to match existing style, or you're unsure whether a pattern (e.g. atomic write, flock) is a repo-wide convention or local to one file.

## data-model.md
Every core Go struct/enum with field-level detail: spec types (Requirement/Task/Set/Criterion), lock/brood records, check Findings, graph, bootstrap Manifest/Stamp/Drift types, and the nest Doc/Kind/StatusResult schema.
Read this when: you need exact field names/types before writing code that constructs, parses, or serializes any of these structures.

## dependencies.md
The one third-party runtime dependency (`goccy/go-yaml`) and one test-only dependency (`go-internal`/testscript), deduplicated with per-module usage notes, plus external services (GitHub Releases API, GitHub Actions, git subprocess) and notable absences (no DB, no web framework, no mocking library).
Read this when: adding a new dependency (to check whether an existing one already covers the need) or tracing what a network/subprocess call in the codebase talks to.

## entry-points.md
The `main()` bridge, build/install/test commands, the full 18-command CLI surface with descriptions, exit codes, the optional pre-commit hook setup, and the agent-facing entry points (`SKILL.md`, `fledge-adapter.md`) for this repo's own dogfooding.
Read this when: you need to run/build/test the project, or want the one-line summary of what a specific `fledge` subcommand does.

## testing.md
Frameworks (stdlib testing + testscript/txtar), exact run commands, and a layer-by-layer breakdown of what's covered: acceptance txtar files, per-package unit tests, and the "docs/CI as tests" pattern (`ciconfig`, `doctest`, `hooktest`).
Read this when: adding a test, deciding whether something needs an acceptance txtar vs. a unit test, or figuring out how to run just the tests relevant to your change.

## domain.md
Glossary of the bird-themed vocabulary (plumage, feather, brood, preen, molt, nest, scout, forager, brooder, skua, incubator, primitive, tier, wave, colony, etc.) with precise lifecycle states and cross-references.
Read this when: you hit an unfamiliar term in code, specs, or another nest doc and need its definition, or you're writing new prose and want to use the established terminology correctly.

## Open Questions surfaced during synthesis
- Whether `docs/generalization-plan.md`'s 7-primitive contract (includes `spawn-pool`) reflects a dropped/deferred primitive relative to the shipped 6-primitive `internal/bootstrap/primitives.go`.
- Whether "molt" canonically refers to the evidence directory (`.fledge/molt/`) or to AC-checkbox heading style (CLAUDE.md phrasing) — both usages exist and weren't reconciled to one definition.
- Whether user-authored adapter manifests are a supported extension point beyond the three shipped harnesses (claude/codex/pi).
