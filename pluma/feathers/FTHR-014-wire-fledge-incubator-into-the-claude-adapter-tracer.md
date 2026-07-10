---
id: FTHR-014
title: Wire fledge-incubator into the Claude adapter (tracer)
plumage: PLM-010
status: pipping
priority: P2
depends_on: []
oversight: merge
authored: 2026-07-10T21:19:38Z
agent: fledge-orchestrate/planning
fledge_version: 0.3.0
---

# FTHR-014: Wire fledge-incubator into the Claude adapter (tracer)

## Description
The thin end-to-end tracer for PLM-010: introduce the `fledge-incubator` agent spec and wire
it through the Claude adapter so `fledge init` scaffolds it to `.claude/agents/`. After this
feather a real `fledge init` produces the incubator agent file, and the acceptance fixtures +
registry tests prove the whole path — embedded source → `manifest.yaml` → scaffolded output →
green tests — without touching any Go scaffolding logic (adding a file to an existing adapter
is a manifest edit, zero Go code). FTHR-015 (core delegation prose) references this agent;
FTHR-016 refreshes this repo's own scaffold to include it.

## Affected Modules
- **`internal/bootstrap/adapters/claude/agents/fledge-incubator.md`** (new) — the agent spec,
  structured like `fledge-forager.md`: frontmatter (`name: fledge-incubator`, description,
  `model: claude-sonnet-5`) + a body stating the one-shot, non-interactive drafting contract.
  See `.fledge/nest/modules.md` → internal-bootstrap; `.fledge/nest/architecture.md` → adapter
  manifest/agents.
- **`internal/bootstrap/adapters/claude/manifest.yaml`** — add one `files:` entry
  (`src: agents/fledge-incubator.md`, `dst: .claude/agents/fledge-incubator.md`) using the
  default copy/skip-if-exists policy, matching the brooder/forager/scout/skua entries.
- **`cmd/fledge/testdata/{init,init_agents,agents}.txtar`** — extend the agent assertions to
  include the incubator (the "must-update" fixtures; see `.fledge/nest/testing.md`).
- **`internal/bootstrap/registry_test.go`** — assert the Claude adapter includes the incubator
  and that derived tier + primitive coverage are unchanged.

## Approach
- The agent `.md` is a thin spawn-prompt like the peers. Its body encodes the contract the
  plumage fixed: given the orchestrator's resolved decisions + pointers (prospective
  `PLM-###`/`FTHR-###` ID, which template — `templates/plumage.md` or `templates/feather.md`,
  which `.fledge/nest/` concern docs to cite, and for feathers the plumage link / `depends_on`
  / `oversight`), the incubator **reads the template and cited concern docs itself** and
  returns the **full drafted body** (frontmatter fields + all sections) as its single final
  message. It is stateless (one draft per spawn), non-interactive, and **mutates no spec** —
  it never runs `fledge new`/`status`/`set`; the orchestrator commits after its gate.
- `manifest.yaml`: insert the entry among the `agents/` files. Default policy → the stamp
  records it and `--refresh` preserves user edits, exactly like the other agents.
- No new primitive is introduced; `tier_primitives` is untouched, so `DeriveTier` still yields
  Tier C. This is a Claude adapter file, so naming Claude mechanisms is fine here (unlike core).

## Tests
Written test-first: (1) write; (2) run against unchanged code, observe FAIL for the expected
reason (agent file + manifest entry absent); (3) implement until green.
- **`internal/bootstrap/registry_test.go`**
  - Assert `FindAdapter("claude")` `Files` contains an entry writing
    `.claude/agents/fledge-incubator.md` (extends the manifest/file coverage test).
  - Assert the Claude adapter's `DeriveTier` == Tier C and its `tier_primitives` set is
    unchanged (the incubator adds no primitive).
- **txtar** (`cmd/fledge/testdata/`)
  - `init.txtar` — `stdout 'created .claude/agents/fledge-incubator.md'`,
    `exists .claude/agents/fledge-incubator.md`, and `grep '<stable marker>'
    .claude/agents/fledge-incubator.md` (a stable phrase from the contract, e.g. the
    no-spec-mutation clause).
  - `init_agents.txtar` — include the incubator in the refresh/grep assertions alongside the
    existing agents.
  - `agents.txtar` — include the incubator where scaffolded agents are inventoried.
- Whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [ ] AC-2: An embedded `agents/fledge-incubator.md` + a `manifest.yaml` entry cause
      `fledge init` to scaffold `.claude/agents/fledge-incubator.md`; the file declares
      `model: claude-sonnet-5` and a non-interactive, one-shot draft-and-return contract that
      mutates no spec (satisfies PLM-010 FC-1, AC-1).
- [ ] AC-3: The agent's instructions define the input/output contract — given resolved
      decisions + pointers, it reads the template and cited concern docs itself and returns the
      full drafted body (frontmatter + all sections) as its final message (satisfies PLM-010
      FC-2, AC-2).
- [ ] AC-4: `registry_test.go` asserts the Claude adapter includes `fledge-incubator` and that
      its derived tier (C) and primitive coverage are unchanged; `go test ./...` and
      `go vet ./...` pass (satisfies PLM-010 AC-6).
