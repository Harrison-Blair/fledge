package worktree

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// markerFile is the marker's name in a linked checkout's Git admin directory,
// <common dir>/worktrees/<name>. Git deletes that directory when it removes or
// prunes the checkout, so a checkout recreated at the same path, even on the
// same branch, starts without a marker. A random token, unlike an inode
// number or the admin directory's name, is never reused.
const markerFile = "fledge-checkout-id"

// Mark gives the linked checkout at path a new random marker and returns it.
// It refuses the primary checkout and never replaces an existing marker.
func Mark(ctx context.Context, path string) (string, error) {
	admin, err := adminDir(ctx, path)
	if err != nil {
		return "", err
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b[:])
	f, err := os.OpenFile(filepath.Join(admin, markerFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("mark checkout %s: %w", path, err)
	}
	_, err = f.WriteString(id + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", fmt.Errorf("mark checkout %s: %w", path, err)
	}
	return id, nil
}

// Marker returns the marker of the linked checkout at path, or "" when it has
// none or git cannot say, so an unreadable marker never matches a record.
func Marker(ctx context.Context, path string) string {
	admin, err := adminDir(ctx, path)
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(admin, markerFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// adminDir returns the Git admin directory of the linked checkout whose top
// level is path, refusing the primary checkout and anything else.
func adminDir(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("read checkout %s: %w", path, err)
	}
	f := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(f) != 3 || Canonical(f[0]) != Canonical(path) {
		return "", fmt.Errorf("%s is not a checkout's top level", path)
	}
	if Canonical(f[1]) == Canonical(f[2]) {
		return "", fmt.Errorf("%s is the primary checkout", path)
	}
	return f[1], nil
}
