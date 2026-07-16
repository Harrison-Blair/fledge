---
generated: 2026-07-15T23:53:12Z
commit: a4d02e8187c64ef9f3f1231052990b282207420b
agent: fledge-forager
fledge_version: 0.5.5
---

# Dependencies

External libraries, tools, and services the codebase relies on, deduplicated across modules with usage notes.

## Go module dependencies (`go.mod`)

- **`github.com/goccy/go-yaml` v1.19.2** — YAML parsing/marshaling. Used for: spec frontmatter (`internal/spec/frontmatter.go`), adapter manifests (`internal/bootstrap/registry.go`, `LoadAdapters`), `.fledge/scaffold.json`-adjacent config, and CI-workflow-YAML parsing in the `internal/ciconfig` tests.
- **`github.com/rogpeppe/go-internal` v1.15.0** — provides `testscript`, the `.txtar`-driven CLI acceptance test framework used by `cmd/fledge/main_test.go` and all 23 fixtures in `cmd/fledge/testdata/`.
- **`golang.org/x/sys` v0.26.0** — indirect (transitive dependency, likely via testscript or stdlib extensions).
- **`golang.org/x/tools` v0.26.0** — indirect (transitive dependency, likely via testscript).

## Go standard library (notable usage by package)

- **`embed`** — `internal/bootstrap/bootstrap.go` (`//go:embed core adapters`), `internal/nest` (templates), `internal/spec/templates.go` (plumage/feather skeletons).
- **`text/template`** — `internal/bootstrap/registry.go` `renderEntry()`, for `generate`/`primitive_map` write policies (renders `fledge-adapter.md` with `.Tier`, `.Rows`, `.Provided`, etc.).
- **`crypto/sha256`** — `internal/bootstrap` drift/stamp content hashing (`DriftReport`, `ExpectedFiles`, `classifyContent`).
- **`encoding/json`** — `Stamp` marshal/unmarshal (`internal/bootstrap/stamp.go`), `lock.Record` serialization, all `--json` CLI output.
- **`os.Link`** — `internal/lock/lock.go` `Acquire()`, giving atomic exclusive-create semantics for `.brood` claim files (O_EXCL equivalent).
- **`syscall`** (flock) — `internal/spec/ids.go` `AllocateAndCreate`, serializing concurrent ID allocation via a `.alloc.lock` dotfile.
- **`os/exec`** — `internal/repo/repo.go` (`git rev-parse --show-toplevel`), `internal/scan/scan.go` (`git ls-files`, `git check-ignore`), `internal/hooktest/precommit_test.go` (git operations in temp repos), `internal/cli/update.go` (external tooling during self-update).
- **`archive/tar`, `archive/zip`, `compress/gzip`** — `internal/cli/update.go`, unpacking downloaded release archives during `fledge update`.
- **`net/http`** — `internal/cli/update.go`, calling the GitHub Releases API.

## External CLI tools

- **`git`** — pervasive: repo-root detection, file enumeration (`ls-files`), `.fledgeignore` filtering (`check-ignore`), evidence/commit history lookups (`git log`), worktree management during implementation (`git worktree add`), release-version diffing (`git show HEAD^:VERSION`).
- **`gofmt` / `go vet` / `go build` / `go test`** — the lint/build/test gate, run identically in `.github/workflows/pr-check.yml`, `.github/workflows/release.yml`'s `safety-net` job, and the optional local `scripts/hooks/pre-commit` (kept in sync; asserted by `internal/hooktest`).
- **`gh` (GitHub CLI)** — `.github/workflows/release.yml`, `gh release create` using the implicit `github.token`.
- **`tar`, `sha256sum`** — release artifact packaging/checksumming in `release.yml`.

## GitHub Actions

- `actions/checkout@v4`, `actions/setup-go@v5` (Go version from `go.mod`), `actions/upload-artifact@v4`, `actions/download-artifact@v4` — used in the 4-platform release build/merge pipeline.

## Runtime/config file dependencies

- **`VERSION`** (repo root) — read by `release.yml` (`detect-version` job), `scripts/install.sh`, and compared against `internal/cli/version.go`'s `binaryVersion` (injected via `-ldflags`).
- **`.fledgeignore`** — consulted by `internal/scan` (via `git check-ignore`) to filter `fledge scan` output; a default pattern set ships embedded as `internal/cli/fledgeignore.default`.
- **`.fledge/scaffold.json`** — read/written by `internal/bootstrap` (`LoadStamp`, `Stamp.Write`); records which files fledge owns and at what content hash, consumed by `fledge preen` for drift detection.

## Harness/runtime dependencies (adapter-specific, not Go deps)

- **Claude Code**: `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`, `teammateMode: tmux` (`internal/bootstrap/adapters/claude/settings.json`); tmux for Tier C team-loop pane management.
- **Codex / pi**: no team-loop dependency (Tier A, solo execution); pi points at `.fledge/skills` via its own `settings.json`.

## Open Questions

- Whether `golang.org/x/sys` and `golang.org/x/tools` (both indirect) are pulled in by `testscript` specifically or by another transitive path — not resolvable from `go.mod` alone without `go mod graph`.
