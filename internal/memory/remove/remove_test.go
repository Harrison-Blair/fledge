package remove

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/testutil/identitytest"
)

func TestRemoveDeletesMemory(t *testing.T) {
	root := identitytest.Repository(t)
	m := memory.Memory{Name: "herdr-socket", Description: "d", Type: "project", Body: "b\n"}
	if err := memory.Add(context.Background(), root, m, &libagent.Outcome{}); err != nil {
		t.Fatal(err)
	}
	out := Run(context.Background(), root, Options{Name: "herdr-socket"})
	if out.Error != nil || out.Operation != "memory.remove" || out.Result != (Result{Name: "herdr-socket"}) {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	dir := filepath.Join(root, ".fledge", "memories")
	if _, err := os.Stat(filepath.Join(dir, "herdr-socket.md")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, memory.IndexName)); err != nil || len(b) != 0 {
		t.Fatalf("%q %v", b, err)
	}
	var b bytes.Buffer
	if err := out.Write(&b, false, Render); err != nil || b.String() != "Removed memory herdr-socket\n" {
		t.Fatalf("%q %v", b.String(), err)
	}
}

func TestRemoveUnknownNameFails(t *testing.T) {
	root := identitytest.Repository(t)
	out := Run(context.Background(), root, Options{Name: "herdr-socket"})
	if out.Error == nil || out.Error.Code != "memory_not_found" || out.Error.Phase != "memory" {
		t.Fatalf("%+v %+v", out, out.Error)
	}
	out = Run(context.Background(), root, Options{Name: "../x"})
	if out.Error == nil || out.Error.Code != "invalid_input" {
		t.Fatalf("%+v", out.Error)
	}
}
