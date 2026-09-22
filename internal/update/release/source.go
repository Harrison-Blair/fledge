package release

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// Artifact downloads the release archive for arch and returns its compressed
// bytes only after they match the release's published checksum.
func (s Source) Artifact(ctx context.Context, tag, arch string) ([]byte, error) {
	name := assetName(tag, arch)
	base := s.baseURL() + repoPath + "/releases/download/" + tag + "/"
	sums, err := s.fetch(ctx, base+"checksums.txt", maxChecksumsBytes)
	if err != nil {
		return nil, err
	}
	want, err := expectedChecksum(sums, tag, name)
	if err != nil {
		return nil, err
	}
	asset, err := s.fetch(ctx, base+name, maxAssetBytes)
	if err != nil {
		return nil, err
	}
	if err := verify(asset, want, name); err != nil {
		return nil, err
	}
	return asset, nil
}
