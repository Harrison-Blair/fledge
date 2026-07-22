package filebridge

import (
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestRequestResponseLifecycle(t *testing.T) {
	root := t.TempDir()
	if err := ResetServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { CloseServer(root, "alpha") })

	id, err := Submit(root, "alpha", protocol.Request{Op: protocol.OpSpawn, Model: "gpt-x"})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := Take(root, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != id || pending[0].Request.Op != protocol.OpSpawn {
		t.Fatalf("pending = %+v", pending)
	}
	if err := Respond(root, "alpha", id, protocol.Response{Name: "pi-emperor"}); err != nil {
		t.Fatal(err)
	}
	resp, err := Await(root, "alpha", id, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "pi-emperor" {
		t.Fatalf("response = %+v", resp)
	}

	if err := CloseServer(root, "alpha"); err != nil {
		t.Fatal(err)
	}
	if Available(root, "alpha") {
		t.Fatal("closed bridge still reports available")
	}
}
