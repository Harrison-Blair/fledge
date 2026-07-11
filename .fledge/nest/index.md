---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Context Index

## architecture.md
Describes the two-layer split — the deterministic `internal/cli` command dispatcher vs. the `internal/bootstrap` scaffolding/adapter engine (embed, manifest, 6 primitives, tier derivation, stamp/drift) — and how the two layers and the domain packages under `internal/` relate to each other. Also notes this repo's own dogfooding setup (symlinked `.claude/agents/`).
Read this when: you need to understand how a change ripples across CLI ↔ bootstrap ↔ domain packages, or how the adapter/primitive/tier system fits together before touching `fledge init` or scaffolding behavior.

## modules.md
A repo map — one entry per top-level module (`<root>`, `cmd`, `docs`, `internal` broken into its 9 packages, `pluma`, `scripts`) — with purpose, key files, and a "Look here for" pointer per module.
Read this when: you're orienting in an unfamiliar part of the repo and need to know which module/package owns a given concern before diving into files.

## conventions.md
Reconciled coding/process conventions: bird-themed naming, the CLI's self-registering command pattern, atomic-write and byte-preservation discipline, YAML frontmatter safety rules, spec lifecycle/governance rules, and scaffold write-policy classification order.
Read this when: writing new code in this repo and you want to match existing idiom (error handling, naming, file-write safety, flag conventions) rather than introduce a new pattern.

## data-model.md
Every core Go type across the codebase (`Requirement`, `Task`, `Set`, `Graph`, `Record`, `Doc`, `Manifest`, `Stamp`, `Drift`, CLI report structs), organized by the domain they model (spec, dependency/lock, nest, validation/scan, bootstrap).
Read this when: you need to know a struct's exact fields before writing code that constructs, serializes, or pattern-matches on it.

## dependencies.md
The full external dependency list: two Go module deps (`goccy/go-yaml`, `rogpeppe/go-internal`'s testscript) with usage notes, key stdlib packages by concern, the `git` subprocess dependency, and a callout that the AI-infrastructure-tiering ideas in `docs/` are not wired into any code.
Read this when: adding a new dependency (check if something already covers the need) or auditing what fledge actually requires to build and run.

## entry-points.md
How to build, install, and run fledge: `scripts/install.sh`, `main()` → `cli.Run()`, the 17-command CLI surface (`commandOrder`), `nest`'s subcommands, and the exact `go test`/`go vet` invocations including single-test forms.
Read this when: you need the exact command to build, install, or invoke one part of the CLI or test suite, or you're tracing execution from `main()` inward.

## testing.md
Frameworks (Go `testing`, testscript/txtar) and how to run them, a description of what each of the 21 CLI acceptance tests covers, and a per-package rundown of unit test files and case counts across `internal/spec`, `internal/bootstrap`, `internal/check`, `internal/graph`, `internal/lock`, `internal/nest`, `internal/scan`, `internal/cli`.
Read this when: adding or changing behavior and you need to know which existing test(s) already cover it, or how to write a new acceptance test in txtar form.

## domain.md
The full bird-themed glossary — spec artifacts (plumage, feather, criteria, frontmatter), work lifecycle (brood, ready, wave, colony, orphan, molt), repo-knowledge/orchestration terms (nest, concern doc, scout, forager, brooder, skua, incubator), and scaffolding terms (scaffold, stamp, drift, adapter, primitive, tier, write policy).
Read this when: you hit an unfamiliar term in code, a spec, or a teammate message and need its precise fledge-specific meaning.

## Open Questions carried forward

See each concern doc's own `## Open Questions` section for specifics. Notably unresolved across this pass: exact Claude/Codex/Cursor/opencode skill-discovery mechanics (`conventions.md`), cross-version scaffold migration behavior (`architecture.md`), adoption status of the AI-infra-tiering proposal in `docs/` (`dependencies.md`), and the canonical source of the `agent` value stamped into nest docs (`domain.md`).
