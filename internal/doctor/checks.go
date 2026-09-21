package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// PinnedProtocol is the Herdr wire protocol number Fledge is built against.
const PinnedProtocol uint32 = 22

// CompatibilityData records the pong fields the compatibility check compared.
type CompatibilityData struct {
	Version      string              `json:"version"`
	Protocol     uint32              `json:"protocol"`
	Expected     uint32              `json:"expected"`
	Capabilities *herdr.Capabilities `json:"capabilities"`
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

// connectivityCheck reports whether ping reached the Herdr socket.
func connectivityCheck(pingErr error) Check {
	if pingErr != nil {
		return Check{Name: "herdr_connectivity", Status: Fail, Detail: "ping failed: " + pingErr.Error()}
	}
	return Check{Name: "herdr_connectivity", Status: OK, Detail: "Herdr socket responded to ping", Human: "socket responded to ping"}
}

// compatibilityCheck compares the pong protocol against the pinned value.
// A mismatch is a warning; an unreachable server leaves the protocol unknown.
func compatibilityCheck(pong herdr.PongResult, pingErr error) Check {
	if pingErr != nil {
		return Check{Name: "herdr_compatibility", Status: Warn, Detail: "protocol unknown: Herdr did not respond to ping"}
	}
	data := CompatibilityData{Version: pong.Version, Protocol: pong.Protocol, Expected: PinnedProtocol, Capabilities: pong.Capabilities}
	base := fmt.Sprintf("version=%s protocol=%d capabilities=%s", dash(pong.Version), pong.Protocol, capabilityList(pong.Capabilities))
	if pong.Protocol != PinnedProtocol {
		detail := fmt.Sprintf("protocol %d != pinned %d; %s", pong.Protocol, PinnedProtocol, base)
		// The human diagnostic joins capabilities with ", " so the renderer can
		// wrap between capability names; the JSON detail keeps the comma-joined
		// form byte-for-byte.
		humanBase := fmt.Sprintf("version=%s protocol=%d capabilities=%s", dash(pong.Version), pong.Protocol, capabilityListSpaced(pong.Capabilities))
		human := fmt.Sprintf("protocol %d != pinned %d; %s", pong.Protocol, PinnedProtocol, humanBase)
		return Check{Name: "herdr_compatibility", Status: Warn, Detail: detail, Human: human, Data: data}
	}
	return Check{Name: "herdr_compatibility", Status: OK, Detail: base, Data: data}
}

// capabilityTokens lists the enabled capability flags as individual tokens, in
// stable order. The result is empty when nothing is enabled.
func capabilityTokens(c *herdr.Capabilities) []string {
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

// capabilityList renders the enabled capability flags for the stored JSON detail
// line, comma-joined, with "none" when nothing is enabled.
func capabilityList(c *herdr.Capabilities) string {
	on := capabilityTokens(c)
	if len(on) == 0 {
		return "none"
	}
	return strings.Join(on, ",")
}

// capabilityListSpaced renders the enabled capability flags for a human
// diagnostic, joined with ", " so the renderer can wrap between names without
// splitting one. It returns "none" when nothing is enabled.
func capabilityListSpaced(c *herdr.Capabilities) string {
	on := capabilityTokens(c)
	if len(on) == 0 {
		return "none"
	}
	return strings.Join(on, ", ")
}

// harnessCheck enumerates integration targets and their availability.
func harnessCheck(list herdr.IntegrationListResult, listErr error) Check {
	if listErr != nil {
		return Check{Name: "harness_installations", Status: Fail, Detail: "integration.list failed: " + listErr.Error()}
	}
	var available []string
	for _, in := range list.Integrations {
		if in.Available {
			available = append(available, in.Target)
		}
	}
	data := HarnessData{Integrations: list.Integrations}
	if len(available) == 0 {
		return Check{Name: "harness_installations", Status: Warn, Detail: fmt.Sprintf("%d targets, none available", len(list.Integrations)), Data: data}
	}
	return Check{Name: "harness_installations", Status: OK, Detail: fmt.Sprintf("%d targets, %d available: %s", len(list.Integrations), len(available), strings.Join(available, ", ")), Data: data}
}

// modelCheck runs local model discovery per harness kind, gated on availability.
func modelCheck(ctx context.Context, disc agent.Discovery, list herdr.IntegrationListResult, listErr error) Check {
	availability := map[string]bool{}
	for _, in := range list.Integrations {
		availability[in.Target] = in.Available
	}
	var results []ModelHarness
	status := OK
	// Only read-only sources: doctor must never execute a harness command.
	for _, kind := range agent.ReadOnlyModelHarnesses() {
		one := discoverModels(ctx, disc, kind, availability, listErr)
		results = append(results, one)
		status = worst(status, one.Status)
	}
	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("%s:%s(%d)", r.Harness, r.Status, r.Count))
	}
	return Check{Name: "model_discovery", Status: status, Detail: strings.Join(parts, " "), Data: ModelData{Harnesses: results}}
}

