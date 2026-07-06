---
generated: 2026-07-06T21:54:21Z
commit: 22c11810cf8ab8d8e8ae34253a6426af005561c2
agent: context-gatherer
fledge_version: 0.1.0
---

# Conventions

Conventions observable in this repo today. With no source code present, these are repository-hygiene and metadata conventions rather than coding style; expect this document to grow substantially once implementation begins.

## Versioning

- Bare semantic version in a single-line `VERSION` file (`0.1.0`), no prefix or metadata (`VERSION`).
- Generated context documents stamp this value as `fledge_version` in their frontmatter.

## Repository hygiene

- `.fledge/` is the tool's state root. Regenerable per-run intermediates — `.fledge/context/raw/` and `.fledge/locks/` — are git-ignored, with an explanatory comment in `.gitignore` ("Per-run intermediates — regenerable, not shared"). Synthesized context docs in `.fledge/context/` are committed.
- Minimal README style: lowercase project name, one-line tagline (`README.md`).

## Licensing

- AGPL-3.0 — copyleft that extends to network-server use (section 13) (`LICENSE`).

## Open Questions

- The `LICENSE` copyright notice placeholder (`<year> <name of author>`) is not filled in; the copyright holder is unstated.
- Whether `VERSION` is bumped manually or by tooling is not determinable from the current files.
