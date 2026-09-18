// Package agent wires the agent command group.
package agent

import (
	"github.com/Harrison-Blair/fledge/cmd/agent/list"
	"github.com/Harrison-Blair/fledge/cmd/agent/message"
	"github.com/Harrison-Blair/fledge/cmd/agent/spawn"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Launch, list, and message Herdr agents", Args: cobra.NoArgs}
	cmd.AddCommand(spawn.New(), list.New(), message.New())
	return cmd
}