// discoverModels diagnoses one harness kind. A missing harness is a warning; a
// broken source for an installed harness is a failure; unknown availability
// (integration.list failed) never escalates a discovery error past a warning.
func discoverModels(ctx context.Context, disc agent.Discovery, kind string, availability map[string]bool, listErr error) ModelHarness {
	res := ModelHarness{Harness: kind}
	var available *bool
	if listErr == nil {
		a := availability[kind]
		available = &a
	}
	res.Available = available
	if available != nil && !*available {
		res.Status = Warn
		res.Detail = "harness not installed"
		return res
	}
	rows, err := disc.Discover(ctx, kind)
	res.Count = len(rows)
	unknown := ""
	if available == nil {
		unknown = " (availability unknown)"
	}
	switch {
	case err != nil:
		if available == nil {
			res.Status = Warn
			res.Detail = "availability unknown; discovery error: " + err.Error()
		} else {
			res.Status = Fail
			res.Detail = "discovery error: " + err.Error()
		}
	case len(rows) > 0:
		res.Status = OK
		res.Detail = fmt.Sprintf("%d models%s", len(rows), unknown)
	default:
		res.Status = Warn
		res.Detail = "no models found" + unknown
	}
	return res
}

// configurationCheck inspects the Herdr environment variables and socket mode.
func configurationCheck(e Environment) Check {
	herdrEnv := e.Getenv("HERDR_ENV")
	socketPath := e.Getenv("HERDR_SOCKET_PATH")
	paneID := e.Getenv("HERDR_PANE_ID")
	session := e.Getenv("HERDR_SESSION")
	cwd, _ := e.Getwd()

	status := OK
	if herdrEnv != "1" {
		status = Fail
	}

	socketStatus, mode, socketDetail := socketStatus(e, socketPath)
	status = worst(status, socketStatus)

	if paneID == "" {
		status = worst(status, Warn)
	}

	data := ConfigData{HerdrEnv: herdrEnv, SocketPath: socketPath, SocketMode: mode, PaneID: paneID, Session: session, Cwd: cwd, SocketField: socketDetail}
	detail := fmt.Sprintf("HERDR_ENV=%s socket=%s pane=%s session=%s cwd=%s", dash(herdrEnv), socketDetail, dash(paneID), dash(session), dash(cwd))
	return Check{Name: "configuration", Status: status, Detail: detail, Data: data}
}

// socketStatus stats the socket path and reports its status, octal mode, and a
// detail fragment describing the path.
func socketStatus(e Environment, path string) (Status, string, string) {
	if path == "" {
		return Fail, "", "unset"
	}
	info, err := e.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Fail, "", path + " (missing)"
		}
		// A permission or transient stat error leaves the mode unknown, not gone.
		return Warn, "", fmt.Sprintf("%s (mode unknown: %v)", path, err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		return Fail, "", path + " (not a socket)"
	}
	mode := fmt.Sprintf("%04o", info.Mode().Perm())
	if info.Mode().Perm() == 0o600 {
		return OK, mode, fmt.Sprintf("%s (%s)", path, mode)
	}
	return Warn, mode, fmt.Sprintf("%s (mode %s)", path, mode)
}

// dash renders an empty value as "-" for human output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
