#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: %s COMMIT\n' "$0" >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/.." && pwd -P)"
version_file="${repo_root}/internal/buildinfo/VERSION"

mapfile -t version_lines < "${version_file}"
if [[ ${#version_lines[@]} -ne 1 ]]; then
  printf '%s must contain exactly one line\n' "${version_file}" >&2
  exit 1
fi

tag="${version_lines[0]}"
if [[ ! "${tag}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  printf 'invalid release version %q: expected vMAJOR.MINOR.PATCH\n' "${tag}" >&2
  exit 1
fi

cd "${repo_root}"
expected_commit="$(git rev-parse "$1^{commit}")"
checked_out_commit="$(git rev-parse 'HEAD^{commit}')"
if [[ "${checked_out_commit}" != "${expected_commit}" ]]; then
  printf 'checked-out commit %s is not release commit %s\n' \
    "${checked_out_commit}" "${expected_commit}" >&2
  exit 1
fi

if git show-ref --verify --quiet "refs/tags/${tag}"; then
  tag_commit="$(git rev-list -n 1 "${tag}")"
  if [[ "${tag_commit}" != "${expected_commit}" ]]; then
    printf 'release %s already exists at %s; no release needed\n' \
      "${tag}" "${tag_commit}" >&2
    exit 3
  fi
else
  while IFS= read -r existing_tag; do
    [[ "${existing_tag}" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || continue
    highest="$(printf '%s\n%s\n' "${existing_tag}" "${tag}" | sort -V | tail -n 1)"
    if [[ "${highest}" != "${tag}" ]]; then
      printf 'release %s is not newer than existing release %s\n' \
        "${tag}" "${existing_tag}" >&2
      exit 1
    fi
  done < <(git tag --list)
fi

printf '%s\n' "${tag}"
