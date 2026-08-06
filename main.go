package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/Harrison-Blair/fledge/cmd"
)

// embeddedVersion carries the repository's VERSION file, the single source of
// truth for the product version. It is trimmed and validated once here, at the
// edge, so the commands only ever see the normalized string.
//
//go:embed VERSION
var embeddedVersion string

func main() {
	version, err := normalizeVersion(embeddedVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
