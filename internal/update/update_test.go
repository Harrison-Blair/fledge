package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tarball(t *testing.T, name, body string, kind byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	h := &tar.Header{Name: name, Mode: 0755, Typeflag: kind, Size: int64(len(body))}
	if kind == tar.TypeSymlink {
		h.Linkname, h.Size = "/tmp/elsewhere", 0
		body = ""
	}
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fixture(t *testing.T, asset []byte, checksum string) (Options, *bytes.Buffer, *bytes.Buffer, *int) {
	t.Helper()
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Harrison-Blair/fledge/releases/latest":
			w.Header().Set("Location", "/Harrison-Blair/fledge/releases/tag/v0.2.0")
			w.WriteHeader(http.StatusFound)
		case "/Harrison-Blair/fledge/releases/download/v0.2.0/checksums.txt":
			downloads++
			if checksum == "" {
				checksum = fmt.Sprintf("%x", sha256.Sum256(asset))
			}
			fmt.Fprintf(w, "%s  fledge_v0.2.0_linux_amd64.tar.gz\n", checksum)
		case "/Harrison-Blair/fledge/releases/download/v0.2.0/fledge_v0.2.0_linux_amd64.tar.gz":
			downloads++
			_, _ = w.Write(asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	exe := filepath.Join(t.TempDir(), "fledge")
	if err := os.WriteFile(exe, []byte("old binary"), 0751); err != nil {
		t.Fatal(err)
	}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	return Options{Current: "v0.1.0", BaseURL: srv.URL, Client: srv.Client(), GOOS: "linux", GOARCH: "amd64", ExecPath: exe, Stdin: strings.NewReader("y\n"), Out: out, Err: errOut, IsTerminal: func() bool { return true }}, out, errOut, &downloads
}

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name, current, answer                     string
		check, yes, nonterminal, updated, wantErr bool
	}{
		{name: "confirmed", updated: true},
		{name: "check", check: true},
		{name: "check wins over yes", check: true, yes: true},
		{name: "declined", answer: "n\n"},
		{name: "default no", answer: "\n"},
		{name: "EOF", answer: "EOF"},
		{name: "nonterminal", nonterminal: true, wantErr: true},
		{name: "yes", yes: true, nonterminal: true, updated: true},
		{name: "same", current: "v0.2.0"},
		{name: "newer", current: "v0.3.0"},
		{name: "dev confirmed", current: "dev", updated: true},
		{name: "dev check", current: "v0.2.1-0.20260920031334-9d33d8ff0171", check: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, out, errOut, downloads := fixture(t, tarball(t, "fledge", "new binary", tar.TypeReg), "")
			if tc.current != "" {
				opts.Current = tc.current
			}
			if tc.answer != "" {
				opts.Stdin = strings.NewReader(strings.TrimSuffix(tc.answer, "EOF"))
			}
			opts.Check, opts.Yes = tc.check, tc.yes
			opts.IsTerminal = func() bool { return !tc.nonterminal }
			err := Run(context.Background(), opts)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Run() error = %v", err)
			}
			got, err := os.ReadFile(opts.ExecPath)
			if err != nil {
				t.Fatal(err)
			}
			want := "old binary"
			if tc.updated {
				want = "new binary"
			}
			if string(got) != want {
				t.Fatalf("binary = %q, want %q", got, want)
			}
			if !tc.updated && *downloads != 0 {
				t.Fatalf("unnecessary asset downloads: %d", *downloads)
			}
			if tc.updated && !strings.Contains(out.String(), "v0.2.0") {
				t.Fatalf("output = %q", out)
			}
			if strings.HasPrefix(tc.name, "dev") && !strings.Contains(errOut.String(), "cannot compare") {
				t.Fatalf("warning = %q", errOut)
			}
			fi, _ := os.Stat(opts.ExecPath)
			if fi.Mode().Perm() != 0751 {
				t.Fatalf("mode = %v", fi.Mode())
			}
		})
	}
}

func TestFailedDownloadPreservesExecutable(t *testing.T) {
	for _, tc := range []struct {
		name, entry, checksum string
		kind                  byte
	}{
		{"checksum mismatch", "fledge", "bad", tar.TypeReg},
		{"traversal", "../fledge", "", tar.TypeReg},
		{"absolute", "/fledge", "", tar.TypeReg},
		{"nested binary", "nested/fledge", "", tar.TypeReg},
		{"symlink", "fledge", "", tar.TypeSymlink},
		{"empty binary", "fledge", "", tar.TypeReg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := "new binary"
			if tc.name == "empty binary" {
				body = ""
			}
			opts, _, _, _ := fixture(t, tarball(t, tc.entry, body, tc.kind), tc.checksum)
			opts.Yes = true
			if err := Run(context.Background(), opts); err == nil {
				t.Fatal("expected error")
			}
			got, _ := os.ReadFile(opts.ExecPath)
			if string(got) != "old binary" {
				t.Fatalf("binary changed: %q", got)
			}
			files, _ := os.ReadDir(filepath.Dir(opts.ExecPath))
			if len(files) != 1 {
				t.Fatalf("temporary files remain: %v", files)
			}
		})
	}
}

