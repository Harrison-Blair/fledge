package doctor

import (
	"context"
	"io"
	"io/fs"
	"os"

	"github.com/Harrison-Blair/fledge/internal/agent"
	"github.com/Harrison-Blair/fledge/internal/herdr"
)

// Prober is the read-only Herdr socket boundary the diagnosis needs. A
// herdr.Client satisfies it; tests supply a deterministic double.
type Prober interface {
	Ping(context.Context) (herdr.PongResult, error)
	IntegrationList(context.Context) (herdr.IntegrationListResult, error)
}

// Environment reads process environment state without touching globals, so
// tests can drive each configuration branch deterministically.
type Environment struct {
	Getenv func(string) string
	Stat   func(string) (fs.FileInfo, error)
	Getwd  func() (string, error)
}

// LocalEnvironment reads the real process environment and filesystem.
func LocalEnvironment() Environment {
	return Environment{Getenv: os.Getenv, Stat: os.Stat, Getwd: os.Getwd}
}

// Options carries the dependencies and output target for one diagnosis.
type Options struct {
	Herdr     Prober
	Discovery agent.Discovery
	Env       Environment
	Out       io.Writer
	JSON      bool
	Verbose   bool
}

// Diagnose runs every check independently and returns the assembled report. The
// two Herdr calls happen once and their results feed the dependent checks.
func Diagnose(ctx context.Context, o Options) Report {
	pong, pingErr := o.Herdr.Ping(ctx)
	list, listErr := o.Herdr.IntegrationList(ctx)
	return newReport([]Check{
		connectivityCheck(pingErr),
		compatibilityCheck(pong, pingErr),
		harnessCheck(list, listErr),
		modelCheck(ctx, o.Discovery, list, listErr),
		configurationCheck(o.Env),
	})
}

// Run diagnoses the environment, writes the report once, and returns a typed
// error carrying exit code 1 when any check failed. Warnings never fail.
func Run(ctx context.Context, o Options) error {
	report := Diagnose(ctx, o)
	if err := report.Write(o.Out, o.JSON, o.Verbose); err != nil {
		return &OutputError{Cause: err}
	}
	if report.Summary.Fail > 0 {
		return &ReportError{Report: report}
	}
	return nil
}
