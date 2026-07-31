package fledge

import (
	"strings"
	"testing"
	"time"
)

func TestProtocolMismatchIsActionable(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	fake.mu.Lock()
	fake.pongProtocol = 18
	fake.mu.Unlock()
	_, err := service.Status(t.Context())
	if Translate(err).Code != "protocol_mismatch" || !strings.Contains(err.Error(), "stop and restart") {
		t.Fatalf("unexpected mismatch error: %v", err)
	}
}

func TestStartAndStatusReportSessionSource(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	service.Project.SessionSource = "workspace"
	started, err := service.Start(t.Context(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if started.SessionSource != "workspace" {
		t.Fatalf("start source = %q", started.SessionSource)
	}
	status, err := service.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionSource != "workspace" {
		t.Fatalf("status source = %q", status.SessionSource)
	}
}