func TestUpdateFollowsExecutableSymlink(t *testing.T) {
	opts, _, _, _ := fixture(t, tarball(t, "fledge", "new binary", tar.TypeReg), "")
	real := opts.ExecPath
	link := filepath.Join(t.TempDir(), "fledge-link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	opts.ExecPath, opts.Yes = link, true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Lstat(link)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced symlink")
	}
	got, _ := os.ReadFile(real)
	if string(got) != "new binary" {
		t.Fatalf("binary = %q", got)
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	opts, _, _, downloads := fixture(t, nil, "")
	opts.GOOS, opts.Yes = "windows", true
	if err := Run(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v", err)
	}
	if *downloads != 0 {
		t.Fatal("downloaded unsupported asset")
	}
}

func TestNullDeviceIsNotAnInteractiveTerminal(t *testing.T) {
	opts, _, _, downloads := fixture(t, nil, "")
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	opts.Stdin, opts.IsTerminal = f, nil
	if err := Run(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "not a terminal") {
		t.Fatalf("expected --yes requirement for /dev/null, got %v", err)
	}
	if *downloads != 0 {
		t.Fatal("downloaded without confirmation")
	}
}

type failingIO struct{ err error }

func (f failingIO) Read([]byte) (int, error)  { return 0, f.err }
func (f failingIO) Write([]byte) (int, error) { return 0, f.err }

func TestUpdateIOFailures(t *testing.T) {
	for _, kind := range []string{"confirmation", "warning", "prompt", "check output", "up to date output", "canceled output", "success output"} {
		t.Run(kind, func(t *testing.T) {
			opts, _, _, downloads := fixture(t, tarball(t, "fledge", "new binary", tar.TypeReg), "")
			sentinel := errors.New("I/O failed")
			switch kind {
			case "confirmation":
				opts.Stdin = failingIO{sentinel}
			case "warning":
				opts.Current, opts.Err = "dev", failingIO{sentinel}
			case "prompt":
				opts.Err = failingIO{sentinel}
			case "check output":
				opts.Check, opts.Out = true, failingIO{sentinel}
			case "up to date output":
				opts.Current, opts.Out = "v0.2.0", failingIO{sentinel}
			case "canceled output":
				opts.Stdin = strings.NewReader("n\n")
				opts.Err = &writeFailureAfter{remaining: 1, err: sentinel}
			case "success output":
				opts.Yes, opts.Out = true, failingIO{sentinel}
			}
			if err := Run(context.Background(), opts); !errors.Is(err, sentinel) {
				t.Fatalf("got %v, want %v", err, sentinel)
			}
			want := "old binary"
			if kind == "success output" {
				want = "new binary"
			} else if *downloads != 0 {
				t.Fatalf("unexpected downloads: %d", *downloads)
			}
			assertInstalledFile(t, opts.ExecPath, want)
		})
	}
}

type writeFailureAfter struct {
	remaining int
	err       error
}

func (w *writeFailureAfter) Write(p []byte) (int, error) {
	if w.remaining == 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func assertInstalledFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("installed file = %q, err = %v; want %q", got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0751 {
		t.Fatalf("permissions changed: %v", info.Mode())
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".fledge-update-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("leftover temporary files: %v, %v", files, err)
	}
}

func TestMissingExecutableTarget(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(fmt.Sprint(symlink), func(t *testing.T) {
			opts, _, _, downloads := fixture(t, nil, "")
			original := opts.ExecPath
			opts.ExecPath = filepath.Join(filepath.Dir(original), "missing")
			if symlink {
				link := filepath.Join(filepath.Dir(original), "link")
				if err := os.Symlink(opts.ExecPath, link); err != nil {
					t.Fatal(err)
				}
				opts.ExecPath = link
			}
			opts.Yes = true
			if err := Run(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "resolve executable") {
				t.Fatalf("got %v", err)
			}
			if *downloads != 0 {
				t.Fatal("downloaded assets without an executable target")
			}
			assertInstalledFile(t, original, "old binary")
		})
	}
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCancellationAfterDownloadPreservesExecutable(t *testing.T) {
	opts, _, _, downloads := fixture(t, tarball(t, "fledge", "new binary", tar.TypeReg), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := opts.Client.Transport
	opts.Client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
		resp, err := base.RoundTrip(r)
		if err == nil && strings.HasSuffix(r.URL.Path, ".tar.gz") {
			resp.Body = cancelOnClose{resp.Body, cancel}
		}
		return resp, err
	})
	opts.Yes = true
	if err := Run(ctx, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if *downloads != 2 {
		t.Fatalf("downloads = %d, want checksums and archive", *downloads)
	}
	assertInstalledFile(t, opts.ExecPath, "old binary")
}

// recordRequests logs every request path Run makes, in order.
func recordRequests(opts *Options) *[]string {
	var paths []string
	base := opts.Client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	opts.Client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, strings.TrimPrefix(r.URL.Path, "/Harrison-Blair/fledge/releases/"))
		return base.RoundTrip(r)
	})
	return &paths
}

