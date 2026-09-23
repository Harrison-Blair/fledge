package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLatestTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/Harrison-Blair/fledge/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Location", "https://github.com/Harrison-Blair/fledge/releases/tag/v1.2.3")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	tag, err := (Source{BaseURL: srv.URL + "/", Client: srv.Client()}).LatestTag(context.Background())
	if err != nil || tag != "v1.2.3" {
		t.Fatalf("LatestTag() = %q, %v", tag, err)
	}
}

func TestSourceFailures(t *testing.T) {
	for _, tc := range []struct {
		name, location, want string
		status               int
	}{
		{"no releases", "/Harrison-Blair/fledge/releases", "update: no releases published yet", 302},
		{"invalid tag", "/Harrison-Blair/fledge/releases/tag/nope", `update: "nope" is not a vX.Y.Z version`, 302},
		{"other repo", "/someone/else/releases/tag/v1.2.3", `update: unexpected release redirect "/someone/else/releases/tag/v1.2.3"`, 302},
		{"missing redirect", "", `update: unexpected release redirect ""`, 302},
		{"server failure", "", "update: latest release returned HTTP 500", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", tc.location)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			_, err := (Source{BaseURL: srv.URL}).LatestTag(context.Background())
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRequestCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := (Source{BaseURL: srv.URL}).LatestTag(ctx); err == nil || !strings.HasPrefix(err.Error(), "update: find latest release: ") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

// releaseServer serves checksums and one archive for v0.2.0 on amd64.
func releaseServer(t *testing.T, sums string, asset []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Harrison-Blair/fledge/releases/download/v0.2.0/checksums.txt":
			fmt.Fprint(w, sums)
		case "/Harrison-Blair/fledge/releases/download/v0.2.0/fledge_v0.2.0_linux_amd64.tar.gz":
			_, _ = w.Write(asset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestArtifactReturnsVerifiedCompressedBytes(t *testing.T) {
	asset := []byte("compressed archive bytes")
	sum := fmt.Sprintf("%x", sha256.Sum256(asset))
	for name, sums := range map[string]string{
		"text mode":   "ffff  other.tar.gz\n" + sum + "  fledge_v0.2.0_linux_amd64.tar.gz\n",
		"binary mode": sum + " *fledge_v0.2.0_linux_amd64.tar.gz\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := releaseServer(t, sums, asset)
			got, err := (Source{BaseURL: srv.URL, Client: srv.Client()}).Artifact(context.Background(), "v0.2.0", "amd64")
			if err != nil || !bytes.Equal(got, asset) {
				t.Fatalf("Artifact() = %q, %v; want the unextracted asset", got, err)
			}
		})
	}
}

func TestArtifactChecksumMismatch(t *testing.T) {
	srv := releaseServer(t, "bad  fledge_v0.2.0_linux_amd64.tar.gz\n", []byte("asset"))
	_, err := (Source{BaseURL: srv.URL}).Artifact(context.Background(), "v0.2.0", "amd64")
	if err == nil || err.Error() != "update: checksum mismatch for fledge_v0.2.0_linux_amd64.tar.gz" {
		t.Fatalf("error = %v", err)
	}
}

func TestMissingAssetsAndChecksums(t *testing.T) {
	for _, tc := range []struct {
		name, sums, want string
		status           int
	}{
		{"missing checksum asset", "", "checksums.txt: HTTP 404", 404},
		{"missing archive", "abc  fledge_v0.2.0_linux_amd64.tar.gz\n", "fledge_v0.2.0_linux_amd64.tar.gz: HTTP 404", 200},
		{"missing checksum entry", "abc  unrelated.tar.gz\n", "update: release v0.2.0 has no checksum for fledge_v0.2.0_linux_amd64.tar.gz", 200},
		{"duplicate checksums", "abc  fledge_v0.2.0_linux_amd64.tar.gz\nabc  fledge_v0.2.0_linux_amd64.tar.gz\n", "update: duplicate checksum for fledge_v0.2.0_linux_amd64.tar.gz", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "checksums.txt") {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.sums)
				} else {
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			if _, err := (Source{BaseURL: srv.URL}).Artifact(context.Background(), "v0.2.0", "amd64"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDownloadLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "oversize") }))
	defer srv.Close()
	if _, err := (Source{}).fetch(context.Background(), srv.URL, 3); err == nil || err.Error() != "update: download exceeds 3 bytes" {
		t.Fatalf("expected download limit, got %v", err)
	}
}
