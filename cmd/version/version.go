// Package version configures the CLI version flags.
package version

import (
	"github.com/spf13/cobra"

	internalversion "github.com/Harrison-Blair/fledge/internal/lib/version"
)

// Configure adds --version and -V to the root command.
func Configure(root *cobra.Command) {
	root.Version = internalversion.Version()
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	root.Flags().BoolP("version", "V", false, "Print the Fledge version")
}
