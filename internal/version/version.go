// Package version reports the fledge binary's version.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Get returns the version of the fledge binary.
func Get() string { return strings.TrimSpace(raw) }
