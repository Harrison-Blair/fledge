// Package version reports the embedded Fledge release version.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the release version without surrounding whitespace.
func Version() string {
	return strings.TrimSpace(version)
}
