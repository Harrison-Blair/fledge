package report

import (
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// CompatibilityData records the pong fields the compatibility check compared.
type CompatibilityData struct {
	Version      string              `json:"version"`
	Protocol     uint32              `json:"protocol"`
	Expected     uint32              `json:"expected"`
	Capabilities *herdr.Capabilities `json:"capabilities"`
}

// CapabilityNames lists the enabled capability flags as individual tokens, in
// stable order. The result is empty when nothing is enabled.
func (d CompatibilityData) CapabilityNames() []string {
	c := d.Capabilities
	if c == nil {
		return nil
	}
	var on []string
	if c.LiveHandoff {
		on = append(on, "live_handoff")
	}
	if c.DetachedServerDaemon {
		on = append(on, "detached_server_daemon")
	}
	if c.HealthCheck {
		on = append(on, "health_check")
	}
	if c.SurfaceInterest {
		on = append(on, "surface_interest")
	}
	if c.EndpointProtocolGeneration != nil {
		on = append(on, fmt.Sprintf("endpoint_protocol_generation=%d", *c.EndpointProtocolGeneration))
	}
	return on
}

// HarnessData lists every integration target and its install state.
type HarnessData struct {
	Integrations []herdr.IntegrationInfo `json:"integrations"`
}

// ModelHarness is one harness kind's model-discovery result.
type ModelHarness struct {
	Harness   string `json:"harness"`
	Available *bool  `json:"available"`
	Count     int    `json:"count"`
	Status    Status `json:"status"`
	Detail    string `json:"detail"`
}

// ModelData collects per-harness model-discovery results.
type ModelData struct {
	Harnesses []ModelHarness `json:"harnesses"`
}

// ConfigData records the environment values the configuration check inspected.
type ConfigData struct {
	HerdrEnv   string `json:"herdr_env"`
	SocketPath string `json:"herdr_socket_path"`
	SocketMode string `json:"socket_mode"`
	PaneID     string `json:"herdr_pane_id"`
	Session    string `json:"herdr_session"`
	Cwd        string `json:"cwd"`
	// SocketField is the human socket fragment (path plus mode, or a state such
	// as "unset"/"(missing)"). It is derived for the grouped human report only
	// and is never serialized, so the JSON document is unaffected.
	SocketField string `json:"-"`
}
