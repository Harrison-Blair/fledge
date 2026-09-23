package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/Harrison-Blair/fledge/internal/lib/version"
	"github.com/Harrison-Blair/fledge/internal/update/archive"
	"github.com/Harrison-Blair/fledge/internal/update/confirm"
	"github.com/Harrison-Blair/fledge/internal/update/install"
	"github.com/Harrison-Blair/fledge/internal/update/release"
)

// Options supplies command inputs and the external dependencies used by Run.
type Options struct {
	Check, Yes                               bool
	Current, BaseURL, GOOS, GOARCH, ExecPath string
	Client                                   *http.Client
	Stdin                                    io.Reader
	Out, Err                                 io.Writer
	IsTerminal                               func() bool
}

// Run checks for or installs the latest stable release. Declining is a successful
// no-op; an unknown development version requires the same explicit confirmation.
func Run(ctx context.Context, o Options) error {
	if o.Current == "" {
		o.Current = version.Version()
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Out == nil {
		o.Out = os.Stdout
	}
	if o.Err == nil {
		o.Err = os.Stderr
	}
	if o.GOOS != "linux" || (o.GOARCH != "amd64" && o.GOARCH != "arm64") {
		return fmt.Errorf("update: unsupported platform %s/%s; releases support Linux amd64 and arm64", o.GOOS, o.GOARCH)
	}
	src := release.Source{BaseURL: o.BaseURL, Client: o.Client}
	latest, err := src.LatestTag(ctx)
	if err != nil {
		return err
	}
	cmp, compareErr := release.CompareVersions(o.Current, latest)
	if compareErr != nil {
		if _, err := fmt.Fprintf(o.Err, "warning: cannot compare development build %q with the latest release\n", o.Current); err != nil {
			return err
		}
	} else if cmp >= 0 {
		_, err := fmt.Fprintf(o.Out, "fledge %s is up to date\n", o.Current)
		return err
	}
	if o.Check {
		_, err := fmt.Fprintf(o.Out, "fledge %s is available (you have %s); run fledge update to install it\n", latest, o.Current)
		return err
	}
	if !o.Yes {
		proceed, err := confirm.Proceed(o.Stdin, o.Err, o.IsTerminal, latest, o.Current)
		if err != nil || !proceed {
			return err
		}
	}
	executable, err := install.ResolveExecutable(o.ExecPath)
	if err != nil {
		return err
	}
	asset, err := src.Artifact(ctx, latest, o.GOARCH)
	if err != nil {
		return err
	}
	bin, err := archive.ExtractFledge(asset)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := install.Replace(executable, bin); err != nil {
		return err
	}
	_, err = fmt.Fprintf(o.Out, "fledge updated to %s\n", latest)
	return err
}
