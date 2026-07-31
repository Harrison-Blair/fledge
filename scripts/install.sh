#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/.." && pwd -P)"
source_path="${repo_root}/bin/fledge"

if [[ ! -x "${source_path}" ]]; then
  printf 'error: %s is missing or not executable; run scripts/build.sh first\n' \
    "${source_path}" >&2
  exit 1
fi

destination_dir="$(go env GOBIN)"
if [[ -z "${destination_dir}" ]]; then
  gopath="$(go env GOPATH)"
  if [[ -z "${gopath}" ]]; then
    printf 'error: could not determine GOPATH for binary installation\n' >&2
    exit 1
  fi

  if [[ "$(go env GOOS)" == "windows" ]]; then
    destination_dir="${gopath%%;*}/bin"
  else
    destination_dir="${gopath%%:*}/bin"
  fi
fi

mkdir -p "${destination_dir}"

temporary_path="${destination_dir}/.fledge.install.$$"
trap 'rm -f "${temporary_path}"' EXIT
cp "${source_path}" "${temporary_path}"
chmod 0755 "${temporary_path}"
mv -f "${temporary_path}" "${destination_dir}/fledge"
trap - EXIT

printf 'Installed fledge to %s\n' "${destination_dir}/fledge"
