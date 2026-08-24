/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	versioncmd "fledge/cmd/version"

	"github.com/spf13/cobra"
)

// New constructs the root command and its CLI adapters.
func New() *cobra.Command {
	command := &cobra.Command{
		Use:   "fledge",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	command.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	versioncmd.Configure(command)

	return command
}

// Execute constructs and runs the root command.
func Execute() {
	err := New().Execute()
	if err != nil {
		os.Exit(1)
	}
}
