---
generated: 2026-07-08T01:03:26Z
commit: e44524d1f089dcfe1c1f313f819ec18d9a42eceb
agent: fledge-forager
fledge_version: 0.2.1
---

# Context Index

## architecture.md
Describes the three-part system: the deterministic `internal/cli`+domain-package layer, the embedded `internal/bootstrap` scaffolding/adapter layer (7-primitive contract, tier derivation, manifest-driven adapters, file write policies), and the `pluma/` spec corpus they both operate on — plus exactly how the layers interact (init scaffolds → agent uses skill → agent drives CLI).
Read this when: you need to understand how a change ripples across layers, are about to touch `internal/bootstrap` or add/change a harness adapter, or need the big picture before a cross-cutting change.

## modules.md
Repo map organized by `fledge scan` module (root, cmd, docs, internal/bootstrap, internal domain packages, pluma): purpose, key files, and a "Look here for" pointer per module.
Read this when: you know roughly what you need to change but not which files/directory own it — start here to orient before diving into a specific concern doc.

## conventions.md
Cross-cutting patterns actually observed in the code: command-registration pattern, CLI-owns-frontmatter discipline, manifest-driven scaffolding, byte-idempotent writes, agent-neutral core-prose rules, primitive/tier discipline, bird-themed naming, worker-species naming, testing conventions.
Read this when: writing new code in this repo and you want it to match existing idiom — especially before adding a CLI command, a scaffolded file, or touching spec frontmatter.

## data-model.md
Every core Go type (`Requirement`, `Task`, `Set`, `Criterion`, `Finding`, `Graph`, `Record`, `Repo`, `Result`, `Manifest`, `ManifestFile`, ...) with file:symbol references, plus an ASCII relationship diagram tying plumage/feather/lock/evidence/manifest together.
Read this when: you need to know a type's exact fields, or how spec/lock/evidence/manifest records relate to each other, before writing code that touches them.

## dependencies.md
The (short) list of third-party Go packages (`goccy/go-yaml`, `rogpeppe/go-internal`), notable stdlib usage (`embed`, `text/template`, `os/exec` for git), and the non-Go runtime dependency on git; also lists per-harness adapter mechanisms (tmux, `AskUserQuestion`, `fledge_gate`, etc.) as external integration points.
Read this when: adding a dependency, wondering whether a capability already exists via an existing library, or working on a harness adapter and need to know what mechanism it should target.

## entry-points.md
Every way into the system: the `main()` binary entry, build/install/verify commands, all 16 CLI commands with one-line descriptions, the core Go domain APIs (`spec.Load`, `check.Run`, `graph.New`, `lock.Acquire`, `scan.Run`), the `internal/bootstrap` public API, and the agent-facing skill entry points (`SKILL.md` files).
Read this when: you need the exact command/flag/API surface for a command or package, or need to know how an agent enters the orchestration workflow.

## testing.md
How to run every test tier (`go test ./...`, scoped acceptance tests, scoped unit tests), a table mapping each of the 17 `cmd/fledge/testdata/*.txtar` files to what it covers, a list of every `internal/*/**_test.go` file with what it exercises (including the 9-test `internal/bootstrap/registry_test.go` suite), and the test-first spec-level convention (AC-1 pattern, `.fledge/molt/` evidence).
Read this when: writing or extending a test, deciding which existing test file already covers behavior you're about to change, or verifying you haven't broken an acceptance-test fixture that asserts on scaffolded output.

## domain.md
Glossary of the bird-themed vocabulary (plumage, feather, brood, preen, molt, vee, colony, forager, scout, brooder, skua, species) and the orchestration concepts (primitive, tier, harness, adapter, manifest, core skill) with grounding references — resolves the "what is a skua" ambiguity left open by an individual scout report.
Read this when: you hit unfamiliar terminology anywhere in this repo's code, specs, or skill prose and need a precise definition rather than inferring from context.
