---
id: FTHR-005
title: "nest foundation + nest new verb (tracer)"
plumage: PLM-003
status: pipping
priority: P2
depends_on: []
authored: 2026-07-08T01:49:45Z
agent: fledge-orchestrate/planning
fledge_version: 0.2.1
---

# FTHR-005: nest foundation + nest new verb (tracer)

## Description
The thin end-to-end tracer for PLM-003: stand up the `internal/nest` package (both nest
frontmatter schemas + a canonical fixed-key renderer + embedded body templates + the closed
known-doc registry), register a new `fledge nest` command, and implement exactly one verb —
`nest new <doc>` — through every layer (render → embedded template → atomic write →
text/JSON output → exit codes). Registering the command also updates the generated Claude
allow-list and the `init`/`agents` txtar fixtures so the suite stays green. Later feathers
widen the verb set (FTHR-006) and migrate the prose (FTHR-007); this feather proves the
spine.

## Affected Modules
- **New `internal/nest`** — package with the two schemas, renderer, registry, embedded
  `templates/*.md`. Imports `internal/spec` for shared primitives (`SplitFrontmatter`,
  `WriteFileAtomic`, and the scalar-quoting helper, exported as `spec.YAMLScalar` in this
  feather). See `.fledge/nest/architecture.md` → Layer 1 and `.fledge/nest/modules.md` →
  internal/spec.
- **New `internal/cli/nest.go`** — `register("nest", runNest, usage)` in `init()`; adds
  `"nest"` to `commandOrder` (`internal/cli/cli.go`). Reuses `emitJSON`, `usageErr`, `fail`,
  `RequireFledge`, `relPath`, and repo helpers `ContextDir`/`Head`/`Version`. See
  `.fledge/nest/conventions.md` → Command dispatch.
- **`internal/bootstrap`** — no Go change, but the generated Claude Bash allow-list now
  includes `nest`; `cmd/fledge/testdata/init.txtar` and `init_agents.txtar`/`agents.txtar`
  fixtures updated to expect it (`registry_test.go:TestClaudeAllowListGenerated` passes
  automatically once `commandOrder` includes it). See `.fledge/nest/architecture.md` → "How
  the layers interact" (3).

## Approach
- **Schemas & renderer** (`internal/nest/nest.go`): a `Doc` with a `Kind`
  (`Concern`|`Scout`) and fields `Generated/Commit/Agent/FledgeVersion` (concern) and
  `Module/Authored/Agent/FledgeVersion` (scout). `Frontmatter()` writes the fixed key order
  per kind with `spec.YAMLScalar` quoting and both `---` fences — the exact pattern of
  `internal/spec/frontmatter.go`. `Render() = Frontmatter() + Body`.
- **Registry** (`internal/nest/docs.go`): `ConcernDocs` — the ordered closed set
  (`architecture, modules, conventions, data-model, dependencies, entry-points, testing,
  domain, index`); `IsKnownDoc(name)`; `Title(name)` for the stub heading.
- **Templates** (`internal/nest/templates/*.md`, `//go:embed`): `concern-doc.md` (generic
  `# <Title>` + scope-statement placeholder + comment pointing to context-doc conventions),
  `index.md` stub, and `scout-report.md` (the canonical body, moved here as the single
  source — its verb lands in FTHR-006 but the template + scout schema are defined now).
- **Command** (`internal/cli/nest.go`): `runNest` dispatches on `args[0]`. FTHR-005
  implements `new`; unimplemented verbs return a usage error listing the available verbs
  (FTHR-006 wires them). `nest new <doc>`: reject unknown `<doc>` (`usageErr`, exit 2); build
  a concern `Doc` with `Generated=now UTC RFC3339`, `Commit=r.Head()`,
  `FledgeVersion=r.Version(binaryVersion)`, `Agent=--agent|default fledge-forager`,
  `Body=concern stub`; write to `r.ContextDir()/<doc>.md` with `O_EXCL` unless `--force` (an
  existing file without `--force` → `fail`, exit 1, matching `fledge new`). `--json` emits
  `{"path": "<rel>"}`; text prints `created <rel>`.

## Tests
Written test-first: (1) write; (2) observe FAIL against unchanged code for the expected
reason; (3) implement until green.
- **`internal/nest/nest_test.go`**
  - `TestConcernFrontmatterKeyOrder` — pins the exact `generated/commit/agent/fledge_version`
    order and fences.
  - `TestScoutFrontmatterKeyOrder` — pins `module/authored/agent/fledge_version`.
  - `TestRenderPreservesBody` — `SplitFrontmatter(Render())` returns the body bytes unchanged
    (round-trip).
  - `TestYAMLScalarQuoting` — a title needing quotes is canonically quoted.
- **`cmd/fledge/testdata/nest.txtar`** (new)
  - `nest new architecture` creates `.fledge/nest/architecture.md` with correct frontmatter
    (commit/version stamped) and titled body.
  - unknown doc `nest new bogus` → exit 2, stderr message.
  - existing file without `--force` → exit 1; with `--force` → overwrites, exit 0.
  - `--json` emits the `{"path":...}` shape.
- **Fixture updates**: `init.txtar` / `init_agents.txtar` / `agents.txtar` updated to expect
  `nest` in the generated allow-list; whole `go test ./...` + `go vet ./...` green.

## Acceptance Criteria
- [ ] AC-1: The tests listed above were observed failing before implementation and pass
  after.
- [ ] AC-2: `internal/nest` renders both frontmatter schemas in canonical fixed-key order
  with body byte-preservation (satisfies PLM-003 FC-2; unit tests above).
- [ ] AC-3: `fledge nest new <doc>` creates a known concern doc with stamped frontmatter,
  rejects unknown names (exit 2), and refuses overwrite without `--force` (exit 1) (satisfies
  PLM-003 FC-4; `--json`/exit codes per FC-8).
- [ ] AC-4: `nest` is registered in `commandOrder` and the generated Claude allow-list +
  `init`/`agents` txtar fixtures are updated; `go test ./...` and `go vet ./...` pass
  (satisfies PLM-003 FC-1 and AC-4).
