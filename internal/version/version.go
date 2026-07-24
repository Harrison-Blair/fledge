// Package version reports the fledge binary's version.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Get returns the version of the fledge binary. Builds made with the "dev"
// build tag (local installs via scripts/install.sh) carry a "-dev" suffix.
func Get() string { return strings.TrimSpace(raw) + suffix }
