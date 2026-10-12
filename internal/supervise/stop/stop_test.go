package stop

import (
	"bytes"
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/herdrscript"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestStopWithoutDaemon(t *testing.T) {
	t.Setenv("HERDR_SESSION", "work")
	out := Run(context.Background(), Options{Cwd: identitytest.Repository(t)})
	var b bytes.Buffer
	out.Write(&b, false, Render)
	if out.Error != nil || out.Result.(Result).Stopped || b.String() != "Supervisor for session work was not running.\n" {
		t.Fatalf("%q %+v", b.String(), out.Error)
	}
}

func TestStopOutsideRepository(t *testing.T) {
	out := Run(context.Background(), Options{Cwd: t.TempDir()})
	if out.Error == nil {
		t.Fatalf("%+v", out)
	}
}

func TestOutputFailuresPropagate(t *testing.T) {
	herdrscript.CheckOutputFailures(t, Render, cli.Outcome{Result: Result{Session: "s", Stopped: true}})
}
