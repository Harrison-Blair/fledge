// Package agent wires the agent command group.
package agent

import (
	"github.com/Harrison-Blair/fledge/cmd/agent/adopt"
	"github.com/Harrison-Blair/fledge/cmd/agent/current"
	"github.com/Harrison-Blair/fledge/cmd/agent/get"
	"github.com/Harrison-Blair/fledge/cmd/agent/list"
	"github.com/Harrison-Blair/fledge/cmd/agent/message"
	"github.com/Harrison-Blair/fledge/cmd/agent/models"
	"github.com/Harrison-Blair/fledge/cmd/agent/pause"
	"github.com/Harrison-Blair/fledge/cmd/agent/read"
	"github.com/Harrison-Blair/fledge/cmd/agent/spawn"
	"github.com/Harrison-Blair/fledge/cmd/agent/stop"
	"github.com/Harrison-Blair/fledge/cmd/agent/wait"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Launch, adopt, list, inspect, identify the caller, message, pause, stop, read, wait for, and discover models for Herdr agents", Args: cobra.NoArgs}
	cmd.AddCommand(spawn.New(), adopt.New(), list.New(), get.New(), current.New(), message.New(), pause.New(), stop.New(), models.New(), read.New(), wait.New())
	return cmd
}
