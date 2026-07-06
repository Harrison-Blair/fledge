package main

import (
	"os"

	"github.com/Harrison-Blair/fledge/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
