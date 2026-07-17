---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Architecture

Two deliberately separated layers — a deterministic CLI and an agent-neutral orchestration/bootstrap system — plus three new subsystems (ledger, dev-link, drift) added since the last regeneration.

## Layer 1: the CLI (`internal/cli` + domain packages)

`cmd/fledge/main.go` is a single-line entrypoint that delegates to `internal/cli.Run(os.Args[1:])` (`cmd/fledge/main.go`). `internal/cli/cli.go` holds the command registry: 25 registered subcommands (`internal/cli/cli.go` — init, agents, scan, new, nest, preen, ready, vee, colony, unfledged, status, set, criteria, brood, abandon, broods, heartbeat, await, verdict, escalate, ledger, roster, version, update, dev), each file's `init()` calling `register(name, runFunc, usage)`; `commandOrder` drives both usage text and generated allow-lists. Dispatch is synchronous; the only process-level outcome is the exit code.

**Exit codes** (`internal/cli/cli.go`): `ExitOK=0`, `ExitFail=1`, `ExitUsage=2`, `ExitEnv=3`, and — new since the last index — `ExitTimeout=4`, returned only by `await` on timeout elapse. Any doc or memory that lists just 0–3 is stale as of this regeneration.

Domain logic is split into focused packages, each with one job:
- `internal/spec` — frontmatter parsing/rendering, ID allocation (flock-serialized), byte-preserved body, acceptance-criteria checkbox parsing/flipping (`internal/spec/frontmatter.go`, `ids.go`, `criteria.go`).
- `internal/check` — spec validation = `fledge preen` (`internal/check/check.go`: `Run`, `Finding`).
- `internal/graph` — dependency graph = `fledge vee` (`internal/graph/graph.go`: `Cycle`, `Waves`, `Ready`).
- `internal/lock` — feather claims = `fledge brood` (`internal/lock/lock.go`: exclusive `os.Link`-based acquisition).
- `internal/scan`, `internal/repo` — module scanning (`fledge scan`) and git/`.fledge` path discovery.
- `internal/roster` — worker species token allocation (`internal/roster/roster.go`).
- `internal/nest` — concern-doc/scout-report scaffolding and synthesis-completion checking (`internal/nest/nest.go`, `docs.go`).
- `internal/ledger` — new: agent handoff records (see below).

Every command supports `--json`; the pattern (`internal/cli/*.go`) is: flag parsing via `flag.FlagSet(ContinueOnError)` → `repo.Find()` + `r.RequireFledge()` → `loadSet()` (repo + specs + locked IDs) → business logic → `envErr()`/`fail()`/`usageErr()` → `emitJSON()` or human text.

## Layer 2: bootstrap/adapter system (`internal/bootstrap`)

What `fledge init` scaffolds. `internal/bootstrap/bootstrap.go` embeds two trees via `//go:embed core adapters`.

- **`core/skills/`** — the single agent-neutral source of the orchestration workflow: `fledge-orchestrate` (SKILL.md, planning.md, implementation.md, foraging.md, incubator.md, brooder.md, skua.md, worker-protocols.md, templates/) and `fledge-interrogate`. Written to a repo's `.fledge/skills/` by `WriteCore` (`internal/bootstrap/registry.go`).
- **`adapters/<harness>/`** — thin, format-only per-harness mapping, driven entirely by `manifest.yaml`: detector, `tier_primitives` map, file list with write policies. Three adapters ship: `claude` (Tier C, 6 primitives), `pi` (Tier A, 4 primitives), `codex` (Tier A, 4 primitives) (`internal/bootstrap/adapters/*/manifest.yaml`). Adding/changing a harness is a manifest edit — zero Go code.
- **The 6 primitives** (`internal/bootstrap/primitives.go`, `PrimitiveOrder`): `confirm-gate`, `read-only-shell`, `write-file`, `run-fledge`, `spawn-worker`, `message-peer`. An adapter's tier (A/B/C) is *derived* from its declared primitive coverage via `DeriveTier`, never declared directly — Tier A = first 4 primitives, Tier B = +spawn-worker, Tier C = +message-peer.
- **`stamp.go`** — `Stamp`/`StampEntry` record what `init` wrote (content hashes, symlink targets, required append-lines, fledge version, scaffolded agents) to `.fledge/scaffold.json`. `fledge preen` validates its presence/consistency.
- **`drift.go`** — `DriftReport` classifies every scaffold-owned file against 5 statuses: up-to-date, stale (binary moved), modified (user edited), missing, obsolete (no longer shipped). Dev-link-aware: a dev-linked file's drift compares symlink target to the stamped target, not file content.

