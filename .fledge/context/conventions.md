---
generated: 2026-07-06T23:33:05Z
commit: b701cf5a12a99b5adf9538e83f51178d4dead0c2
agent: fledge-context-gatherer
fledge_version: 0.1.0
---

# Conventions

Patterns observed across the fledge codebase. These are descriptive of the current code, reconciled across modules.

## Code organization
- **One command per file** in `internal/cli` (`init.go`, `new.go`, …), each registered into a shared registry at init time (`internal/cli/cli.go:register`). Command display order is fixed by `commandOrder`.
- **One concern per package** under `internal/`; core packages (`spec`, `lock`, `repo`, `scan`) depend only on the standard library where possible, keeping the domain model dependency-light.
- **Thin entry point:** `cmd/fledge/main.go` contains no logic beyond delegating to `internal/cli`.

## Determinism & byte preservation
- **Frontmatter rendered in fixed key order** per spec type, with canonical scalar quoting decided by `yamlScalar` (`internal/spec/frontmatter.go`): plain alphanumerics plus space/dot/slash/parens/hyphen stay unquoted; booleans, numbers, reserved words, and leading/trailing-space values are quoted.
- **Markdown body is never rewritten.** `SplitFrontmatter` returns the post-`---` body byte-for-byte; round-trips (`TestTaskRoundTrip`) and internal `---` lines in the body are preserved. `fledge set`/`status` only rewrite the frontmatter block.
- **`scan` output is byte-compatible** with the retired `.fledge/scripts/scan` bash implementation (noted in `internal/cli/scan.go`).

## Identifiers & filenames
- **IDs:** `PREFIX-NNN` — `REQ-001`, `TASK-042`. Minimum 3-digit zero padding; if any existing ID in the directory is wider, new IDs match that width (`internal/spec/ids.go:NextID`). Allocation is max-existing + 1 (gaps are not backfilled).
- **Filenames:** `{ID}-{slug}.md` where slug is `Kebab(title)` — unicode-aware lowercase, runs of non-alphanumerics collapsed to a single hyphen, trailing hyphens trimmed (`internal/spec/ids.go:Kebab`). Validation checks the filename prefix matches the ID; renaming title via `fledge set` intentionally does NOT rename the file (leaves a mismatch warning surface).

## Enums (canonical source: `internal/spec/types.go`)
- **Requirement status:** `draft` → `approved` → `done` (transitions: draft→approved, approved→done|draft).
- **Task status:** `blocked`, `ready`, `in-progress`, `done` (transitions: ready→in-progress, blocked→in-progress, in-progress→done|ready). Enforced by `internal/cli/status.go` unless `--force`.
- **Priority:** `P0`–`P3` only (`Priorities` slice). Out-of-range values such as `P9` are rejected (`cmd/fledge/testdata/new.txtar:34`).
- **Oversight:** `merge` or `during`; empty when unset (omitted from frontmatter when empty).

## Field mutability
- **Immutable after creation:** `id`, `requirement`, `authored`, `agent`, `fledge_version`. `fledge set` rejects these.
- **Mutable via `fledge set`:** `priority`, `oversight`, `depends_on` (cycle-checked), `title` (updates frontmatter and the `# ID: <title>` heading).

## Error handling & I/O
- **Validation uses findings, not errors:** `check.Run` returns `[]check.Finding{File, Rule, Severity, Message}`; `Severity` is `Error` or `Warning`. Go `error`s are reserved for I/O and environment problems.
- **Atomic writes everywhere:** temp file in the same directory → `chmod 0o644` → atomic rename, with temp cleanup on failure (`spec.WriteFileAtomic`). Locks use `O_EXCL` create so exactly one acquirer wins a race.
- **Graceful missing state:** missing spec directories load as an empty `Set` (no error); missing `scan-ignore` means no filtering; missing VERSION falls back; no git commits yields `ShortCommit == "none"`; missing lock dir lists empty.
- **Exit codes** are a deliberate taxonomy (`internal/cli/cli.go`): 0 OK, 1 domain failure, 2 usage error, 3 environment error. See `entry-points.md`.

## Output
- **Dual output:** most commands accept `--json` for structured output alongside the default human-readable text; JSON is emitted via a shared `emitJSON` helper with indentation.
- **Sorted output:** graph node lists, `ready`, `locks`, and scan modules are sorted (by ID or name) for stable output.

## Testing conventions
- **Two-layer testing:** black-box testscript/txtar suites in `cmd/fledge/testdata/` drive the whole binary; unit tests live beside each core package. See `testing.md`.
- **Git determinism in tests:** `main_test.go` pins `GIT_CONFIG_GLOBAL`/`SYSTEM` to `/dev/null` and locks author/committer identity to `test`/`test@example.invalid`.
- **Test-first discipline is baked into the task template:** the task body scaffold mandates AC-1 = "tests observed failing before implementation and passing after" (`internal/spec/templates/task.md`).

## Metadata
- **Frontmatter block** starts every generated markdown file: `id/title/status/...` for specs; `generated/commit/agent/fledge_version` for context docs.
- **Timestamps** are RFC 3339 / ISO 8601 UTC (validated by `check.checkAuthored`).
- **Agent field** records the authoring agent, e.g. `fledge-orchestrate/planning` (default for new requirements).
