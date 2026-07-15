# Releasing fledge

How a new `fledge` binary version gets published, and the dogfood-scaffold
step that must follow it in this repo.

## 1. Bump the version-stamped locations

A release bumps the version in two places that must move together:

- **`VERSION`** — the file at the repo root; this is the source of truth the
  release workflow reads.
- **`internal/cli/version.go`** — the hardcoded `binaryVersion` var (the
  fallback baked into `fledge version` when a binary isn't built with the
  release `-ldflags` override).

Also check `cmd/fledge/testdata/stamp_warning.txtar` — its fixture pins a
specific old/new version pair to exercise the scaffold-version-mismatch
warning, so it can need a matching update when the version changes underneath
it.

## 2. Push to `main`

`.github/workflows/release.yml` triggers on every push to `main`. Its
`detect-version` job diffs `VERSION` against the previous commit: if
unchanged, the workflow stops there (only the lint/build/test safety net
runs). If `VERSION` changed, it builds the 4 release binaries
(`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, each with
`-ldflags "-X .../internal/cli.binaryVersion=$VERSION"`) and publishes a
GitHub Release tagged `v$VERSION` with the archives and checksums attached.
No separate manual tag/push step is needed — merging the version bump to
`main` is what triggers the release.

## 3. Refresh and commit this repo's own scaffold

This repo is itself fledge-managed (dogfooding), and its scaffold is stamped
with the fledge version that wrote it (`.fledge/scaffold.json`'s
`fledgeVersion` field). After a version bump, that stamp is stale until it's
regenerated, so once the new binary is built:

```sh
go install ./cmd/fledge   # reinstall the just-bumped binary locally
hash -r
fledge init --refresh     # rewrite fledge-owned scaffold files + scaffold.json
git status                # review what regeneration changed
```

Commit the regenerated `.fledge/scaffold.json` (and any other scaffold files
`--refresh` touched) so the dogfood stamp tracks the new version — otherwise
`fledge preen` / every command will start warning about a stale scaffold
(see the `stamp_warning.txtar` behavior above) even though the repo is
running the version that wrote it.
