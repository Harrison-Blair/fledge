package release

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// assetName is the release archive name for one Linux architecture.
func assetName(tag, arch string) string {
	return fmt.Sprintf("fledge_%s_linux_%s.tar.gz", tag, arch)
}

// expectedChecksum finds name's single entry in a sha256sum-format listing.
func expectedChecksum(sums []byte, tag, name string) (string, error) {
	want := ""
	for line := range strings.SplitSeq(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if want != "" {
				return "", fmt.Errorf("update: duplicate checksum for %s", name)
			}
			want = fields[0]
		}
	}
	if want == "" {
		return "", fmt.Errorf("update: release %s has no checksum for %s", tag, name)
	}
	return want, nil
}

// verify compares asset's SHA-256 digest with the expected hex checksum.
func verify(asset []byte, want, name string) error {
	if got := fmt.Sprintf("%x", sha256.Sum256(asset)); got != want {
		return fmt.Errorf("update: checksum mismatch for %s", name)
	}
	return nil
}
