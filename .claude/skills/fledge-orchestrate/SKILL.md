---
name: fledge-orchestrate
description: Orchestrates fledge spec-driven development workflows in a fledge-managed repository (one containing a .fledge/ directory). Use when the user asks to make a plan, plan a feature, write requirements, or author tasks. Routes to the appropriate phase.
---

# fledge-orchestrate

You are running the fledge orchestration workflow. Fledge is a spec-driven development tool: repository knowledge lives in `.fledge/context/`, feature intent lives in `spec/requirements/`, and implementable work lives in `spec/tasks/`.

## Routing

Determine which phase the user's request calls for and load that phase's instructions from this skill's directory:

| Request looks like | Phase | Instructions |
|---|---|---|
| "make a plan", "plan <feature>", "write requirements for…", "break this into tasks", "author tasks for REQ-###" | Planning | Read `planning.md` in this skill's directory and follow it |
| "implement", "implement REQ-###", "implement TASK-###", "start implementation", "run the tasks" | Implementation | Read `implementation.md` in this skill's directory and follow it |
| Anything else (review, …) | Not built yet | Say so plainly; offer an existing phase if it fits |

Future phases will be added as sibling files to `planning.md`.

## Ground rules (all phases)

- Verify this is a fledge-managed repo (a `.fledge/` directory exists at the git root). If not, stop and ask whether to initialize one (`fledge init` creates the scaffold).
- Deterministic spec operations go through the `fledge` CLI — never hand-edit what it can write. Creation: `fledge new req|task` (ID allocation, filenames, frontmatter). Status: `fledge status <ID> <new>`. Other frontmatter fields: `fledge set`. Readiness/structure: `fledge ready`, `fledge graph`. Validation: `fledge check`. Task claims: `fledge lock`/`unlock`/`locks`. Spec *bodies* (prose sections) are yours to write and edit directly.
- Run `fledge check` as a pre-flight before closing any phase; fix errors before proceeding.
- All generated files carry frontmatter with authored/generated datetime (UTC ISO 8601), authoring agent, and `fledge_version` — `fledge new` stamps these automatically.
- Decisions belong to the user; facts belong in the repo. Look up facts, interrogate for decisions.
