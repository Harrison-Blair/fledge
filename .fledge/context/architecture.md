---
generated: 2026-07-06T21:54:21Z
commit: 22c11810cf8ab8d8e8ae34253a6426af005561c2
agent: context-gatherer
fledge_version: 0.1.0
---

# Architecture

This repository is `fledge`, a spec-driven development tool at version 0.1.0, in a pre-implementation state: the scan surface consists only of root metadata files (`README.md`, `LICENSE`, `VERSION`, `.gitignore`). There is no application source code, so the architecture that exists today is the tooling scaffold that generates and consumes planning context.

## Current shape

- Single module (`root`) containing project identity and metadata only:
  - `README.md` — project name and tagline ("my spec driven development tool"); no install or usage docs.
  - `LICENSE` — AGPL-3.0, verbatim FSF text.
  - `VERSION` — bare semver string `0.1.0`.
  - `.gitignore` — excludes per-run regenerable intermediates.
- `.fledge/` is the tool's state root (`.gitignore` comments it as holding "Per-run intermediates — regenerable, not shared"): `.fledge/context/raw/` holds ephemeral scout reports and `.fledge/locks/` holds locks; both are git-ignored. Committed context lives in `.fledge/context/*.md`.
- Context generation flows: `.fledge/scripts/scan` enumerates modules → per-module scout reports in `.fledge/context/raw/` → synthesized concern docs in `.fledge/context/`.

## Cross-module relationships

None — there is only one module. No dependency edges exist yet.

## Open Questions

- Implementation language and tooling for fledge itself are not determinable; no package manifests exist at the root (`root.md` scout report).
