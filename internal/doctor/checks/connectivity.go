package checks

import "github.com/Harrison-Blair/fledge/internal/doctor/report"

// Connectivity reports whether ping reached the Herdr socket.
func Connectivity(pingErr error) report.Check {
	if pingErr != nil {
		return report.Check{Name: "herdr_connectivity", Status: report.Fail, Detail: "ping failed: " + pingErr.Error()}
	}
	return report.Check{Name: "herdr_connectivity", Status: report.OK, Detail: "Herdr socket responded to ping", Human: "socket responded to ping"}
}
