package checks

import (
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// PinnedProtocol is the Herdr wire protocol number Fledge is built against.
const PinnedProtocol uint32 = 22

// Compatibility compares the pong protocol against the pinned value.
// A mismatch is a warning; an unreachable server leaves the protocol unknown.
func Compatibility(pong herdr.PongResult, pingErr error) report.Check {
	if pingErr != nil {
		return report.Check{Name: "herdr_compatibility", Status: report.Warn, Detail: "protocol unknown: Herdr did not respond to ping"}
	}
	data := report.CompatibilityData{Version: pong.Version, Protocol: pong.Protocol, Expected: PinnedProtocol, Capabilities: pong.Capabilities}
	names := data.CapabilityNames()
	base := fmt.Sprintf("version=%s protocol=%d capabilities=%s", dash(pong.Version), pong.Protocol, capabilityList(names, ","))
	if pong.Protocol != PinnedProtocol {
		detail := fmt.Sprintf("protocol %d != pinned %d; %s", pong.Protocol, PinnedProtocol, base)
		// The human diagnostic joins capabilities with ", " so the renderer can
		// wrap between capability names; the JSON detail keeps the comma-joined
		// form byte-for-byte.
		humanBase := fmt.Sprintf("version=%s protocol=%d capabilities=%s", dash(pong.Version), pong.Protocol, capabilityList(names, ", "))
		human := fmt.Sprintf("protocol %d != pinned %d; %s", pong.Protocol, PinnedProtocol, humanBase)
		return report.Check{Name: "herdr_compatibility", Status: report.Warn, Detail: detail, Human: human, Data: data}
	}
	return report.Check{Name: "herdr_compatibility", Status: report.OK, Detail: base, Data: data}
}

// capabilityList joins the enabled capability names with sep, with "none" when
// nothing is enabled.
func capabilityList(names []string, sep string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, sep)
}

// dash renders an empty value as "-" for human output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