This repo dogfoods itself: it has its own `.fledge/` with `.fledge/pluma/` specs, `.fledge/broods/` claims, and a `.fledge/skills/` copy that — per commit `1f5224d` — is untracked and **dev-linked** into `internal/bootstrap/core/skills/` rather than a plain copy (confirmed via `internal/bootstrap/registry.go`'s dev-link write path and this repo's expanded `.gitignore`, `root.md` raw report). Editing `internal/bootstrap/core/skills/...` is editing this repo's live `.fledge/skills/...` directly — the scaffolded copy is generated output, never the source of truth, but here it's also *symlinked*, not just regenerated.

## New since the last index: the PLM-030 handoff ledger

`internal/ledger/ledger.go` (new package) persists three record kinds under `.fledge/ledger/<subject>.<kind>.json`, written atomically (temp-then-rename), latest-value-only: `KindStatus` (heartbeat liveness, PID + note + timestamp, 5-minute `StaleAfter` TTL), `KindVerdict` (pass/fail + note, write-once per review), `KindEscalation` (free-text blocker message, write-once). Five new CLI commands read/write it: `fledge heartbeat`, `await`, `verdict`, `escalate`, `ledger read` (`internal/cli/heartbeat.go`, `await.go`, `verdict.go`, `escalate.go`, `ledger.go`).

`fledge await`'s wait contract (`internal/cli/await.go`) is precise and kind-dependent:
- **Change-wait** (default; used for the repeatedly-written `status` kind): baseline-sampled at call time, returns when the record first appears or its payload diverges from the baseline.
- **Existence-wait** (`--exists`; used for the write-once `verdict`/`escalation` kinds): no baseline sampling — returns on the first read where the record is present, immune to the baseline race and to identical-payload rewrites.
- `--timeout` is **mandatory on both paths** — omitting it is `ExitUsage=2`, not a default duration.
- Timeout elapse (no change/appearance within the window) → `ExitTimeout=4`, the new exit code.
- Poll interval is a fixed 1 second (`awaitPollInterval`); `awaitClock` injects a fake time source for fast, sleep-free unit tests (`internal/cli/await_test.go`).

## New since the last index: PLM-031 dev-install mode

`internal/cli/dev.go` (`fledge dev status`) plus substantial changes to `internal/cli/init.go` (`--dev=<path>` / bare `--dev`) let a fledge-managed repo symlink copy-type scaffold files into a local fledge source tree instead of copying them, so edits to `internal/bootstrap/core/...` show up live without a re-run. `ValidateDevSource` (`internal/bootstrap/registry.go`) validates the target is a real fledge source tree (go.mod check). `init --dev` refuses when the dev-linked paths are already tracked by git (a stray-tracked-symlink guard). `internal/bootstrap/drift.go` and `stamp.go` gained dev-link-aware classification and a `Stamp.DevSource` field for this. This repo itself is dev-linked to `~/source/fledge` (per user memory / `.gitignore` entries covering `.fledge/scaffold.json` and the dev-link targets).

## Cross-module relationships

`internal/cli` is the thin glue: it never contains business logic, only orchestrates calls into `spec`, `check`, `graph`, `lock`, `ledger`, `roster`, `nest`, `bootstrap`, `scan`, `repo`. `internal/bootstrap` and `internal/cli` interact at exactly one seam — `fledge init`/`fledge agents`/`fledge dev` in `internal/cli` call into `internal/bootstrap`'s `LoadAdapters`, `WriteCore`, `WriteAdapter`, `DriftReport`, `LoadStamp`. The CLI acceptance-test suite (`cmd/fledge/testdata/*.txtar`, run via `go test ./cmd/fledge -run TestScripts`) is the primary integration-test surface spanning both layers; 37 fixtures as of this commit (`ls cmd/fledge/testdata/*.txtar | wc -l` = 37, including 8 new ones: `await`, `verdict`, `escalate`, `ledger-read`, `dev_preen`, `dev_rails`, `dev_refresh`, `dev_status`).

## Open Questions

- Windows symlink fallback for dev-link mode is referenced in a `drift.go` comment ("Windows degradation path — never errors") but the full fallback strategy isn't explicit in the code read (`internal-bootstrap` scout report).
- Whether `DevSource` is stamped on every `--dev` write or only on refresh is not confirmed from the files read (`internal-bootstrap` scout report).
