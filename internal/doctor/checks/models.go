package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/models"
)

// Models runs local model discovery per harness kind, gated on availability.
func Models(ctx context.Context, disc models.Discovery, list herdr.IntegrationListResult, listErr error) report.Check {
	availability := map[string]bool{}
	for _, in := range list.Integrations {
		availability[in.Target] = in.Available
	}
	var results []report.ModelHarness
	status := report.OK
	// Only read-only sources: doctor must never execute a harness command.
	for _, kind := range models.ReadOnlyModelHarnesses() {
		one := discoverModels(ctx, disc, kind, availability, listErr)
		results = append(results, one)
		status = report.Worst(status, one.Status)
	}
	var parts []string
	for _, r := range results {
		parts = append(parts, fmt.Sprintf("%s:%s(%d)", r.Harness, r.Status, r.Count))
	}
	return report.Check{Name: "model_discovery", Status: status, Detail: strings.Join(parts, " "), Data: report.ModelData{Harnesses: results}}
}

// discoverModels diagnoses one harness kind. A missing harness is a warning; a
// broken source for an installed harness is a failure; unknown availability
// (integration.list failed) never escalates a discovery error past a warning.
func discoverModels(ctx context.Context, disc models.Discovery, kind string, availability map[string]bool, listErr error) report.ModelHarness {
	res := report.ModelHarness{Harness: kind}
	var available *bool
	if listErr == nil {
		a := availability[kind]
		available = &a
	}
	res.Available = available
	if available != nil && !*available {
		res.Status = report.Warn
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
			res.Status = report.Warn
			res.Detail = "availability unknown; discovery error: " + err.Error()
		} else {
			res.Status = report.Fail
			res.Detail = "discovery error: " + err.Error()
		}
	case len(rows) > 0:
		res.Status = report.OK
		res.Detail = fmt.Sprintf("%d models%s", len(rows), unknown)
	default:
		res.Status = report.Warn
		res.Detail = "no models found" + unknown
	}
	return res
}
