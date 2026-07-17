---
generated: 2026-07-17T07:00:54Z
commit: ee49464adb830bef7189f94a1d3253927d33fb5f
agent: fledge-forager
fledge_version: 0.6.7
---

# Context Index

## architecture.md
The two-layer split (deterministic CLI vs. agent-neutral bootstrap/adapter system), the 6-primitive/tier model, and the two subsystems that are new since the last regeneration: the PLM-030 handoff ledger (`await`'s change-wait vs. existence-wait contract, the new `ExitTimeout` exit code) and PLM-031 dev-install mode (dev-linked scaffold files, drift-aware classification).
Read this when: orienting to the codebase for the first time, planning a change that spans the CLI/bootstrap seam, or needing to understand `fledge await`'s wait semantics or dev-link mode before touching either.

## modules.md
Repo map: every top-level module from `fledge scan` (root, cmd, docs, `.github`+scripts, and five `internal/` groupings — bootstrap, cli, spec, core-infra, state) with purpose, key files, and file/byte counts.
Read this when: deciding which module(s) a change touches, or orienting to an unfamiliar part of the tree before diving into source.

## conventions.md
Reconciled conventions: the uniform CLI command pattern, spec frontmatter/ID/criteria rules (byte-preserved bodies, single-byte checkbox flips, `fledge set` replaces-not-appends), bootstrap manifest/write-policy/drift rules, ledger atomicity and wait-contract conventions, and the flock/os.Link/atomic-rename concurrency idiom used throughout.
Read this when: writing new CLI code, a new spec-touching feature, or anything that needs to match existing idiom rather than invent a new one.

## data-model.md
Every core struct across the codebase: spec types (`Requirement`, `Task`, `Criterion`, `Set`), the new ledger types (`Record`, `StatusRecord`, `VerdictRecord`, `EscalationRecord`), lock/nest/roster types, and bootstrap/scaffold types (`Manifest`, `Stamp`, `StampEntry`, `Drift`). Also resolves the internal/nest/templates vs. .fledge/skills/templates naming collision (different purposes, not duplicates).
Read this when: adding a field to a spec/ledger/scaffold type, or needing exact struct shapes before writing code that consumes or produces them.

## dependencies.md
The small, stable external dependency set (goccy/go-yaml, rogpeppe/go-internal's testscript, golang.org/x/{sys,tools}) plus stdlib usage patterns (flock, os.Link, atomic rename, embed, text/template) and GitHub Actions used in CI.
Read this when: adding a new dependency (to check whether an existing one already covers the need) or tracing which package pulls in which stdlib/third-party primitive.

## entry-points.md
The binary entrypoint (`cmd/fledge/main.go` → `internal/cli.Run`), build/test/install commands (including the dogfooding reinstall loop and `fledge init --refresh` regeneration step), and every public package API surface (`internal/spec`, `internal/check`, `internal/graph`, `internal/ledger`, `internal/bootstrap`, etc.).
Read this when: needing the exact command to build, test, or reinstall fledge, or looking for which function is the public entry into a given package's logic.

## testing.md
Both test layers (37 txtar CLI-acceptance fixtures under `cmd/fledge/testdata/`, and per-package Go unit tests) with counts and coverage highlights per package, including the 8 fixtures new since the last regeneration (await, verdict, escalate, ledger-read, dev_preen, dev_rails, dev_refresh, dev_status).
Read this when: writing or extending a test, deciding whether a behavior is already covered, or needing the exact `go test` invocation for a specific command or package.

## domain.md
Full bird-themed glossary — spec vocabulary (plumage, feather, pipping, brood, molt, preen), orchestration vocabulary (nest, forager, scout, skua, brooder, tier, primitive, adapter, dev-link), and the new PLM-030 ledger vocabulary (subject, kind, heartbeat, await's two wait modes, stale).
Read this when: unsure what a bird-themed term means, or writing prose/specs that need to use this vocabulary correctly and consistently.

## Open Questions carried forward

- Windows dev-link fallback behavior is referenced but not fully specified in code comments (see `architecture.md`).
- Whether `Stamp.DevSource` is written on every `--dev` invocation or only on refresh is unconfirmed from the files read (see `architecture.md`, `data-model.md`).
