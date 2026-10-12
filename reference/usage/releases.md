# Install, update, and releases

[README](../../README.md)

## Install and update

Install with Go, or download a Linux amd64/arm64 archive and `checksums.txt` from [Releases](https://github.com/Harrison-Blair/fledge/releases/latest).
Check the archive with `sha256sum --check --ignore-missing checksums.txt` before extracting.
Release archives contain `fledge`, `LICENSE`, and `README.md`.

```sh
go install github.com/Harrison-Blair/fledge@latest
fledge --version
fledge update --check
fledge update
fledge update --yes
```

Existing binaries without the `update` command need the Go install command or a manual archive installation once.
The updater installs the latest stable release only, verifies the archive's SHA-256, and replaces the running executable while preserving permissions and following symlinks.
It needs write access to the installation directory.
Download, verification, and replacement failures leave the installed executable intact.

Updating affects subsequent invocations.
Already-running agents and commands continue with their existing processes.

`--check` only reports availability.
Ordinary updates ask for confirmation with a default of no.
Noninteractive use requires `--yes`.
The updater leaves equal or newer stable versions alone.

Development builds cannot be compared reliably, so installing a stable release over one requires confirmation or `--yes`.
There are no background checks.
The updater currently supports Linux amd64 and arm64.

## Releases

Git tags own the version.
There is no version file to edit.
Merge reviewed changes into `main`, then select **Actions → Release → Run workflow** on `main` and choose a bump.
The choice authorizes publication once checks and packaging pass:

| Bump | Example from `v0.4.7` |
| --- | --- |
| `patch` (default) | `v0.4.8` |
| `minor` | `v0.5.0` |
| `major` | `v1.0.0` |

You or an agent with authenticated GitHub write access can also run:

```sh
gh workflow run release.yml --ref main -f bump=minor
# Find the dispatched run, then watch it:
gh run list --workflow release.yml --event workflow_dispatch
gh run watch RUN_ID --exit-status
```

The workflow releases the exact selected `main` commit, runs all checks, builds both Linux archives, and generates notes since the previous stable tag.
It uses a temporary draft to upload and verify every asset, then publishes automatically and marks the release latest.
Merging alone does not release.

If a run fails after creating its draft, rerun that original workflow run using the same bump and commit.
The workflow reuses existing matching assets.
The workflow refuses mismatched assets.
A published commit is a successful no-op.

A draft targeting another commit blocks new releases until you finish it or deliberately remove the draft.
Existing tags are never moved.
Deleting a draft does not authorize moving its tag.
The workflow serializes concurrent release requests.

GitHub may replace an older pending run with a newer request, so inspect the run status rather than assuming all queued requests will publish.

Stable tags use `vMAJOR.MINOR.PATCH`.
A `v2` or later release requires migrating `go.mod` and imports to the corresponding `/vN` module path first.
Release binaries report their exact tag.
Local builds use Go's embedded tag or commit-derived version, including `+dirty` when appropriate.

Metadata-free builds report `dev`.
