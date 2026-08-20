// Package version configures the CLI surface for application version output.
package version

import (
	internalversion "fledge/internal/version"

	"github.com/spf13/cobra"
)

// Configure adds version metadata and flags to command.
func Configure(command *cobra.Command) {
	command.Version = internalversion.Version()
	command.Flags().BoolP("version", "V", false, "version for "+command.Name())
}
