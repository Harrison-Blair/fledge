// Package list adapts managed profile listing to Cobra.
package list

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	internalprofile "fledge/internal/profile"

	"github.com/spf13/cobra"
)

type listOperation func() []internalprofile.Profile

// New constructs the profile list command.
func New() *cobra.Command {
	return newCommand(internalprofile.List)
}

func newCommand(list listOperation) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Fledge-managed agent profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profiles := list()
			sort.Slice(profiles, func(i, j int) bool {
				return profiles[i].Name < profiles[j].Name
			})
			return writeTable(cmd.OutOrStdout(), profiles)
		},
	}
}

func writeTable(output io.Writer, profiles []internalprofile.Profile) error {
	writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tHARNESS\tMODEL\tDESCRIPTION"); err != nil {
		return err
	}
	for _, configured := range profiles {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
			configured.Name,
			optional(configured.Defaults.Harness),
			optional(configured.Defaults.Model),
			configured.Description,
		); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func optional(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
