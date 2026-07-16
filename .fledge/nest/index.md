---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Context Index

## architecture.md
Explains fledge's deliberate two-layer split (deterministic CLI vs. manifest-driven bootstrap/adapter system) and how the small `internal/` domain packages (spec, check, graph, lock, scan, repo, nest) compose behind `internal/cli` to implement every command. Also covers the dogfooding loop (this repo runs fledge on itself).
Read this when: orienting to the codebase for the first time, deciding which layer a change belongs in, or tracing how a CLI command reaches its underlying package.

## modules.md
Repo map, one entry per top-level module (per `fledge scan`'s grouping, including the split subtrees of `internal/bootstrap`): purpose, key files, and "look here for…" per module.
Read this when: you know *what* you need to change but not *where* it lives, or need the file list for a specific module before editing it.

## conventions.md
Reconciled naming, lifecycle, layering, error-handling, and process conventions: spec ID/status rules, CLI exit codes and JSON conventions, bird-themed naming, worker-naming scheme, gating/interrogation protocol, test-first discipline, versioning/release rules.
Read this when: writing new code or specs and need to match existing idiom, or unsure what's CLI-owned vs. hand-editable.

## data-model.md
Every core struct/type with file:line references: `Requirement`/`Task`/`Set` (spec), `Finding` (check), `Graph` (graph), `Record`/`HeldError` (lock), `Manifest`/`ManifestFile`/`Stamp`/`StampEntry`/`Drift` (bootstrap), `Doc`/`StatusResult` (nest), plus the on-disk frontmatter shapes for plumages, feathers, nest docs, evidence files, and brood claims.
Read this when: adding or modifying a struct, parsing/writing frontmatter, or needing the exact shape of a JSON/YAML artifact.

## dependencies.md
Deduplicated external dependencies with usage notes: the two direct Go deps (`goccy/go-yaml`, `rogpeppe/go-internal`/testscript) and what uses them, stdlib packages of note (embed, text/template, sha256, os.Link, flock), external CLI tools (git, gofmt, go vet, gh), GitHub Actions, and config-file dependencies (VERSION, .fledgeignore, scaffold.json).
Read this when: adding a new dependency (check what's already available), or tracing what a package actually calls out to.

## entry-points.md
The full 24-command CLI table (command → purpose → implementing file), package-level public APIs each command calls into, the binary's `main()` entry point, skill/workflow entry points for agents, and build/test/run commands.
Read this when: implementing or calling a CLI command, or needing the exact `go build`/`go test`/`fledge init --refresh` incantations.

## testing.md
Frameworks in use (stdlib `testing`, `testscript`/`.txtar`), how to run each kind, and per-package coverage summaries — including the three structural "keep docs/CI honest" packages (`ciconfig`, `doctest`, `hooktest`) and all 23 CLI acceptance fixtures.
Read this when: writing a test, deciding unit vs. acceptance-test coverage, or figuring out how an existing behavior is already verified.

## domain.md
Full glossary of fledge's bird-themed vocabulary: spec types and lifecycle states (plumage/feather/egg/hatched/pipping/hatching/fledged), storage layout (nest/molt/brood/scaffold/burrows), CLI verbs (preen/molt/colony/vee/unfledged), worker roles (forager/scout/incubator/brooder/skua/orchestrator/commissioner), naming mechanics (species), and orchestration architecture terms (primitive/harness/adapter/manifest/tier/drift).
Read this when: any unfamiliar bird-themed term appears, or before naming something new (to match the existing metaphor consistently).
