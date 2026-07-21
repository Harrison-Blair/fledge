#!/usr/bin/env bash
# Builds the fledge binary, then installs it.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

"$here/build.sh"
"$here/install.sh" "$@"
