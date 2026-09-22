// Package fledgedir locates and prepares the managed .fledge directory in a
// repository's primary checkout.
package fledgedir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Root resolves the primary (non-linked) checkout of the repository containing cwd.
func Root(ctx context.Context, cwd string) (string, error) {
	b, err := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--is-bare-repository", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("%s is not inside a git repository: %s", cwd, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 || lines[0] == "true" {
		return "", fmt.Errorf("%s is in a bare repository, which has no primary checkout", cwd)
	}
	if filepath.Base(lines[1]) != ".git" {
		return "", fmt.Errorf("unsupported repository layout: git directory %s is not a checkout's .git", lines[1])
	}
	return filepath.Dir(lines[1]), nil
}

// Ensure creates root/.fledge with a .gitignore whose last rule is "*", then
// verifies git ignores its contents. It validates everything before writing,
// only appends to an existing ignore file, and records each mutation on out.
func Ensure(root string, out *libagent.Outcome) (string, error) {
	dir := filepath.Join(root, ".fledge")
	if err := CheckParents(root, dir); err != nil {
		return "", err
	}
	ignore := filepath.Join(dir, ".gitignore")
	var content []byte
	info, err := os.Lstat(ignore)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", libagent.Invalid("managed ignore file must be a regular file: %s", ignore)
		}
		content, err = os.ReadFile(ignore)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = MakeParents(root, dir, out); err != nil {
		return "", err
	}
	if lastRule(content) != "*" {
		if err = appendIgnoreRule(ignore, content, info == nil, out); err != nil {
			return "", err
		}
	}
	if b, err := exec.Command("git", "-C", root, "check-ignore", "--no-index", "--quiet", "--", filepath.Join(".fledge", ".gitignore")).CombinedOutput(); err != nil {
		return "", fmt.Errorf("managed directory is not ignored: %w %s", err, b)
	}
	return dir, nil
}

// CheckParents refuses any existing component between root and parent that
// is not a real directory.
func CheckParents(root, parent string) error {
	rel, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return libagent.Invalid("managed parent must be a real directory: %s", path)
		}
	}
	return nil
}

// MakeParents creates each missing directory between root and parent,
// recording every creation on out.
func MakeParents(root, parent string, out *libagent.Outcome) error {
	rel, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	path := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err = os.Mkdir(path, 0755); err != nil {
			return err
		}
		out.Effects = append(out.Effects, libagent.Effect{Action: "created", Kind: "directory", Path: path})
	}
	return nil
}
func lastRule(content []byte) string {
	lines := strings.Split(string(content), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func appendIgnoreRule(path string, observed []byte, created bool, out *libagent.Outcome) error {
	suffix := []byte("*\n")
	if len(observed) > 0 && observed[len(observed)-1] != '\n' {
		suffix = append([]byte{'\n'}, suffix...)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	action := "updated"
	if created {
		action = "created"
	}
	return writeIgnoreRule(f, suffix, libagent.Effect{Action: action, Kind: "file", Path: path}, out)
}

func writeIgnoreRule(f io.WriteCloser, suffix []byte, effect libagent.Effect, out *libagent.Outcome) error {
	if effect.Action == "created" {
		out.Effects = append(out.Effects, effect)
	}
	n, writeErr := f.Write(suffix)
	if n > 0 && effect.Action != "created" {
		out.Effects = append(out.Effects, effect)
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
