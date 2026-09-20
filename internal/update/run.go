package update

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/version"
	"golang.org/x/term"
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
	src := Source{BaseURL: o.BaseURL, Client: o.Client}
	latest, err := src.LatestTag(ctx)
	if err != nil {
		return err
	}
	cmp, compareErr := CompareVersions(o.Current, latest)
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
		terminal := false
		if o.IsTerminal != nil {
			terminal = o.IsTerminal()
		} else if f, ok := o.Stdin.(*os.File); ok {
			terminal = term.IsTerminal(int(f.Fd()))
		}
		if !terminal {
			return fmt.Errorf("update: stdin is not a terminal; use --yes to update without a prompt")
		}
		if _, err := fmt.Fprintf(o.Err, "Install fledge %s over %s? [y/N] ", latest, o.Current); err != nil {
			return err
		}
		answer, err := bufio.NewReader(o.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("update: read confirmation: %w", err)
		}
		if strings.TrimSpace(answer) != "y" && strings.TrimSpace(answer) != "Y" {
			_, err := fmt.Fprintln(o.Err, "update canceled")
			return err
		}
	}
	executable := o.ExecPath
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("update: resolve executable: %w", err)
	}
	bin, err := src.binary(ctx, latest, o.GOARCH)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replace(executable, bin); err != nil {
		return err
	}
	_, err = fmt.Fprintf(o.Out, "fledge updated to %s\n", latest)
	return err
}

func replace(executable string, binary []byte) error {
	info, err := os.Stat(executable)
	if err != nil {
		return fmt.Errorf("update: stat executable: %w", err)
	}
	dir := filepath.Dir(executable)
	f, err := os.CreateTemp(dir, ".fledge-update-*")
	if err != nil {
		return fmt.Errorf("update: cannot write to %s; use an account with write permission: %w", dir, err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(binary); err != nil {
		return fmt.Errorf("update: write executable: %w", err)
	}
	if err := f.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), executable); err != nil {
		return fmt.Errorf("update: replace executable: %w", err)
	}
	return nil
}
