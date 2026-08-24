package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var value string

// Version returns the application version embedded at build time.
func Version() string {
	return strings.TrimSpace(value)
}
