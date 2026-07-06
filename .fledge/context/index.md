---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Context Index

Generated context for fledge — a single-binary Go CLI for spec-driven development that manages REQ/TASK markdown specs under `.fledge/`. Load docs below based on the `Read this when` routing lines.

## architecture.md
Describes the three-tier layering (cmd entry → internal/cli command layer → focused core packages), the dependency direction, and cross-cutting design principles: determinism/byte-preservation, atomic mutation, lock/status consistency invariants, findings-vs-errors, and how the `.claude/` agent layer sits above the CLI.
Read this when: you need the big-picture shape, how packages relate, or the rationale behind determinism/atomicity/consistency decisions before changing cross-package behavior.

## modules.md
Per-module map of the repo: `root`, `cmd`, and each `internal/*` sub-package (cli, spec, check, graph, lock, repo, scan) with purpose, key files, and a "Look here for" pointer.
Read this when: you need to locate which file or package owns a given concern.

## conventions.md
The codebase's operative patterns: one-command-per-file, fixed-key-order frontmatter and canonical quoting, ID/filename formats, the enum sets (status/priority P0–P3/oversight) and their transition rules, field mutability, findings-vs-errors, atomic writes, exit-code taxonomy, dual text/JSON output, and test conventions.
Read this when: you are writing or reviewing code and need to match existing style, enum values, transition rules, or error/output conventions.

## data-model.md
Authoritative field-by-field definitions of the core types — `Requirement`, `Task`, `Set`/`FileError`, `check.Finding`, `graph.Graph`, `lock.Record`, `scan.Module`/`Result` — plus the status/priority/oversight constants and frontmatter key order.
Read this when: you need exact struct fields, valid enum values, or the frontmatter schema for specs.

## dependencies.md
Direct/indirect third-party deps (goccy/go-yaml for frontmatter parsing, go-internal/testscript for e2e tests), the runtime dependency on git via os/exec, notable stdlib usage per package, the internal import graph (no cycles), and AGPL v3 licensing.
Read this when: you need to know what a package may import, why a dependency exists, or the runtime git assumption.

## entry-points.md
Process entry (`main` → `cli.Run`), build/run/test commands and version stamping, the 0/1/2/3 exit-code taxonomy, the full subcommand table with args and flags, the library public interfaces, and graph output formats.
Read this when: you need the exact command surface, flags, exit codes, or how to build/run/test the binary.

## testing.md
The two test layers — testscript/txtar e2e scripts in `cmd/fledge/testdata/` (one per command + e2e) and per-package unit tests — with what each covers, the git-determinism setup, the test-first convention, and known coverage gaps (internal/repo untested).
Read this when: you are adding or debugging tests, or need to know how a behavior is currently verified.

## domain.md
Glossary of the problem-domain vocabulary: spec-driven development, Requirement/Task and their lifecycles, dependency graph, waves, ready/blocked, locks, oversight, findings, priority, frontmatter, body preservation, scan modules, scan-ignore, and agent.
Read this when: you encounter an unfamiliar term or need precise definitions of fledge's spec/workflow concepts.
