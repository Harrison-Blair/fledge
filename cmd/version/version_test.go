package version

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	internalversion "fledge/internal/version"

	"github.com/spf13/cobra"
)

func TestVersionFlags(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		t.Run(flag, func(t *testing.T) {
			command := &cobra.Command{Use: "fledge"}
			Configure(command)

			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs([]string{flag})

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			want := fmt.Sprintf("fledge version %s\n", internalversion.Version())
			if got := output.String(); got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestVersionFlagIsShownInHelp(t *testing.T) {
	command := &cobra.Command{
		Use: "fledge",
		Run: func(*cobra.Command, []string) {},
	}
	Configure(command)

	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := output.String(); !strings.Contains(got, "-V, --version") {
		t.Fatalf("help output does not advertise -V, --version:\n%s", got)
	}
}
