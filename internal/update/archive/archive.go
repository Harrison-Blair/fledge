// Package archive extracts the fledge executable from a release archive.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
)

// maxUncompressedBytes bounds the whole decompressed tar stream, including
// headers and padding, so a small archive cannot expand without limit.
const maxUncompressedBytes int64 = 200 << 20

// ExtractFledge validates a gzip-compressed tar archive and returns the bytes of
// its single top-level regular "fledge" entry.
func ExtractFledge(asset []byte) ([]byte, error) {
	return extract(asset, maxUncompressedBytes)
}

func extract(asset []byte, limit int64) ([]byte, error) {
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
