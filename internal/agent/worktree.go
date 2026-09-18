package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

func (s *Service) worktreePlacement(ctx context.Context, o SpawnOptions, snap *herdr.Snapshot, out *Outcome) (herdr.Pane, error) {
	source, err := workspace(snap, o.Workspace, o.WorkspaceID)
	if err != nil {
		return herdr.Pane{}, err
	}
	if o.Workspace != "" && source == "" {
		return herdr.Pane{}, invalid("source workspace %q does not exist", o.Workspace)
	}
	params := map[string]any{}
	if source != "" {
		params["workspace_id"] = source
	} else {
		cwd := o.Cwd
		if cwd == "" {
			if o.Worktree == "new" {
				cwd = s.Cwd
			} else {
				cwd = o.Worktree
			}
		}
		if cwd == "" {
			return herdr.Pane{}, fmt.Errorf("cannot resolve source working directory")
		}
		cwd, err = filepath.Abs(cwd)
		if err != nil {
			return herdr.Pane{}, err
		}
		params["cwd"] = cwd
	}
	var listing herdr.WorktreeListResult
	err = s.call(ctx, "worktree.list", params, &listing)
	if err == nil && (listing.Type != "worktree_list" || listing.Source.RepoRoot == "" || listing.Worktrees == nil) {
		err = &phaseError{phase: "worktree.list", cause: protocol("incomplete worktree.list result")}
	}
	if err != nil {
		return herdr.Pane{}, err
	}
	method := "worktree.open"
	path := o.Worktree
	if o.Worktree == "new" {
		branch := o.Branch
		if branch == "" {
			branch = o.Name
		}
		path, err = prepareWorktree(ctx, listing.Source.RepoRoot, branch, out)
		if err != nil {
			return herdr.Pane{}, err
		}
		method = "worktree.create"
		params["branch"] = branch
		if o.Base != "" {
			params["base"] = o.Base
		}
	} else {
		path, err = filepath.Abs(path)
		if err != nil {
			return herdr.Pane{}, err
		}
		// Resolve known named-tab conflicts before opening a workspace.
		for _, w := range listing.Worktrees {
			if filepath.Clean(w.Path) == path && w.OpenWorkspaceID != nil && o.Tab != "" {
				t, err := tab(snap, *w.OpenWorkspaceID, o.Tab, "")
				if err != nil {
					return herdr.Pane{}, err
				}
				if t != nil {
					if _, err = anchor(snap, *t); err != nil {
						return herdr.Pane{}, err
					}
				}
			}
		}
	}
	out.Result.(*SpawnResult).WorktreePath = &path
	params["path"] = path
	params["focus"] = false
	var r herdr.CreatedResult
	err = s.call(ctx, method, params, &r)
	if err == nil {
		validType := r.Type == "worktree_created"
		if method == "worktree.open" {
			validType = r.Type == "worktree_opened" && r.AlreadyOpen != nil
		}
		if !validType || !validCreated(r, true) || r.Worktree.Path == "" {
			err = protocol("incomplete " + method + " result")
		}
	}
	if err != nil {
		out.fail(err, method, true)
		return herdr.Pane{}, err
	}
	path = r.Worktree.Path
	out.Result.(*SpawnResult).WorktreePath = &path
	o.Cwd = path
	alreadyOpen := r.AlreadyOpen != nil && *r.AlreadyOpen
	if !alreadyOpen {
		if method == "worktree.create" {
			out.Effects = append(out.Effects, Effect{Action: "created", Kind: "worktree", Path: path})
		}
		recordCreated(out, r, true)
		return s.initialTab(ctx, o, r, out)
	}
	out.Effects = append(out.Effects, Effect{Action: "reused", Kind: "workspace", ID: r.Workspace.ID})
	return s.placeInWorkspace(ctx, o, r.Workspace.ID, snap, out)
}

// prepareWorktree validates all known collisions before touching managed paths.
func prepareWorktree(ctx context.Context, root, branch string, out *Outcome) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	checked, err := exec.CommandContext(ctx, "git", "check-ref-format", "--branch", branch).Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("check branch: %w", ctx.Err())
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() < 0 {
			return "", fmt.Errorf("check branch: %w", err)
		}
	}
	if err != nil || strings.TrimSpace(string(checked)) != branch || strings.Contains(branch, "@{") {
		return "", invalid("invalid exact branch name %q", branch)
	}
	refs, err := exec.CommandContext(ctx, "git", "-C", root, "for-each-ref", "--format=%(refname)", "refs/heads/").Output()
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}
	requested := "refs/heads/" + branch
	for _, ref := range strings.Fields(string(refs)) {
		if ref == requested {
			return "", invalid("branch %q already exists", branch)
		}
		if strings.HasPrefix(requested, ref+"/") || strings.HasPrefix(ref, requested+"/") {
			return "", invalid("branch %q conflicts with existing branch %q", branch, strings.TrimPrefix(ref, "refs/heads/"))
		}
	}

	managed := filepath.Join(root, ".fledge", "worktrees")
	path := filepath.Join(managed, filepath.FromSlash(branch))
	relative, err := filepath.Rel(managed, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", invalid("branch path escapes managed worktrees")
	}
	if _, err = os.Lstat(path); err == nil {
		return "", invalid("worktree destination %s already exists", path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err = checkParents(root, filepath.Dir(path)); err != nil {
		return "", err
	}
	ignore := filepath.Join(managed, ".gitignore")
	var content []byte
	info, err := os.Lstat(ignore)
	if err == nil {
		if !info.Mode().IsRegular() {
			return "", invalid("managed ignore file must be a regular file: %s", ignore)
		}
		content, err = os.ReadFile(ignore)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err = makeParents(root, filepath.Dir(path), out); err != nil {
		return "", err
	}
	if lastRule(content) != "*" {
		if err = appendIgnoreRule(ignore, content, info == nil, out); err != nil {
			return "", err
		}
	}

	rel, _ := filepath.Rel(root, path)
	if b, err := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--no-index", "--quiet", "--", rel).CombinedOutput(); err != nil {
		return "", fmt.Errorf("managed checkout is not ignored: %w %s", err, b)
	}
	return path, nil
}
func checkParents(root, parent string) error {
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
			return invalid("managed parent must be a real directory: %s", path)
		}
	}
	return nil
}
func makeParents(root, parent string, out *Outcome) error {
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
		out.Effects = append(out.Effects, Effect{Action: "created", Kind: "directory", Path: path})
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

func appendIgnoreRule(path string, observed []byte, created bool, out *Outcome) error {
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
	return writeIgnoreRule(f, suffix, Effect{Action: action, Kind: "file", Path: path}, out)
}

func writeIgnoreRule(f io.WriteCloser, suffix []byte, effect Effect, out *Outcome) error {
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
