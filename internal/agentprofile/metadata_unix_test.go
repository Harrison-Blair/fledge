//go:build unix

package agentprofile

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
)

type fileInfoWithMetadata struct {
	fs.FileInfo
	metadata any
}

func (i fileInfoWithMetadata) Sys() any { return i.metadata }

func TestValidateProfileInodeRejectsForeignOwner(t *testing.T) {
	path := t.TempDir() + "/profile.toml"
	if err := os.WriteFile(path, []byte(validTOML(HarnessCodex)), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid = uint32(os.Geteuid() + 1)

	err = validateProfileInode(fileInfoWithMetadata{FileInfo: info, metadata: &stat})
	if err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("validateProfileInode() error = %v, want ownership rejection", err)
	}
}
