#!/usr/bin/env bash
# Build reproducible Linux release archives. Usage: package.sh vX.Y.Z DIST
set -euo pipefail

tag=${1:?release tag required}
dist=${2:?output directory required}
if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "Invalid stable release tag: $tag" >&2
  exit 1
fi
mkdir -p "$dist"
dist=$(cd "$dist" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
module=$(go list -m)
timestamp=$(git show -s --format=%ct HEAD)

for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -buildvcs=true \
    -ldflags "-s -w -X $module/internal/lib/version.release=$tag" -o "$stage/fledge" .
  cp README.md LICENSE "$stage/"
  # Run the archive's binary on a matching Linux build host.
  if [[ "$(go env GOHOSTOS)/$(go env GOHOSTARCH)" == "linux/$arch" ]]; then
    [[ "$("$stage/fledge" --version)" == "fledge $tag" ]]
  fi
  tar --sort=name --mtime="@$timestamp" --owner=0 --group=0 --numeric-owner \
    -czf "$dist/fledge_${tag}_linux_${arch}.tar.gz" -C "$stage" fledge LICENSE README.md
done
(
  cd "$dist"
  sha256sum "fledge_${tag}_linux_amd64.tar.gz" "fledge_${tag}_linux_arm64.tar.gz" > checksums.txt
  sha256sum --check checksums.txt
)
