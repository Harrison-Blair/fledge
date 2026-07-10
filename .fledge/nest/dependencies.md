---
generated: 2026-07-10T20:53:53Z
commit: f28efebd76d6aa135adb0956a3337a40a8d98351
agent: fledge-forager
fledge_version: 0.3.0
---

# Dependencies

External (non-stdlib) dependencies actually used by the fledge codebase, deduplicated across modules, with usage notes. (`go.mod` at repo root is the source of truth for versions.)

## Direct dependencies

- **`github.com/goccy/go-yaml v1.19.2`** — YAML parsing/marshaling. Used in two places: `internal/spec/frontmatter.go` (plumage/feather frontmatter parse+render, canonical key order, `YAMLScalar` quoting rules) and `internal/bootstrap/registry.go` (parsing each adapter's `manifest.yaml`).
- **`github.com/rogpeppe/go-internal v1.15.0`** (specifically its `testscript` subpackage) — the txtar-based CLI acceptance test framework. Used exclusively in `cmd/fledge/main_test.go` (`TestMain` registers the `fledge` command as a testscript command; `TestScripts` runs every `cmd/fledge/testdata/*.txtar` fixture). Not used anywhere in production code.

## Indirect dependencies

- **`golang.org/x/sys v0.26.0`** — transitive, system interface calls (pulled in by one of the above).
- **`golang.org/x/tools v0.26.0`** — transitive, tooling support.

## Standard library (heavy use, worth noting)

- **`embed`** — `internal/bootstrap/bootstrap.go` embeds the entire `core/` and `adapters/` trees at compile time (`//go:embed core adapters`); this is how `fledge init` scaffolds without touching the filesystem for its source content.
- **`crypto/sha256`** — content hashing for `.fledge/scaffold.json` stamp entries, drift detection.
- **`encoding/json`** — all `--json` CLI output (`emitJSON()`), brood `Record` marshaling, stamp serialization (`json.MarshalIndent` with sorted keys for determinism).
- **`os/exec`** — shells out to `git` for repository operations: `git rev-parse --show-toplevel` (repo discovery, `internal/repo/repo.go:Find`), `git ls-files` (tracked+untracked file listing, `internal/scan/scan.go`, `-z` null-delimited), `git check-ignore` (`.fledgeignore` filtering, respects `core.excludesFile`), `git rev-parse HEAD` (commit SHA for nest frontmatter).
- **`text/template`** — renders `generate`/`primitive_map` manifest files (e.g. each harness's `fledge-adapter.md` primitive-map table) in `internal/bootstrap/registry.go`.
- **`flag`** — every CLI command builds its own `flag.FlagSet(name, flag.ContinueOnError)`.
- **`testing`** — all unit tests; no third-party assertion library (no testify) — plain `t.Errorf`/`t.Fatal`, table-driven style throughout.

## Runtime/environment dependencies

- **git** — required at runtime for `fledge scan`, `fledge repo` discovery, and `fledge version`'s commit reporting; not vendored, invoked via `os/exec`.
- **tmux** — referenced in `internal/bootstrap/adapters/claude/team-loop.md` as part of Tier C's harness piping (teammate display); a Claude Code runtime concern, not a Go build dependency.

## Notes on `docs/` — not real dependencies of this codebase

`docs/google_ai_mode_response.md` references external services/models (OpenCode Go/Zen, DeepSeek V4-Pro, Claude Sonnet, GLM-5.2, etc.) and a Python `requests`-based sketch — these are NOT dependencies of the fledge Go codebase; the file is an unrelated infrastructure-design exploration with no evident connection to fledge's implementation (see Open Questions in `architecture.md`).

## Open Questions

None survive synthesis — dependency set is small and unambiguous across scout reports.
</content>
