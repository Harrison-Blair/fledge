package get

import (
	"bytes"
	"context"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

var m = memory.Memory{Name: "herdr-socket", Description: "Herdr commands need socket access", Type: "project", Body: "Use the socket.\n"}

func TestGetPrintsFullMemory(t *testing.T) {
	root := identitytest.Repository(t)
	if err := memory.Add(context.Background(), root, m, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), root, Options{Name: "herdr-socket"})
	if out.Error != nil || out.Operation != "memory.get" || out.Result != m {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != string(memory.Format(m)) {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestGetUnknownNameFails(t *testing.T) {
	root := identitytest.Repository(t)
	out := Run(context.Background(), root, Options{Name: "herdr-socket"})
	if out.Error == nil || out.Error.Code != "memory_not_found" || out.Error.Phase != "memory" || out.ExitCode() != 1 {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	var b bytes.Buffer
	if out.Write(&b, false, Render); b.String() != "rejected: memory_not_found: no memory named herdr-socket (memory)\n" {
		t.Fatalf("%q", b.String())
	}
	out = Run(context.Background(), root, Options{})
	if out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out.Error)
	}
}
