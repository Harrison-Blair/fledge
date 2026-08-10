package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/Harrison-Blair/fledge/internal/harness"
	"github.com/spf13/cobra"
)

type catalogDiscoverer func(context.Context, harness.Harness, harness.DiscoveryOptions) harness.Catalog

func newAgentModelsCommand(lookPath harness.LookPath, discover catalogDiscoverer) *cobra.Command {
	if discover == nil {
		discover = harness.Discover
	}
	return &cobra.Command{
		Use:   "models [harness]",
		Short: "List advisory model catalogs for installed harnesses",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			installed := harness.Installed(lookPath)
			if len(installed) == 0 {
				return fmt.Errorf("no supported agent harnesses are installed")
			}

			selected := installed
			if len(args) == 1 {
				resolved, ok := harness.Resolve(installed, args[0])
				if !ok {
					return fmt.Errorf("requested harness %q is not installed", args[0])
				}
				selected = []harness.Harness{resolved}
			}

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "HARNESS\tPROVIDER / INTEGRATION\tMODEL\tNAME\tDESCRIPTION"); err != nil {
				return err
			}
			for _, selectedHarness := range selected {
				catalog := discover(cmd.Context(), selectedHarness, harness.DiscoveryOptions{})
				if catalog.Warning != "" {
					if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", catalog.Warning); err != nil {
						return err
					}
				}
				for _, model := range catalog.Models {
					if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
						modelCell(selectedHarness.Name), modelGroup(selectedHarness, model), modelValue(model),
						modelCell(model.Name), modelCell(model.Description)); err != nil {
						return err
					}
				}
			}
			return writer.Flush()
		},
	}
}

func modelGroup(selected harness.Harness, model harness.Model) string {
	if model.Provider == "" {
		return modelCell(selected.Name)
	}
	return modelCell(harness.ProviderName(model.Provider))
}

func modelValue(model harness.Model) string {
	if model.ID == "" {
		return "(default)"
	}
	return modelCell(model.ID)
}

func modelCell(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
