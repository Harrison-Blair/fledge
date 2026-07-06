package main

import (
	"os"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/cli"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"fledge": func() { os.Exit(cli.Run(os.Args[1:])) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(env *testscript.Env) error {
			// Keep git deterministic and identity-free inside scripts.
			env.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
			env.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
			env.Setenv("GIT_AUTHOR_NAME", "test")
			env.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
			env.Setenv("GIT_COMMITTER_NAME", "test")
			env.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
			return nil
		},
	})
}
