// Package show adapts managed profile inspection to Cobra.
package show

import (
	"fmt"
	"io"
	"strings"

	internalprofile "fledge/internal/profile"

	"github.com/spf13/cobra"
)

type getOperation func(string) (internalprofile.Profile, bool)

// New constructs the profile show command.
func New() *cobra.Command {
	return newCommand(internalprofile.Get)
}

func newCommand(get getOperation) *cobra.Command {
	return &cobra.Command{
		Use:   "show NAME",
		Short: "Show a Fledge-managed agent profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configured, ok := get(args[0])
			if !ok {
				return fmt.Errorf("unknown profile %q", args[0])
			}
			return writeProfile(cmd.OutOrStdout(), configured)
		},
	}
}

func writeProfile(output io.Writer, configured internalprofile.Profile) error {
	_, err := fmt.Fprintf(output,
		"Name: %s\nDescription: %s\nDefault harness: %s\nDefault model: %s\nDefault arguments: %s\n\nInstructions:\n%s",
		configured.Name,
		configured.Description,
		optional(configured.Defaults.Harness),
		optional(configured.Defaults.Model),
		optional(strings.Join(configured.Defaults.Args, " ")),
		configured.Instructions,
	)
	return err
}

func optional(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
