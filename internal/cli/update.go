package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// updateAPIBaseURL is the GitHub API base URL queried for the latest
// release. Overridable in tests.
var updateAPIBaseURL = "https://api.github.com"

// updateHTTPTimeout bounds how long fetchLatestRelease and downloadBytes
// wait for a peer to connect, complete a TLS handshake, or send response
// headers before failing. It does not bound the time spent reading a
// response body, so a large but progressing download is not truncated.
// Overridable in tests.
var updateHTTPTimeout = 10 * time.Second

// updateHTTPClient builds the client used for a fledge-update network
// request. Its Transport fails a connect-but-stalled peer within
// updateHTTPTimeout without capping the total time of a healthy,
// in-progress download (Client.Timeout is deliberately left unset).
func updateHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: updateHTTPTimeout,
			}).DialContext,
			TLSHandshakeTimeout:   updateHTTPTimeout,
			ResponseHeaderTimeout: updateHTTPTimeout,
		},
	}
}

// updateExecutablePath resolves the path of the running binary. It is the
// target-path seam: overridable in tests to point at a throwaway temp file
// instead of the real test binary.
var updateExecutablePath = os.Executable

func init() { register("update", runUpdate, "fledge update [--yes] [--json]") }

// githubRelease is the subset of the GitHub releases/latest response fledge
// needs.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// updateJSON is the --json dry-run payload.
type updateJSON struct {
	Current  string `json:"current"`
	Latest   string `json:"latest"`
	UpToDate bool   `json:"upToDate"`
	Notes    string `json:"notes"`
}

func runUpdate(args []string) int {
	return runUpdateWith(args, os.Stdin, os.Stdout)
}

func runUpdateWith(args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	jsonOut := fs.Bool("json", false, "machine-readable dry-run output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	rel, err := fetchLatestRelease(updateAPIBaseURL)
	if err != nil {
		return fail("checking for update: %v", err)
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	current := binaryVersion
	upToDate := latest == current

	if *jsonOut {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(updateJSON{
			Current:  current,
			Latest:   latest,
			UpToDate: upToDate,
			Notes:    rel.Body,
		}); err != nil {
			return fail("encoding update status: %v", err)
		}
		return ExitOK
	}

	if upToDate {
		fmt.Fprintf(out, "fledge is already up to date (v%s)\n", current)
		return ExitOK
	}

	fmt.Fprintf(out, "current version: v%s\n", current)
	fmt.Fprintf(out, "latest version:  v%s\n", latest)
	if rel.Body != "" {
		fmt.Fprintf(out, "\nrelease notes:\n%s\n", rel.Body)
	}

	confirmed := *yes
	if !confirmed {
		confirmed = promptYesNo(in, out, fmt.Sprintf("Update to v%s? [y/N]: ", latest))
	}
	if !confirmed {
		return ExitOK
	}

	if err := performUpdate(rel); err != nil {
		return fail("updating: %v", err)
	}
	fmt.Fprintf(out, "updated to v%s\n", latest)
	return ExitOK
}

// fetchLatestRelease fetches and decodes the latest GitHub release from
// {baseURL}/repos/Harrison-Blair/fledge/releases/latest.
func fetchLatestRelease(baseURL string) (*githubRelease, error) {
	url := fmt.Sprintf("%s/repos/Harrison-Blair/fledge/releases/latest", baseURL)
	resp, err := updateHTTPClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s fetching latest release", resp.Status)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &rel, nil
}

// updateAssetName returns the release asset name expected for the running
// platform, per PLM-012's convention: fledge_<GOOS>_<GOARCH>[.tar.gz|.zip].
func updateAssetName() string {
	return fmt.Sprintf("fledge_%s_%s%s", runtime.GOOS, runtime.GOARCH, updateArchiveExt())
}

// updateArchiveExt returns the archive extension used for the running
// platform's release asset: .zip on Windows, .tar.gz everywhere else.
func updateArchiveExt() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

// findReleaseAsset returns the browser_download_url of the release asset
// with the given name, or "" if none matches.
func findReleaseAsset(rel *githubRelease, name string) string {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// downloadBytes GETs url and returns the full response body.
func downloadBytes(url string) ([]byte, error) {
	resp, err := updateHTTPClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s fetching %s", resp.Status, url)
	}
	return io.ReadAll(resp.Body)
}

// checksumFor looks up the SHA-256 hex digest for name in the sha256sum-style
// contents of checksums.txt (lines of "<hex>  <name>" or "<hex> *<name>").
func checksumFor(checksums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksums.txt entry for %s", name)
}

// extractBinary extracts the single binary entry from a downloaded release
// archive (.tar.gz or .zip, chosen by assetName's extension) and returns its
// contents.
func extractBinary(archive []byte, assetName string) ([]byte, error) {
	if strings.HasSuffix(assetName, ".zip") {
		return extractFromZip(archive)
	}
	return extractFromTarGz(archive)
}

func extractFromTarGz(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("archive contains no regular file")
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		return io.ReadAll(tr)
	}
}

func extractFromZip(archive []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry: %w", err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("archive contains no regular file")
}

// swapBinary atomically replaces the file at targetPath with data: a temp
// file is written in targetPath's directory (so os.Rename is atomic on the
// same filesystem), chmod'd executable, then renamed over targetPath. Any
// failure before the rename leaves targetPath untouched.
func swapBinary(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(targetPath); err == nil {
		mode = fi.Mode()
	}
	tmp, err := os.CreateTemp(dir, ".fledge-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		cleanup()
		return err
	}
	return nil
}

// performUpdate downloads the release asset matching the running platform,
// verifies its checksum against the release's checksums.txt, and atomically
// swaps it in over the running binary. No changes are made to the target
// binary unless every step succeeds.
func performUpdate(rel *githubRelease) error {
	assetName := updateAssetName()
	assetURL := findReleaseAsset(rel, assetName)
	if assetURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}
	checksumsURL := findReleaseAsset(rel, "checksums.txt")
	if checksumsURL == "" {
		return fmt.Errorf("release is missing checksums.txt")
	}

	archiveData, err := downloadBytes(assetURL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", assetName, err)
	}
	checksumsData, err := downloadBytes(checksumsURL)
	if err != nil {
		return fmt.Errorf("downloading checksums.txt: %w", err)
	}

	wantSum, err := checksumFor(checksumsData, assetName)
	if err != nil {
		return err
	}
	gotSum := sha256.Sum256(archiveData)
	gotHex := hex.EncodeToString(gotSum[:])
	if gotHex != wantSum {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, gotHex, wantSum)
	}

	binData, err := extractBinary(archiveData, assetName)
	if err != nil {
		return fmt.Errorf("extracting %s: %w", assetName, err)
	}

	targetPath, err := updateExecutablePath()
	if err != nil {
		return fmt.Errorf("resolving current binary path: %w", err)
	}
	return swapBinary(targetPath, binData)
}
