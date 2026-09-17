// Command fledge is the Fledge CLI.
package main

import (
	"os"

	"github.com/Harrison-Blair/fledge/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
