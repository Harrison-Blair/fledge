#!/usr/bin/env bash
# Builds the fledge binary, then installs it.
# Takes no arguments; install.sh parses none. Override the destination with the
# BINDIR environment variable (e.g. BINDIR=/opt/bin scripts/reinstall.sh).
set -euo pipefail

if [[ $# -gt 0 ]]; then
	echo "usage: $0 (no arguments; set BINDIR to override the install dir)" >&2
	exit 2
fi

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$here/build.sh"
"$here/install.sh"
