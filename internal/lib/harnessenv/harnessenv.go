// Package harnessenv is where harness state lives and how harness commands run:
// a home directory and a command runner, injectable for tests.
package harnessenv

import (
	"context"
	"io"
	"os"
	"os/exec"
)

// Runner executes a harness command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Env reads harness stores under Home and runs harness commands through Run.
type Env struct {
	Home string
	Run  Runner
}

// Local reads the real home directory and executes real harness commands.
func Local() Env {
	home, _ := os.UserHomeDir()
	return Env{Home: home, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, name, args...)
		command.Stderr = io.Discard
		return command.Output()
	}}
}
