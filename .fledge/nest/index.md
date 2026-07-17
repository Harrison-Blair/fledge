---
generated: 2026-07-17T17:48:26Z
commit: 1c9011d6e6a06f72f96bc98e3b2bd99c408ab79e
agent: fledge-forager
fledge_version: 0.6.10
---

# Context Index

## architecture.md
Two-layer architecture (deterministic `internal/cli` command layer + `internal/bootstrap` scaffolding/adapter system), the 6-primitive/tier-derivation model, and the orchestration-workflow prose layer that both layers exist to serve. Includes cross-module relationships (e.g. how ledger, spec, and nest packages are shared between the CLI and the workflow protocols).
Read this when: you need the big picture before touching bootstrap/adapter code, changing how a harness is scaffolded, or understanding how the CLI and the workflow prose relate.

## modules.md
Repo map: every top-level module from `fledge scan` (cmd, internal/cli, internal/bootstrap, internal/spec, the small support packages, root, .agents, .codex, .github, docs, scripts) with purpose, key files, and "look here for" pointers.
Read this when: you know roughly what you want to change but not which file/package owns it.

## conventions.md
Reconciled coding conventions (command registration, exit codes, atomicity, flock patterns), spec-lifecycle/CLI-only-mutation rules, ID/naming rules, bootstrap/scaffold policies, and worker-coordination discipline (ledger-over-messages, heartbeat, scope discipline, test-first) pulled from both the Go source and the workflow prose.
Read this when: writing or reviewing any change — CLI command, spec mutation, scaffold policy, or worker protocol — and you need to match existing house style.

## data-model.md
Every persisted/in-memory type: spec `Requirement`/`Task`/`Criterion`, ledger `StatusRecord`/`VerdictRecord`/`EscalationRecord`, lock `Record`, roster `Entry`, nest `Doc`/`StatusResult`, bootstrap `Manifest`/`Stamp`/`Drift`, evidence-file structure, and CLI JSON output structs.
Read this when: you need exact field names/types for a struct, frontmatter schema, or a ledger/lock/stamp file format.

## dependencies.md
The full go.mod dependency list (goccy/go-yaml, rogpeppe/go-internal/testscript, golang.org/x/{sys,tools}) with usage notes, plus notable stdlib usage (flag, syscall/flock, sha256, text/template) and external services (GitHub Actions, GitHub Releases API).
Read this when: adding a new dependency, checking what's already available, or tracing what a package import is actually used for.

## entry-points.md
Build/install/test commands, the binary's `main.go`, the full 26-command CLI surface, the workflow's phase entry points (SKILL.md routing → planning/foraging/implementation), and harness-specific entry files per adapter.
Read this when: running or building fledge, adding a new CLI command, or figuring out where an agent-facing protocol actually starts.

## testing.md
Test frameworks (stdlib `testing` + `testscript` only), how to run any test subset, an inventory of the 36 txtar acceptance fixtures, per-package unit-test counts/coverage patterns, and the three-layer coverage philosophy (unit / acceptance / prose-and-config invariant tests) unique to this repo's dogfooding structure.
Read this when: writing a new test, deciding which layer a test belongs in, or running a specific existing test.

## domain.md
Full bird-themed glossary — spec artifacts (plumage, feather, FC/AC), repo-knowledge artifacts (nest, concern doc, scout report), coordination artifacts (ledger, brood, roster, molt, verdict, escalation), spawned-worker roles (incubator, forager, scout, brooder, skua), and the primitive/tier/adapter vocabulary.
Read this when: you hit an unfamiliar term anywhere in this repo's code, prose, or commit history and need its precise meaning.

## Open Questions carried into this run

- `docs/google_ai_mode_response.md` / `docs/research_prompt.md` (multi-tier AI infrastructure research) don't map onto fledge's core domain — purpose unclear (see domain.md, architecture.md).
- The exact non-dev-mode relationship between `.agents/skills/` and `.fledge/skills/` wasn't confirmed by any assigned source file (see architecture.md).
