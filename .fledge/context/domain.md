---
generated: 2026-07-06T21:54:21Z
commit: 22c11810cf8ab8d8e8ae34253a6426af005561c2
agent: context-gatherer
fledge_version: 0.1.0
---

# Domain

Glossary of business/domain concepts this repository embodies, drawn from the root metadata files.

- **fledge** — the project itself: a spec-driven development tool (`README.md`).
- **spec-driven development** — workflow where specs/requirements are authored before implementation; fledge's core purpose per the `README.md` tagline.
- **per-run intermediates** — regenerable artifacts under `.fledge/context/raw/` and `.fledge/locks/` that are excluded from version control (`.gitignore`).
- **context** — the synthesized `.fledge/context/` document set that downstream planning agents consume; committed, unlike the raw intermediates it is built from.
