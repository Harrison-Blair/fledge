package checks

import (
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Harness enumerates integration targets and their availability.
func Harness(list herdr.IntegrationListResult, listErr error) report.Check {
	if listErr != nil {
		return report.Check{Name: "harness_installations", Status: report.Fail, Detail: "integration.list failed: " + listErr.Error()}
	}
	var available []string
	for _, in := range list.Integrations {
		if in.Available {
			available = append(available, in.Target)
		}
	}
	data := report.HarnessData{Integrations: list.Integrations}
	if len(available) == 0 {
		return report.Check{Name: "harness_installations", Status: report.Warn, Detail: fmt.Sprintf("%d targets, none available", len(list.Integrations)), Data: data}
	}
	return report.Check{Name: "harness_installations", Status: report.OK, Detail: fmt.Sprintf("%d targets, %d available: %s", len(list.Integrations), len(available), strings.Join(available, ", ")), Data: data}
}