// TestRunSequencing pins Run's order: platform validation before any request,
// the latest-tag lookup before comparison, confirmation and executable
// resolution before any download, and exact user-facing text.
func TestRunSequencing(t *testing.T) {
	const prompt = "Install fledge v0.2.0 over v0.1.0? [y/N] "
	for _, tc := range []struct {
		name          string
		setup         func(*Options)
		wantPaths     []string
		wantOut       string
		wantErrOut    string
		wantErr       string
		wantInstalled string
	}{
		{name: "confirmed", wantPaths: []string{"latest", "download/v0.2.0/checksums.txt", "download/v0.2.0/fledge_v0.2.0_linux_amd64.tar.gz"},
			wantOut: "fledge updated to v0.2.0\n", wantErrOut: prompt, wantInstalled: "new binary"},
		{name: "yes", setup: func(o *Options) { o.Yes = true }, wantPaths: []string{"latest", "download/v0.2.0/checksums.txt", "download/v0.2.0/fledge_v0.2.0_linux_amd64.tar.gz"},
			wantOut: "fledge updated to v0.2.0\n", wantInstalled: "new binary"},
		{name: "check", setup: func(o *Options) { o.Check = true }, wantPaths: []string{"latest"},
			wantOut: "fledge v0.2.0 is available (you have v0.1.0); run fledge update to install it\n"},
		{name: "up to date", setup: func(o *Options) { o.Current = "v0.2.0" }, wantPaths: []string{"latest"}, wantOut: "fledge v0.2.0 is up to date\n"},
		{name: "declined", setup: func(o *Options) { o.Stdin = strings.NewReader("n\n") }, wantPaths: []string{"latest"}, wantErrOut: prompt + "update canceled\n"},
		{name: "dev check", setup: func(o *Options) { o.Current, o.Check = "dev", true }, wantPaths: []string{"latest"},
			wantOut: "fledge v0.2.0 is available (you have dev); run fledge update to install it\n", wantErrOut: "warning: cannot compare development build \"dev\" with the latest release\n"},
		{name: "unsupported", setup: func(o *Options) { o.GOARCH = "386" }, wantErr: "update: unsupported platform linux/386; releases support Linux amd64 and arm64"},
		{name: "nonterminal", setup: func(o *Options) { o.IsTerminal = func() bool { return false } }, wantPaths: []string{"latest"},
			wantErr: "update: stdin is not a terminal; use --yes to update without a prompt"},
		{name: "missing executable", setup: func(o *Options) { o.Yes, o.ExecPath = true, o.ExecPath+"-missing" }, wantPaths: []string{"latest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, out, errOut, _ := fixture(t, tarball(t, "fledge", "new binary", tar.TypeReg), "")
			exe := opts.ExecPath
			paths := recordRequests(&opts)
			if tc.setup != nil {
				tc.setup(&opts)
			}
			err := Run(context.Background(), opts)
			if tc.name == "missing executable" {
				if err == nil || !strings.HasPrefix(err.Error(), "update: resolve executable: ") {
					t.Fatalf("error = %v", err)
				}
			} else if (err == nil && tc.wantErr != "") || (err != nil && err.Error() != tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if len(*paths) != len(tc.wantPaths) || strings.Join(*paths, ",") != strings.Join(tc.wantPaths, ",") {
				t.Fatalf("requests = %q, want %q", *paths, tc.wantPaths)
			}
			if out.String() != tc.wantOut || errOut.String() != tc.wantErrOut {
				t.Fatalf("stdout = %q, stderr = %q; want %q, %q", out, errOut, tc.wantOut, tc.wantErrOut)
			}
			want := tc.wantInstalled
			if want == "" {
				want = "old binary"
			}
			assertInstalledFile(t, exe, want)
		})
	}
}

func TestLatestTagFailureStopsBeforeMutation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer srv.Close()
	opts, out, errOut, _ := fixture(t, nil, "")
	opts.BaseURL, opts.Client, opts.Yes = srv.URL, srv.Client(), true
	if err := Run(context.Background(), opts); err == nil || err.Error() != "update: latest release returned HTTP 500" {
		t.Fatalf("error = %v", err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("unexpected output: %q %q", out, errOut)
	}
	assertInstalledFile(t, opts.ExecPath, "old binary")
}
