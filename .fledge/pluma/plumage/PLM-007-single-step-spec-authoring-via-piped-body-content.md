---
id: PLM-007
title: Single-step spec authoring via piped body content
status: hatched
priority: P2
authored: 2026-07-08T06:21:33Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# PLM-007: Single-step spec authoring via piped body content

## Context
Creating a spec today is a three-step round-trip: `fledge new` emits a template skeleton,
the agent must then read the created file to learn the CLI-allocated ID and skeleton shape,
and only then overwrite the body. The read-back exists solely because the agent needs the
allocated ID to write the `# PLM-###: title` heading and must avoid clobbering CLI-owned
frontmatter. This costs an extra read on every spec created and adds friction to the very
authoring loop the tool is built around. If `fledge new` could accept the body directly —
with the CLI still owning frontmatter and the title heading — the agent could author a spec
in a single call without ever needing the ID.

## User Stories
- As an agent authoring a spec, I want to supply the body when I create it, so that I do not
  have to create the file, read it back for the ID, and then rewrite it.
- As a workflow author, I want the planning process to create specs in one step, so that the
  dogfooded authoring loop is simpler and cheaper.

## Functional Criteria
Numbered, testable statements of behavior. Referenced downstream as FC-1, FC-2, …
1. FC-1: `fledge new` optionally accepts body content from a file or stdin and writes it as
   the spec body, under CLI-generated frontmatter and title heading.
2. FC-2: Supplying the body requires no knowledge of the allocated ID; the CLI owns and
   generates both the frontmatter and the `# <ID>: <title>` heading.
3. FC-3: Omitting the body input preserves today's behavior exactly (template skeleton).
4. FC-4: The dogfooded planning workflow authors specs in a single step using this input,
   preserving the draft-then-gate discipline.

## Acceptance Criteria
- [ ] AC-1: `fledge new plumage|feather … --body-file <path>` creates a spec whose body is the supplied content under CLI-generated frontmatter and a `# <ID>: <title>` heading; `--body-file -` reads the body from stdin.
- [ ] AC-2: Without `--body-file`, `fledge new` emits the template skeleton exactly as before (backward compatible).
- [ ] AC-3: Supplied content containing a frontmatter fence or a top-level H1 heading is rejected with a clear, actionable error and no file is created.
- [ ] AC-4: Body content is otherwise written verbatim; `new` performs no section-shape validation — `preen` remains the sole structural validator.
- [ ] AC-5: `planning.md` (core) is rewired so the post-gate authoring step uses `fledge new … --body-file -` instead of create-then-read-then-write; the scaffold is regenerated and the core/`init` txtar fixtures updated.
- [ ] AC-6: Automated tests (`--body-file` via path and stdin, both rejection cases, backward-compat skeleton, `--json` shape) cover AC-1..AC-4 and the full suite passes.

## Out of Scope
- Editing existing spec bodies (this is `new` only; no `set --body`).
- An inline `--body "<string>"` flag (file/stdin only).
- Section-shape validation inside `new` (`preen` owns structure).
- Any change to frontmatter-ownership rules.
- `implementation.md` prose and `fledge nest new`.

## Open Questions
None — all decisions resolved during the 2026-07-08 interrogation.
