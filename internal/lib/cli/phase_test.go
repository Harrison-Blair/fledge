package cli

import (
	"errors"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestAtPhaseLocatesFailures(t *testing.T) {
	cause := &herdr.Error{Code: "protocol_error", Message: "incomplete worktree.list result", Uncertain: true}
	err := AtPhase("worktree.list", cause)
	o := Outcome{}
	o.Fail(err, "placement", true)
	if !errors.Is(err, cause) || err.Error() != cause.Error() || o.Error.Code != "protocol_error" || o.Error.Message != cause.Message || o.Error.Phase != "worktree.list" || o.Status != "unknown" {
		t.Fatalf("%v %s %+v", err, o.Status, o.Error)
	}
}
