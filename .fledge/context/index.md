---
generated: 2026-07-06T21:54:21Z
commit: 22c11810cf8ab8d8e8ae34253a6426af005561c2
agent: context-gatherer
fledge_version: 0.1.0
---

# Context Index

## architecture.md
The repo is pre-implementation: one metadata-only `root` module and the `.fledge/` context-generation scaffold (scan script → raw scout reports → concern docs). Describes which `.fledge/` paths are ephemeral vs. committed. No cross-module relationships exist.
Read this when: you need the big picture of what exists in this repo, or how the `.fledge/` context pipeline is laid out, before planning any new structure.

## modules.md
Repo map with a single entry, `root`: README (name + tagline only), AGPL-3.0 LICENSE, `VERSION` = 0.1.0, and a `.gitignore` covering `.fledge/` intermediates.
Read this when: you need to locate a specific file or confirm what modules exist before assigning work or adding a new top-level directory.

## conventions.md
Bare-semver `VERSION` file, git-ignoring of `.fledge/context/raw/` and `.fledge/locks/` as regenerable intermediates, minimal lowercase README style, and AGPL-3.0 licensing. Notes the unfilled LICENSE copyright line.
Read this when: writing anything that must match existing repo hygiene — version bumps, `.gitignore` changes, licensing headers — or establishing coding conventions for the first code.

## data-model.md
Empty by fact: no types, schemas, or tables exist; the only structured value is the semver string in `VERSION`.
Read this when: you would otherwise search for existing types to reuse — this doc confirms there are none, so design from scratch.

## dependencies.md
Empty by fact: no manifests or lockfiles, so no third-party dependencies and no committed language/toolchain choice.
Read this when: deciding on a language or adding the first dependency — nothing constrains that choice yet.

## entry-points.md
No product entry points, build steps, or usage docs exist; the only executable is the context-tooling helper `.fledge/scripts/scan`.
Read this when: you need to run or build the project (you cannot yet) or are about to create its first CLI/entry point.

## testing.md
Empty by fact: no tests, frameworks, or runners exist; testing conventions are unestablished.
Read this when: adding the first tests or defining the test strategy — there is no prior pattern to follow.

## domain.md
Four-term glossary: fledge, spec-driven development, per-run intermediates, and context (the `.fledge/context/` doc set).
Read this when: you encounter fledge-specific vocabulary in specs or prompts and need its meaning in this repo.
