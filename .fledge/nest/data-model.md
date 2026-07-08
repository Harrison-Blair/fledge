---
generated: 2026-07-08T05:28:12Z
commit: e46c481a047d45ef10bcd79a3326d47932b32868
agent: fledge-forager
fledge_version: 0.2.1
---

# Data Model

The on-disk artifacts fledge reads and writes, and the in-memory types that back them. All frontmatter is YAML, CLI-owned; bodies are markdown.

## Plumage (`pluma/plumage/PLM-###-<kebab>.md`)

Requirement/feature spec. Frontmatter: `id`, `title`, `status` (`egg` → `hatched` → `fledged`), `priority` (P0–P3), `authored`, `agent`, `fledge_version`. Body sections: Context/motivation, user stories, Functional Criteria, Acceptance Criteria (unchecked `- [ ] AC-N:` boxes), out-of-scope. Backed by `spec.Requirement` (`internal/spec`); rendered via `Requirement.Render` + `RequirementBody`.

## Feather (`pluma/feathers/FTHR-###-<kebab>.md`)

Implementable task under one plumage. Frontmatter: `id`, `title`, `plumage` (parent link), `status` (`egg` → `pipping` → `hatching` → `fledged`), `priority`, `depends_on` (list of FTHR IDs, DAG — cycles rejected), optional `oversight` (`during` | `merge`; omitted = autonomous), `authored`, `agent`, `fledge_version`. Body: Description, Affected Modules, Approach, Tests, Acceptance Criteria (`- [ ] AC-N:`). Backed by `spec.Task`.

## Brood / lock (`.fledge/broods/FTHR-###.brood`)

Claim on a feather while it is worked. `lock.Record{Task, Owner, PID, Created, Branch}` (JSON). Created by `fledge brood` (atomically also sets the feather `status: hatching`), released by `fledge abandon`. `fledge broods` lists held locks with a PID-liveness check. Gitignored. **This is the only durable record of who owns in-flight work** — relevant to any worker-pairing/recovery work.

## Evidence / molt (`.fledge/molt/FTHR-###.md`)

Per-feather evidence file holding per-criterion (AC-N) output — AC-1 typically the pre-implementation failing test run, the rest post-implementation proof. Written inside the worktree by the brooder; the paired skua audits it. `check.Run()` receives `repo.EvidenceDir()` and validates evidence presence for checked criteria. NOTE: the most recent commit (`e46c481`) reconstructs an FTHR-004 evidence file "never captured at build time" — evidence files can go missing/orphaned, which is a live concern.

## Nest documents (`.fledge/nest/`)

Distilled repo knowledge. Eight concern docs (architecture, modules, conventions, data-model, dependencies, entry-points, testing, domain) + `index.md` (routing) written by the forager; `raw/<module>.md` scout reports (gitignored). Backed by `internal/nest` (`Doc`, `Kind` = Concern|Scout, renderers). Frontmatter (`generated`, `commit`, `agent`, `fledge_version`) is stamped by `fledge nest scaffold`/`stamp` — the binary is the schema's single source.

## Manifest (`internal/bootstrap/adapters/<harness>/manifest.yaml`, embedded)

`Manifest{name, detector, tier_primitives (primitive→mechanism), files, piping_file, dir}`; `ManifestFile{src, dst, + write-policy flags}`. Read at build time by `registry.go`, embedded in the binary — never lands in a target repo. Tier is derived from `tier_primitives` coverage, never stored.

## Relationships & integrity

- feather `plumage` → PLM must resolve, else it is an **orphan feather** (`colony.go`).
- feather `depends_on` → FTHRs must resolve, else a **dangling ref** (`colony.go`).
- checked AC boxes require a matching evidence file (`check`/`preen`).
- These integrity checks are surfaced by `fledge preen` (errors) and `fledge colony` (reports orphans, dangling refs, blocked, degraded-data). See `entry-points.md`.
