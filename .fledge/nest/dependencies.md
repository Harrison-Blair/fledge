---
generated: 2026-07-11T01:58:32Z
commit: 96a3ac38bc843217824d6d6886c49906053bf686
agent: fledge-forager
fledge_version: 0.3.4
---

# Dependencies

External dependencies used by fledge, deduplicated across modules with usage notes.

## Go module dependencies (`go.mod`)

- **`github.com/goccy/go-yaml` v1.19.2** — YAML parsing/unmarshaling for spec frontmatter (`internal/spec/frontmatter.go`) and for adapter `manifest.yaml` files (`internal/bootstrap/registry.go`, validated in `registry_test.go`), and for `internal/nest`'s `RefreshDoc()` frontmatter rewriting.
- **`github.com/rogpeppe/go-internal` v1.15.0** — specifically its `testscript` subpackage; drives the entire CLI acceptance-test suite (`cmd/fledge/main_test.go:TestScripts`) against `.txtar` fixtures in `cmd/fledge/testdata/`.

## Go standard library (representative, not exhaustive)

- `flag`, `os`, `path/filepath`, `strings`, `bufio`, `syscall`, `time`, `sort`, `slices`, `encoding/json` — CLI arg parsing and I/O (`internal/cli`).
- `crypto/sha256`, `io/fs`, `text/template`, `bytes` — scaffold hashing and template rendering (`internal/bootstrap`).
- `regexp`, `embed`, `errors`, `sync` — validation regex, embedded templates, misc (`internal/check`, `internal/nest`).

## External tools invoked as subprocesses

- **`git`** — `git rev-parse` (repo root detection, `internal/repo/repo.go:Find`; HEAD sha, `Repo.Head()`), `git ls-files`/`git check-ignore` (file inventory and `.fledgeignore` filtering, `internal/scan/scan.go`).

## Runtime/toolchain

- **Go 1.26.4** (or later compatible) — language runtime; no Makefile, build via `go build`/`go install` directly.

## Documented-but-not-yet-adopted dependencies (from `docs/`)

These appear only in planning documents, not in current Go code — do not treat as active dependencies:
- Agent Skills standard (load mechanism shared by Claude Code, pi, Codex) — referenced in `docs/generalization-plan.md` as the mechanism multi-harness skill loading relies on.
- OpenCode Go / OpenCode Zen APIs, local inference servers (vLLM, SGLang, Ollama) — proposed in `docs/google_ai_mode_response.md`; unclear adoption status (see Open Questions).

## Open Questions

- Whether the infrastructure-tiering proposal in `docs/google_ai_mode_response.md` (OpenCode Go/Zen, local inference) is adopted, exploratory, or rejected — no code in `internal/` references any of it, and no other scouted file links it to the fledge roadmap.
