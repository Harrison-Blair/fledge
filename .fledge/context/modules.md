---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Modules

Repository map organized by module (top-level directory). Use the "Look here for" lines to route to the right package.

## root
- **Purpose:** Project metadata and Go module definition.
- **Key files:** `go.mod` (module `github.com/Harrison-Blair/fledge`, `go 1.26.4`), `VERSION` (`0.1.0`), `README.md` (one-line description), `LICENSE` (AGPL v3), `.gitignore`.
- **Look here for:** the module path, declared Go/dependency versions, license, and the ignore rules for the built `/fledge` binary and per-run intermediates.

## cmd
- **Purpose:** CLI binary entry point and the end-to-end (testscript/txtar) test suite that exercises every subcommand.
- **Key files:** `cmd/fledge/main.go` (thin `cli.Run` bootstrap), `cmd/fledge/main_test.go` (testscript runner with locked git identity), `cmd/fledge/testdata/*.txtar` (10 scripts: `init`, `new`, `check`, `graph`, `ready`, `lock`, `status`, `set`, `scan`, `e2e`).
- **Look here for:** how the binary is launched, and the authoritative behavioral contract of each command expressed as black-box CLI scripts.

## internal
The library behind the CLI. Split below into the sub-packages that matter (scouted as `internal-cli`, `internal-spec`, `internal-core`).

### internal/cli — command layer
- **Purpose:** Implements every subcommand, dispatch, flag parsing, output formatting, and exit codes.
- **Key files:** `cli.go` (registry, `Run`, exit-code constants, usage), one file per command (`init.go`, `scan.go`, `new.go`, `check.go`, `ready.go`, `graph.go`, `status.go`, `set.go`, `lock.go`, `version.go`), `specload.go` (shared `loadSet`/path helpers), `scan-ignore.default` (embedded default ignore file).
- **Look here for:** what a command does, its flags, status-transition rules, JSON output shapes, and how lock/status consistency is enforced.

### internal/spec — data model
- **Purpose:** Parse, validate, scaffold, and byte-preservingly serialize REQ/TASK spec files.
- **Key files:** `types.go` (`Requirement`, `Task`, status/priority/oversight constants), `frontmatter.go` (split/parse/render/atomic-save), `ids.go` (`NextID`, `Kebab`), `load.go` (`Load` → `Set`, lookups), `templates.go` + `templates/{requirement,task}.md` (embedded scaffolds).
- **Look here for:** the canonical struct definitions, frontmatter schema and key order, ID/filename format, and the requirement/task body templates.

### internal/check — validation engine
- **Purpose:** Run all spec-integrity rules over a loaded set.
- **Key files:** `check.go` (`Run` → `[]Finding`, `HasErrors`).
- **Look here for:** every validation rule (dangling refs, duplicate IDs, cycles, missing sections, ID/filename mismatch, unapproved-requirement links, stale-ready hints, lock consistency) and its severity.

### internal/graph — dependency DAG
- **Purpose:** Compute cycles, waves, and ready sets over task `depends_on` edges.
- **Key files:** `graph.go` (`New`, `Cycle`, `Waves`, `Ready`).
- **Look here for:** cycle detection, topological wave layout, and ready-task computation semantics (including dangling-dep handling).

### internal/lock — task locks
- **Purpose:** Advisory, atomic per-task claim files under `.fledge/locks/`.
- **Key files:** `lock.go` (`Acquire`, `Release`, `Get`, `List`, `Record`, `HeldError`).
- **Look here for:** lock file format, acquisition/contention semantics, and holder metadata.

### internal/repo — repo discovery
- **Purpose:** Locate the repo via git and resolve fledge paths.
- **Key files:** `repo.go` (`Find`, `Repo` path methods for `.fledge/`, spec dirs, VERSION, HEAD).
- **Look here for:** how the repo root and all fledge-relative paths are computed (only package here with no test file).

### internal/scan — file inventory
- **Purpose:** List tracked+untracked files, filter via scan-ignore, group into modules.
- **Key files:** `scan.go` (`Run` → `Result{Commit, ShortCommit, Modules}`, `Module{Name, Files, Count, Bytes}`).
- **Look here for:** how context modules are derived and how `.fledge/scan-ignore` filtering and commit stamping work.
