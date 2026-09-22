package doctor

import (
	"context"
	"io"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
	"github.com/Harrison-Blair/fledge/internal/lib/models"
)

// Prober is the read-only Herdr socket boundary the diagnosis needs. A
// herdr.Client satisfies it; tests supply a deterministic double.
type Prober interface {
	Ping(context.Context) (herdr.PongResult, error)
	IntegrationList(context.Context) (herdr.IntegrationListResult, error)
}

// Options carries the dependencies and output target for one diagnosis.
type Options struct {
	Herdr     Prober
	Discovery models.Discovery
	Env       checks.Environment
	Out       io.Writer
	JSON      bool
	Verbose   bool
}

// Diagnose runs every check independently and returns the assembled report. The
// two Herdr calls happen once and their results feed the dependent checks.
func Diagnose(ctx context.Context, o Options) report.Report {
	pong, pingErr := o.Herdr.Ping(ctx)
	list, listErr := o.Herdr.IntegrationList(ctx)
	return report.New([]report.Check{
		checks.Connectivity(pingErr),
		checks.Compatibility(pong, pingErr),
		checks.Harness(list, listErr),
		checks.Models(ctx, o.Discovery, list, listErr),
		checks.Configuration(o.Env),
	})
}

// Run diagnoses the environment, writes the report once, and returns a typed
// error carrying exit code 1 when any check failed. Warnings never fail.
func Run(ctx context.Context, o Options) error {
	r := Diagnose(ctx, o)
	if err := r.Write(o.Out, o.JSON, o.Verbose); err != nil {
		return &cli.OutputError{Cause: err}
	}
	if r.Summary.Fail > 0 {
		return &report.ReportError{Report: r}
	}
	return nil
}
