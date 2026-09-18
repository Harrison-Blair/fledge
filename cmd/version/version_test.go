package version

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"

	internalversion "github.com/Harrison-Blair/fledge/internal/version"
)

func TestVersionFlags(t *testing.T) {
	for _, flag := range []string{"--version", "-V"} {
		root := &cobra.Command{Use: "fledge"}
		Configure(root)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs([]string{flag})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "fledge "+internalversion.Version()+"\n"; got != want {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}
