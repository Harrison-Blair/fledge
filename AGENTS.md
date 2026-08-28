# Repository Guidelines

## Architecture

Fledge is a Go CLI built with Cobra. Keep dependencies flowing in one direction:

```text
main.go -> cmd (composition root) -> cmd/<command> -> internal/<capability>
```

- `main.go` is the composition entry point and delegates to `cmd.Execute()`.
- `cmd/root.go` constructs the Cobra command tree and wires its immediate
  children.
- `cmd/<command>/` owns Cobra commands, flags, argument validation, help text,
  and CLI input/output adaptation for one command family.
- `internal/` owns application behavior, domain logic, and application metadata.
- Packages under `internal/` must not import Cobra or packages under `cmd/`.
- Keep business decisions out of Cobra callbacks. Parse CLI input, call the
  internal operation, then translate its result or error for the terminal.

## Command Organization

For a CLI capability named `<name>`, use this layout by default:

```text
cmd/<name>/<name>.go
cmd/<name>/<name>_test.go
internal/<name>/<name>.go
internal/<name>/<name>_test.go
```

- `cmd/<name>/` is the thin Cobra adapter package for the command or flag.
- `internal/<name>/` contains that capability's implementation and supporting
  files. Keep additional files in this package when the capability grows; see
  Package Organization for when a subpackage is warranted.
- Export a direct operation named after the capability when that name reads
  clearly at the call site; for example, `cmd/version/version.go` calls
  `version.Version()` from `internal/version`.
- Standalone command packages export `New() *cobra.Command`. The parent calls
  `AddCommand` for its immediate children; child packages never import or
  register themselves with their parent.
- A package that configures a root concern rather than creating a standalone
  command may export `Configure(*cobra.Command)`. The version flag uses this
  form.
- Mirror nested CLI command families below `cmd/`; for example,
  `cmd/project/create`. The immediate parent owns the child's registration.
- Do not use `init` functions or shared command globals for command-tree
  assembly. Construct and connect commands explicitly from `cmd.New()`.
- Pass inputs and dependencies explicitly and return values or errors. Do not
  pass `*cobra.Command` into `internal` packages.
- Do not call `os.Exit` from business logic. Return errors to `cmd`; process exit
  belongs at the top-level CLI boundary.
- Prefer one direct function over an interface or command struct when there is
  only one implementation. A Cobra command is not, by itself, a reason to
  introduce the GoF Command pattern.
- Mirror the CLI hierarchy under `cmd`, but organize `internal` by application
  capability. The names may align, but a CLI rename or nested command does not
  require moving reusable business logic.
- When mirrored packages have the same name, use role-based import aliases at
  the wiring site, such as `versioncmd` and `internalversion`.

## Package Organization

`internal/<capability>/` is flat by default. Grow it with files, not
subpackages, until a group of files serves one distinct sub-responsibility
and the rest of the package needs only a narrow API from it (roughly 1–3
exported entry points). File count is a smell, not a trigger.

- Subpackages are short singular nouns naming the responsibility, with no
  parent prefix: `internal/session/record`, not `sessionrecord`.
- `<pkg>/types` is allowed when two or more sibling packages share data types
  or interfaces. It holds contracts only: no logic, no sibling imports.
- `<pkg>/utils` is allowed for stateless helpers with no domain meaning used
  by two or more siblings. It imports no sibling. Prefer a package named for
  what it does (`internal/subprocess`) when one name fits.
- The parent package stays the facade. `cmd/` imports
  `internal/<capability>`, never its subpackages. A sibling capability may
  import a subpackage only for leaf contracts (`types`).
- When a subpackage type must remain visible through the parent, re-export
  it with a type alias (`type AgentChoice = types.AgentChoice`).
- Import a subpackage unaliased. If a local variable would shadow the package
  name, rename the variable (`rec`, not `sessionrecord`).

### Files

- One concern per file, named as a noun for that concern (`registry.go`,
  `naming.go`). Small related contracts may share a file named for their
  role. There is no size limit; size is a smell that triggers a split by
  concern.
- `constants.go` holds a package's shared `const`, `var`, and sentinel
  `Err*` values once more than one file uses them. A value used by one file
  stays beside its use.
- Every package with more than one non-test file has a `doc.go` containing
  only the package comment: a one-paragraph summary followed by a one-line
  map of each file. Adding, renaming, or removing a file updates `doc.go`
  in the same change.

### Tests

- Test files move with the code they exercise. Tests that cross a subpackage
  boundary are rewritten against the exported API; internals are never
  exported to keep a test alive.
- Fixtures shared by two or more packages' tests live in `<pkg>/<pkg>test`
  (for example `internal/session/sessiontest`). It must not import a package
  whose own tests import it; duplicate a trivial helper instead.

## Version Invariant

- `internal/version/VERSION` is the sole source of truth for the release
  version. Do not duplicate the version in Go code, tests, or build scripts.
- `internal/version.Version()` embeds and returns that value so binaries do not
  depend on their runtime working directory.
- Cobra exposes the value through `fledge --version` and `fledge -V`; there is
  no `version` subcommand or lowercase `-v` alias.

## Design Discipline

- Review `reference/go-design-patterns.md` before introducing a design pattern.
- Match a pattern's documented **Intent** and **Use when** criteria rather than
  adding structure based only on its name.
- Prefer idiomatic Go functions and composition. Introduce interfaces where the
  consumer needs substitution or multiple implementations, not preemptively.
- Keep exported APIs small; `internal` package boundaries are the primary
  encapsulation mechanism.

## Verification

After changing Go code:

```sh
gofmt -w <changed-go-files>
go test ./...
go vet ./...
```

Add unit tests beside internal logic and focused CLI tests beside each command
adapter. Build fresh Cobra commands in CLI tests so flag state does not leak
between cases.
