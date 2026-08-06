package lifecycle

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// Color is a property of where the trace is going, not of what it says: a
// redirected trace and a machine-readable one both have to stay plain.
func TestColorEnabledRequiresATerminalAndNoOverride(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		output     io.Writer
		jsonMode   bool
		noColor    string
		terminal   bool
		want       bool
		wantProbes int
	}{
		{name: "terminal", output: os.Stdout, terminal: true, want: true, wantProbes: 1},
		{name: "NO_COLOR override", output: os.Stdout, noColor: "1", terminal: true},
		{name: "JSON", output: os.Stdout, jsonMode: true, terminal: true},
		{name: "non-terminal file", output: os.Stdout, terminal: false, wantProbes: 1},
		{name: "non-file writer", output: &bytes.Buffer{}, terminal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probes := 0
			got := colorEnabled(test.output, test.jsonMode,
				func(string) string { return test.noColor },
				func(*os.File) bool {
					probes++
					return test.terminal
				})
			if got != test.want {
				t.Fatalf("colorEnabled() = %v, want %v", got, test.want)
			}
			if probes != test.wantProbes {
				t.Fatalf("terminal probes = %d, want %d", probes, test.wantProbes)
			}
		})
	}
}

func TestWatchPassesTheOutputSelectionToTheWatcher(t *testing.T) {
	t.Parallel()

	client := &fakeHerdr{}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	root := t.TempDir()
	writeTestRecord(t, root)
	var seen watchproc.Options
	manager.watchRunner = func(_ context.Context, options watchproc.Options) error {
		seen = options
		return nil
	}
	if err := manager.Watch(context.Background(), root, WatchOptions{JSON: true}); err != nil {
		t.Fatal(err)
	}
	if !seen.JSON || seen.Color {
		t.Fatalf("watcher options = %#v, want JSON without color", seen)
	}
}
