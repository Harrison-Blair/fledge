// Package agent wires the agent command group.
package agent

import (
	"github.com/Harrison-Blair/fledge/cmd/agent/get"
	"github.com/Harrison-Blair/fledge/cmd/agent/list"
	"github.com/Harrison-Blair/fledge/cmd/agent/message"
	"github.com/Harrison-Blair/fledge/cmd/agent/models"
	"github.com/Harrison-Blair/fledge/cmd/agent/spawn"
	"github.com/Harrison-Blair/fledge/cmd/agent/stop"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Launch, list, inspect, message, stop, and discover models for Herdr agents", Args: cobra.NoArgs}
	cmd.AddCommand(spawn.New(), list.New(), get.New(), message.New(), stop.New(), models.New())
	return cmd
}
