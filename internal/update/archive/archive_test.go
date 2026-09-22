package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

type entry struct {
	name, body string
	kind       byte
}

func archive(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0755, Typeflag: e.kind, Size: int64(len(e.body))}
		body := e.body
		if e.kind == tar.TypeSymlink {
			h.Linkname, h.Size = "/tmp/elsewhere", 0
			body = ""
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractFledge(t *testing.T) {
	asset := archive(t, entry{"README.md", "docs", tar.TypeReg}, entry{"./fledge", "new binary", tar.TypeReg})
	got, err := ExtractFledge(asset)
	if err != nil || string(got) != "new binary" {
		t.Fatalf("ExtractFledge() = %q, %v", got, err)
	}
}

func TestExtractFledgeRejects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		asset []byte
		want  string
	}{
		{"not gzip", []byte("this is not a gzip stream"), "update: read archive: gzip: invalid header"},
		{"traversal", archive(t, entry{"../fledge", "x", tar.TypeReg}), `update: unsafe archive path "../fledge"`},
		{"backslash traversal", archive(t, entry{`..\fledge`, "x", tar.TypeReg}), `update: unsafe archive path "..\\fledge"`},
		{"absolute", archive(t, entry{"/fledge", "x", tar.TypeReg}), `update: unsafe archive path "/fledge"`},
		{"nested binary", archive(t, entry{"nested/fledge", "x", tar.TypeReg}), "update: archive has no fledge executable"},
		{"symlink", archive(t, entry{"fledge", "", tar.TypeSymlink}), "update: invalid fledge archive entry"},
		{"empty binary", archive(t, entry{"fledge", "", tar.TypeReg}), "update: invalid fledge archive entry"},
		{"duplicate binary", archive(t, entry{"fledge", "a", tar.TypeReg}, entry{"fledge", "b", tar.TypeReg}), "update: invalid fledge archive entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ExtractFledge(tc.asset); err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestArchiveLimit(t *testing.T) {
	if _, err := extract(archive(t, entry{"fledge", "too large", tar.TypeReg}), 3); err == nil {
		t.Fatal("expected size limit")
	}
}

func TestArchiveLimitIncludesHeadersAndPadding(t *testing.T) {
	if _, err := extract(archive(t, entry{"fledge", "small", tar.TypeReg}), 512); err == nil {
		t.Fatal("uncompressed archive headers must count toward the size limit")
	}
}
