package cli

import (
	"flag"
	"fmt"
)

// binaryVersion is the version of the fledge binary itself.
// Overridable at build time: -ldflags "-X ...internal/cli.binaryVersion=x.y.z".
var binaryVersion = "0.6.8"

func init() { register("version", runVersion, "fledge version [--json]") }

func runVersion(args []string) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *jsonOut {
		return emitJSON(map[string]string{"version": binaryVersion})
	}
	fmt.Printf("fledge %s\n", binaryVersion)
	return ExitOK
}
