// Package repoconfig reads the committed repository configuration file,
// .fledge/config.toml in the primary checkout's working tree. Read is the
// only code that opens it.
package repoconfig

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Path is the configuration file relative to the primary checkout.
const Path = ".fledge/config.toml"

// MaxSize is the largest configuration file Read accepts, in bytes.
const MaxSize = 64 << 10

// schemaVersion is the only configuration file version this binary reads.
const schemaVersion = 1

var (
	// ErrNotRegular is returned for a symlink, directory, FIFO, or other
	// non-regular file at Path; such a file is never opened.
	ErrNotRegular = errors.New("not a regular file")
	// ErrTooLarge is returned for a file larger than MaxSize.
	ErrTooLarge = fmt.Errorf("larger than %d bytes", MaxSize)
)

// Config is a decoded, validated configuration file. A zero field is not
// configured.
type Config struct {
	BaseBranch string
}

// file is the TOML shape; a pointer distinguishes an omitted version.
type file struct {
	SchemaVersion *int64 `toml:"schema_version"`
	BaseBranch    string `toml:"base_branch"`
}

// Read returns the configuration of the repository whose primary checkout is
// root. An absent file is the zero Config. Any other problem is an error that
// names Path.
func Read(ctx context.Context, root string) (Config, error) {
	c, err := read(ctx, root)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", Path, err)
	}
	return c, nil
}

func read(ctx context.Context, root string) (Config, error) {
	path := filepath.Join(root, Path)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Config{}, nil
	case err != nil:
		return Config{}, err
	case !info.Mode().IsRegular():
		return Config{}, ErrNotRegular
	}
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return Config{}, err
	}
	if len(data) > MaxSize {
		return Config{}, ErrTooLarge
	}
	// The typed decoder matches keys case-insensitively, so check exact
	// names on a raw decode first.
	var raw map[string]any
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return Config{}, err
	}
	for k := range raw {
		if k != "schema_version" && k != "base_branch" {
			return Config{}, fmt.Errorf("unknown key %q", k)
		}
	}
	var fl file
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&fl); err != nil {
		return Config{}, err
	}
	if fl.SchemaVersion == nil {
		return Config{}, errors.New("schema_version is required")
	}
	if *fl.SchemaVersion != schemaVersion {
		return Config{}, fmt.Errorf("unsupported schema_version %d; this Fledge reads %d", *fl.SchemaVersion, schemaVersion)
	}
	if _, ok := raw["base_branch"]; ok {
		if err := checkBranch(ctx, root, fl.BaseBranch); err != nil {
			return Config{}, err
		}
	}
	return Config{BaseBranch: fl.BaseBranch}, nil
}

// checkBranch accepts a branch name git would accept as refs/heads/<name>.
// The full-ref form, unlike --branch, does not expand @{-N} from the reflog.
func checkBranch(ctx context.Context, root, name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("base_branch is blank")
	case strings.ContainsAny(name, "\r\n"):
		return fmt.Errorf("base_branch %q spans lines", name)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("base_branch %q starts with -", name)
	}
	if exec.CommandContext(ctx, "git", "-C", root, "check-ref-format", "refs/heads/"+name).Run() != nil {
		return fmt.Errorf("base_branch %q is not a valid branch name", name)
	}
	return nil
}
