// Package update wires the explicit release updater.
package update

import (
	"github.com/Harrison-Blair/fledge/internal/update"
	"github.com/spf13/cobra"
)

// New returns a fresh update command.
func New() *cobra.Command {
	var options update.Options
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Install the latest stable Fledge release",
		Long:  "Download and verify the latest Linux release, then replace the running executable.\nConfirmation is required unless --yes is supplied; --check only reports availability.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options.Stdin, options.Out, options.Err = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			return update.Run(cmd.Context(), options)
		},
	}
	cmd.Flags().BoolVar(&options.Check, "check", false, "Only check for a newer release")
	cmd.Flags().BoolVar(&options.Yes, "yes", false, "Install without asking for confirmation")
	return cmd
}
