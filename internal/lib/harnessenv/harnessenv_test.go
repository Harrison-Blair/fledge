package harnessenv

import (
	"context"
	"testing"
)

func TestLocalUsesHomeAndRunner(t *testing.T) {
	t.Setenv("HOME", "/nonexistent/fledge-home")
	e := Local()
	if e.Home != "/nonexistent/fledge-home" || e.Run == nil {
		t.Fatalf("%+v", e)
	}
	if _, err := e.Run(context.Background(), "fledge-definitely-missing-binary-9f2c"); err == nil {
		t.Fatal("missing binary succeeded")
	}
}
