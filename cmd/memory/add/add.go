// Package add wires memory addition.
package add

import (
	"os"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/memory/add"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	var options add.Options
	var asJSON bool
	cmd := &cobra.Command{Use: "add", Short: "Record a new memory", Args: cobra.NoArgs,
		Long: "Record one durable, non-obvious fact as a new memory and regenerate the index.\n\nThe name is a kebab-case slug that becomes the file name; an existing name fails\nwith memory_exists instead of being replaced. The description is the one line\nshown in the index."}
	f := cmd.Flags()
	f.StringVar(&options.Name, "name", "", "Kebab-case memory name")
	f.StringVar(&options.Description, "description", "", "One-line summary shown in the index")
	f.StringVar(&options.Type, "type", "", "Memory type: "+strings.Join(memory.Types, ", "))
	f.StringVar(&options.Body, "body", "", "Markdown body")
	f.StringVar(&options.File, "file", "", "UTF-8 body file, or - for stdin")
	f.BoolVar(&asJSON, "json", false, "Emit a structured outcome")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		options.BodySet = f.Changed("body")
		options.FileSet = f.Changed("file")
		cwd, _ := os.Getwd()
		return libagent.Finish(add.Run(cmd.Context(), cwd, options, cmd.InOrStdin()), cmd.OutOrStdout(), asJSON, add.Render)
	}
	return cmd
}
