// Package workspace locates and identifies the workspace a fledge command
// runs in. A workspace is a directory tree rooted at a .fledge directory;
// commands may run anywhere inside it, git-style, and resolve the same root.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// ErrNotFound is the walk-up miss. Its text matches the guidance the daemon
// gives for a bare directory.
var ErrNotFound = errors.New("no " + scaffold.DirName + " directory here or in any parent; run fledge init")

// FindRoot walks up from dir to the nearest directory containing .fledge/ and
// returns it canonical-absolute, symlinks resolved. Canonicalizing matters
// because Hash keys the socket namespace and the session name: a daemon and a
// client reaching one workspace through different spellings must agree on its
// identity. A stray .fledge regular file does not mark a workspace.
func FindRoot(dir string) (string, error) {
	p, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(p, scaffold.DirName)); err == nil && fi.IsDir() {
			if resolved, err := filepath.EvalSymlinks(p); err == nil {
				return resolved, nil
			}
			return p, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", ErrNotFound
		}
		p = parent
	}
}

// Hash identifies a workspace by its absolute path. filepath.Abs only fails
// when the process has no working directory, in which case root is already
// meaningless; hashing it as given keeps every caller total.
func Hash(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}

// Slug is a workspace's human-readable identity: its directory basename
// sanitized to what herdr session names carry (lowercase alphanumerics and
// dashes), plus enough of Hash to tell two same-named directories apart.
func Slug(root string) string {
	return slugBase(filepath.Base(root)) + "-" + Hash(root)[:6]
}

// slugBase sanitizes a directory basename: ASCII letters lowercased, digits
// kept, every other rune folded to a dash, runs collapsed, trimmed, and
// bounded to 16 bytes. A basename with nothing to keep becomes "ws".
func slugBase(base string) string {
	out := make([]byte, 0, len(base))
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out = append(out, byte(r))
		case r >= 'A' && r <= 'Z':
			out = append(out, byte(r)+'a'-'A')
		default:
			if len(out) > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
		}
	}
	if len(out) > 16 {
		out = out[:16]
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if len(out) == 0 {
		return "ws"
	}
	return string(out)
}
