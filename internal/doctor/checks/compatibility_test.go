package checks_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestCompatibilityOK(t *testing.T) {
	c := checks.Compatibility(pong(22), nil)
	if c.Name != "herdr_compatibility" || c.Status != report.OK {
		t.Fatalf("compatibility %+v", c)
	}
	if c.Detail != "version=0.9.1 protocol=22 capabilities=live_handoff,health_check" || c.Human != "" {
		t.Fatalf("detail = %q, human = %q", c.Detail, c.Human)
	}
	data, ok := c.Data.(report.CompatibilityData)
	if !ok || data.Expected != checks.PinnedProtocol || data.Protocol != 22 || data.Version != "0.9.1" || data.Capabilities == nil {
		t.Fatalf("data = %#v", c.Data)
	}
}

func TestCompatibilityUnknownWhenPingFails(t *testing.T) {
	c := checks.Compatibility(herdr.PongResult{}, errors.New("connection refused"))
	if c.Status != report.Warn || c.Detail != "protocol unknown: Herdr did not respond to ping" || c.Data != nil {
		t.Fatalf("compatibility %+v", c)
	}
}

func TestCompatibilityMismatchIsWarnNotFail(t *testing.T) {
	c := checks.Compatibility(pong(21), nil)
	if c.Status != report.Warn || !strings.Contains(c.Detail, "22") {
		t.Fatalf("compatibility %+v", c)
	}
}

func TestCompatibilityMismatchDetailAndHuman(t *testing.T) {
	// The JSON detail stays comma-joined; the human diagnostic joins with ", "
	// so the renderer can wrap between capability names.
	full := &herdr.Capabilities{LiveHandoff: true, DetachedServerDaemon: true, HealthCheck: true, SurfaceInterest: true, EndpointProtocolGeneration: u32(1)}
	c := checks.Compatibility(herdr.PongResult{Type: "pong", Version: "0.9.1", Protocol: 21, Capabilities: full}, nil)
	if want := "protocol 21 != pinned 22; version=0.9.1 protocol=21 capabilities=live_handoff,detached_server_daemon,health_check,surface_interest,endpoint_protocol_generation=1"; c.Detail != want {
		t.Fatalf("detail = %q, want %q", c.Detail, want)
	}
	if want := "protocol 21 != pinned 22; version=0.9.1 protocol=21 capabilities=live_handoff, detached_server_daemon, health_check, surface_interest, endpoint_protocol_generation=1"; c.Human != want {
		t.Fatalf("human = %q, want %q", c.Human, want)
	}
	if _, ok := c.Data.(report.CompatibilityData); !ok {
		t.Fatalf("data = %#v", c.Data)
	}
}

func TestCompatibilityNoCapabilities(t *testing.T) {
	c := checks.Compatibility(herdr.PongResult{Protocol: 22}, nil)
	if c.Detail != "version=- protocol=22 capabilities=none" {
		t.Fatalf("detail = %q", c.Detail)
	}
	c = checks.Compatibility(herdr.PongResult{Protocol: 21}, nil)
	if c.Human != "protocol 21 != pinned 22; version=- protocol=21 capabilities=none" {
		t.Fatalf("human = %q", c.Human)
	}
}
