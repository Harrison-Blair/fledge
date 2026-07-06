---
generated: 2026-07-06T21:54:21Z
commit: 22c11810cf8ab8d8e8ae34253a6426af005561c2
agent: context-gatherer
fledge_version: 0.1.0
---

# Modules

Repo map for `fledge` at commit 22c1181. The scan (`.fledge/scripts/scan`) reports exactly one module; there are no source-code modules yet.

## root

- **Purpose:** Repository-root metadata establishing project identity, licensing, version, and VCS hygiene. No executable code.
- **Key files:**
  - `README.md` — project name `fledge` and tagline "my spec driven development tool" (2 lines).
  - `LICENSE` — GNU AGPL-3.0, verbatim FSF text (661 lines), copyright line not filled in.
  - `VERSION` — `0.1.0`; consumed as `fledge_version` in generated context frontmatter.
  - `.gitignore` — ignores `.fledge/context/raw/` and `.fledge/locks/` as per-run intermediates.
- **Look here for:** the project's name/tagline, the license terms, the current version string, and which `.fledge/` paths are treated as ephemeral vs. committed.
