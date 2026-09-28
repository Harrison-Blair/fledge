// Package memory wires the memory command group.
package memory

import (
	"github.com/Harrison-Blair/fledge/cmd/memory/add"
	"github.com/Harrison-Blair/fledge/cmd/memory/get"
	"github.com/Harrison-Blair/fledge/cmd/memory/list"
	"github.com/Harrison-Blair/fledge/cmd/memory/remove"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Add, list, read, and remove repository memories", Args: cobra.NoArgs,
		Long: "Add, list, read, and remove repository memories: durable, non-obvious facts\nabout the project, Fledge, and Herdr that agents share.\n\nEach memory is one Markdown file in .fledge/memories under the repository's\nprimary checkout, with name, description, and type frontmatter. MEMORY.md\nbeside them indexes every memory, one line each, and is regenerated after\nevery add and remove; never edit it by hand. Profile spawns inject the index\ninto the worker's brief. These commands never contact Herdr."}
	cmd.AddCommand(add.New(), list.New(), get.New(), remove.New())
	return cmd
}
