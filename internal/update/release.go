package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const repoPath = "/Harrison-Blair/fledge"
const maxAssetBytes int64 = 200 << 20
const maxChecksumsBytes int64 = 1 << 20

// Source locates GitHub releases. Dependencies can be supplied for offline tests.
type Source struct {
	BaseURL string
	Client  *http.Client
}

func (s Source) baseURL() string {
	if s.BaseURL == "" {
		return "https://github.com"
	}
	return strings.TrimRight(s.BaseURL, "/")
}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// LatestTag uses GitHub's public latest-release redirect, without API credentials.
func (s Source) LatestTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL()+repoPath+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	client := *s.client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("update: find latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("update: latest release returned HTTP %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		return "", fmt.Errorf("update: invalid release redirect: %w", err)
	}
	if loc.Path == repoPath+"/releases" {
		return "", fmt.Errorf("update: no releases published yet")
	}
	tag, ok := strings.CutPrefix(loc.Path, repoPath+"/releases/tag/")
	if !ok {
		return "", fmt.Errorf("update: unexpected release redirect %q", loc.Path)
	}
	if _, err := ParseVersion(tag); err != nil {
		return "", err
	}
	return tag, nil
}

func (s Source) fetch(ctx context.Context, target string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: GET %s: HTTP %d", target, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("update: download exceeds %d bytes", limit)
	}
	return b, nil
}

func (s Source) binary(ctx context.Context, tag, arch string) ([]byte, error) {
	name := fmt.Sprintf("fledge_%s_linux_%s.tar.gz", tag, arch)
	base := s.baseURL() + repoPath + "/releases/download/" + tag + "/"
	sums, err := s.fetch(ctx, base+"checksums.txt", maxChecksumsBytes)
	if err != nil {
		return nil, err
	}
	want := ""
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if want != "" {
				return nil, fmt.Errorf("update: duplicate checksum for %s", name)
			}
			want = fields[0]
		}
	}
	if want == "" {
		return nil, fmt.Errorf("update: release %s has no checksum for %s", tag, name)
	}
	asset, err := s.fetch(ctx, base+name, maxAssetBytes)
	if err != nil {
		return nil, err
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(asset)); got != want {
		return nil, fmt.Errorf("update: checksum mismatch for %s", name)
	}
	return extractBinary(asset, maxAssetBytes)
}

func extractBinary(asset []byte, limit int64) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(asset))
	if err != nil {
		return nil, fmt.Errorf("update: read archive: %w", err)
	}
	defer gz.Close()
	uncompressed := &io.LimitedReader{R: gz, N: limit + 1}
	tr := tar.NewReader(uncompressed)
	var binary []byte
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("update: read archive: %w", err)
		}
		if h.Size < 0 || h.Size > limit {
			return nil, fmt.Errorf("update: archive exceeds %d bytes", limit)
		}
		name := path.Clean(strings.ReplaceAll(h.Name, `\`, "/"))
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("update: unsafe archive path %q", h.Name)
		}
		if name != "fledge" {
			continue
		}
		if h.Typeflag != tar.TypeReg || len(binary) != 0 || h.Size == 0 {
			return nil, fmt.Errorf("update: invalid fledge archive entry")
		}
		binary, err = io.ReadAll(io.LimitReader(tr, limit+1))
		if err != nil {
			return nil, err
		}
	}
	// Consume the gzip trailer too: Close alone does not verify it. The limit
	// includes headers and padding, including data after tar's end markers.
	if _, err := io.Copy(io.Discard, uncompressed); err != nil {
		return nil, fmt.Errorf("update: finish archive: %w", err)
	}
	if uncompressed.N == 0 {
		return nil, fmt.Errorf("update: archive exceeds %d bytes", limit)
	}
	if len(binary) == 0 {
		return nil, fmt.Errorf("update: archive has no fledge executable")
	}
	return binary, nil
}
