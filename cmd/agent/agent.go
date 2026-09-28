// Package agent wires the agent command group.
package agent

import (
	"github.com/Harrison-Blair/fledge/cmd/agent/adopt"
	"github.com/Harrison-Blair/fledge/cmd/agent/capabilities"
	"github.com/Harrison-Blair/fledge/cmd/agent/cleanup"
	"github.com/Harrison-Blair/fledge/cmd/agent/current"
	"github.com/Harrison-Blair/fledge/cmd/agent/get"
	"github.com/Harrison-Blair/fledge/cmd/agent/list"
	"github.com/Harrison-Blair/fledge/cmd/agent/message"
	"github.com/Harrison-Blair/fledge/cmd/agent/models"
	"github.com/Harrison-Blair/fledge/cmd/agent/pause"
	"github.com/Harrison-Blair/fledge/cmd/agent/profiles"
	"github.com/Harrison-Blair/fledge/cmd/agent/read"
	"github.com/Harrison-Blair/fledge/cmd/agent/rename"
	"github.com/Harrison-Blair/fledge/cmd/agent/send"
	"github.com/Harrison-Blair/fledge/cmd/agent/spawn"
	"github.com/Harrison-Blair/fledge/cmd/agent/stop"
	"github.com/Harrison-Blair/fledge/cmd/agent/usage"
	"github.com/Harrison-Blair/fledge/cmd/agent/wait"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "Launch, adopt, rename, list, inspect, identify the caller, message, send input to, pause, stop, read, wait for, clean up, report usage of, discover models for, report capabilities of, and list launch profiles for Herdr agents", Args: cobra.NoArgs}
	cmd.AddCommand(spawn.New(), adopt.New(), rename.New(), list.New(), get.New(), current.New(), message.New(), send.New(), pause.New(), stop.New(), cleanup.New(), models.New(), capabilities.New(), profiles.New(), read.New(), wait.New(), usage.New())
	return cmd
}
