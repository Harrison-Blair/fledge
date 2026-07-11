package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withUpdateTargetPath points the update command's target-binary-path seam
// at the given path for the duration of the test, restoring it afterward.
func withUpdateTargetPath(t *testing.T, path string) {
	t.Helper()
	prev := updateExecutablePath
	updateExecutablePath = func() (string, error) { return path, nil }
	t.Cleanup(func() { updateExecutablePath = prev })
}

// buildFakeArchive packages content as a single-entry .tar.gz (or .zip, on
// Windows) archive matching the running platform's expected asset format.
func buildFakeArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "fledge", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newUpdateSwapServer serves a canned releases/latest response plus, at
// /assets/archive and /assets/checksums, the given archive and
// checksums.txt bytes. assetName is the name reported for the archive
// asset (and matched against by the update command). When includeAsset is
// false, no archive asset is listed (simulating a missing-platform-asset
// release).
func newUpdateSwapServer(t *testing.T, tag, assetName string, archive, checksums []byte, includeAsset bool) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Harrison-Blair/fledge/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		archiveAsset := ""
		if includeAsset {
			archiveAsset = fmt.Sprintf(`{"name": %q, "browser_download_url": %q},`, assetName, srv.URL+"/assets/archive")
		}
		fmt.Fprintf(w, `{"tag_name": %q, "body": "notes", "assets": [%s{"name": "checksums.txt", "browser_download_url": %q}]}`,
			tag, archiveAsset, srv.URL+"/assets/checksums")
	})
	mux.HandleFunc("/assets/archive", func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/assets/checksums", func(w http.ResponseWriter, r *http.Request) {
		w.Write(checksums)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestUpdate_DownloadVerifyAndSwap_Success(t *testing.T) {
	assetName := updateAssetName()
	payload := []byte("fake fledge binary contents v99.99.99")
	archive := buildFakeArchive(t, payload)
	checksums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex(archive), assetName))

	srv := newUpdateSwapServer(t, "v99.99.99", assetName, archive, checksums, true)
	withUpdateBaseURL(t, srv.URL)

	targetPath := filepath.Join(t.TempDir(), "fledge")
	if err := os.WriteFile(targetPath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	withUpdateTargetPath(t, targetPath)

	var out strings.Builder
	code := runUpdateWith([]string{"--yes"}, strings.NewReader(""), &out)

	if code != ExitOK {
		t.Fatalf("exit code = %d, want ExitOK; output: %s", code, out.String())
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("target binary content = %q, want %q", got, payload)
	}
	if !strings.Contains(out.String(), "99.99.99") {
		t.Errorf("output = %q, want success message naming the new version", out.String())
	}
}

func TestUpdate_ChecksumMismatch_AbortsWithoutSwap(t *testing.T) {
	assetName := updateAssetName()
	payload := []byte("fake fledge binary contents v99.99.99")
	archive := buildFakeArchive(t, payload)
	// Deliberately wrong checksum.
	checksums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex([]byte("not the archive")), assetName))

	srv := newUpdateSwapServer(t, "v99.99.99", assetName, archive, checksums, true)
	withUpdateBaseURL(t, srv.URL)

	targetPath := filepath.Join(t.TempDir(), "fledge")
	original := []byte("old binary")
	if err := os.WriteFile(targetPath, original, 0o755); err != nil {
		t.Fatal(err)
	}
	withUpdateTargetPath(t, targetPath)

	stderr := captureStderr(t, func() {
		var out strings.Builder
		code := runUpdateWith([]string{"--yes"}, strings.NewReader(""), &out)
		if code != ExitFail {
			t.Fatalf("exit code = %d, want ExitFail; output: %s", code, out.String())
		}
	})
	if !strings.Contains(stderr, "checksum mismatch") {
		t.Errorf("stderr = %q, want an error naming the checksum mismatch", stderr)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("target binary content = %q, want unchanged %q", got, original)
	}
}

func TestUpdate_MissingPlatformAsset_AbortsWithoutSwap(t *testing.T) {
	assetName := updateAssetName()
	checksums := []byte(fmt.Sprintf("%s  %s\n", sha256Hex([]byte("irrelevant")), assetName))

	// includeAsset=false: the release has no asset matching this platform.
	srv := newUpdateSwapServer(t, "v99.99.99", assetName, nil, checksums, false)
	withUpdateBaseURL(t, srv.URL)

	targetPath := filepath.Join(t.TempDir(), "fledge")
	original := []byte("old binary")
	if err := os.WriteFile(targetPath, original, 0o755); err != nil {
		t.Fatal(err)
	}
	withUpdateTargetPath(t, targetPath)

	stderr := captureStderr(t, func() {
		var out strings.Builder
		code := runUpdateWith([]string{"--yes"}, strings.NewReader(""), &out)
		if code != ExitFail {
			t.Fatalf("exit code = %d, want ExitFail; output: %s", code, out.String())
		}
	})
	if !strings.Contains(stderr, "no release asset found") {
		t.Errorf("stderr = %q, want an error naming the missing asset", stderr)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("target binary content = %q, want unchanged %q", got, original)
	}
}

func TestUpdate_NetworkFailureDuringDownload_AbortsWithoutSwap(t *testing.T) {
	assetName := updateAssetName()

	// assetSrv hosts the archive/checksums assets but is closed before the
	// update runs, so the release JSON (served separately) points at a dead
	// address: the archive download fails mid-flow, after the release was
	// already fetched successfully.
	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := assetSrv.URL
	assetSrv.Close()

	relSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name": %q, "body": "notes", "assets": [{"name": %q, "browser_download_url": %q}, {"name": "checksums.txt", "browser_download_url": %q}]}`,
			"v99.99.99", assetName, deadURL+"/assets/archive", deadURL+"/assets/checksums")
	}))
	t.Cleanup(relSrv.Close)
	withUpdateBaseURL(t, relSrv.URL)

	targetPath := filepath.Join(t.TempDir(), "fledge")
	original := []byte("old binary")
	if err := os.WriteFile(targetPath, original, 0o755); err != nil {
		t.Fatal(err)
	}
	withUpdateTargetPath(t, targetPath)

	var out strings.Builder
	code := runUpdateWith([]string{"--yes"}, strings.NewReader(""), &out)
	if code != ExitFail {
		t.Fatalf("exit code = %d, want ExitFail; output: %s", code, out.String())
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("target binary content = %q, want unchanged %q", got, original)
	}
}

// captureStderr redirects os.Stderr for the duration of fn and returns what
// was written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = orig
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}
