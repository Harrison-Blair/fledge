#!/usr/bin/env bash
# Validates the embedded version against the repository's semantic release tags.
set -euo pipefail

usage() {
	echo "usage: $0 new | release <commit>" >&2
	exit 2
}

mode="${1:-}"
case "$mode" in
new)
	[[ $# -eq 1 ]] || usage
	;;
release)
	[[ $# -eq 2 ]] || usage
	;;
*)
	usage
	;;
esac

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version_file="$repo/internal/version/VERSION"

mapfile -t version_lines < "$version_file"
if [[ ${#version_lines[@]} -ne 1 ]]; then
	echo "internal/version/VERSION must contain exactly one line" >&2
	exit 1
fi

version="${version_lines[0]}"
if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
	echo "invalid version '$version': expected strict MAJOR.MINOR.PATCH" >&2
	exit 1
fi

cd "$repo"
tag="v$version"
tag_exists=false

if [[ "$mode" == "release" ]]; then
	expected_commit="$(git rev-parse "$2^{commit}")"
	checked_out_commit="$(git rev-parse HEAD^{commit})"
	if [[ "$checked_out_commit" != "$expected_commit" ]]; then
		echo "checked out commit $checked_out_commit is not merge commit $expected_commit" >&2
		exit 1
	fi
fi

if git show-ref --verify --quiet "refs/tags/$tag"; then
	tag_exists=true
	tag_commit="$(git rev-list -n 1 "$tag")"
	if [[ "$mode" == "new" ]]; then
		echo "release tag $tag already exists at $tag_commit" >&2
		exit 1
	fi

	if [[ "$tag_commit" != "$expected_commit" ]]; then
		echo "release tag $tag points to $tag_commit, not $expected_commit" >&2
		exit 1
	fi
fi

if [[ "$tag_exists" != true ]]; then
	while IFS= read -r existing_tag; do
		[[ "$existing_tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || continue

		highest="$(printf '%s\n%s\n' "$existing_tag" "$tag" | sort -V | tail -n 1)"
		if [[ "$highest" != "$tag" ]]; then
			echo "version $version must be greater than existing release tag $existing_tag" >&2
			exit 1
		fi
	done < <(git tag --list)
fi

if [[ "$mode" == "release" && "$tag_exists" == true ]]; then
	echo "version $version is valid; $tag already targets the merge commit" >&2
else
	echo "version $version is valid and $tag is unused" >&2
fi
printf '%s\n' "$version"
