package worktree

import (
	"context"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Source names the repository a worktree request starts from: exactly one of
// a Herdr workspace ID or an absolute directory inside the repository.
type Source struct{ WorkspaceID, Cwd string }

func (s Source) params() map[string]any {
	if s.WorkspaceID != "" {
		return map[string]any{"workspace_id": s.WorkspaceID}
	}
	return map[string]any{"cwd": s.Cwd}
}

// changeParams addresses create/open to the listed primary checkout when the
// source is a directory, since Herdr rejects a linked checkout as their source.
func (s Source) changeParams(listing herdr.WorktreeListResult) map[string]any {
	if s.WorkspaceID != "" {
		return s.params()
	}
	return map[string]any{"cwd": listing.Source.RepoRoot}
}

// List fetches the source repository's worktrees, rejecting incomplete results.
func List(ctx context.Context, api libagent.API, src Source) (herdr.WorktreeListResult, error) {
	var listing herdr.WorktreeListResult
	err := api.Call(ctx, "worktree.list", src.params(), &listing)
	if err == nil && (listing.Type != "worktree_list" || listing.Source.RepoRoot == "" || listing.Worktrees == nil) {
		err = libagent.AtPhase("worktree.list", libagent.Protocol("incomplete worktree.list result"))
	}
	return listing, err
}

// Create asks Herdr to check out a new branch at path and open it unfocused.
// An empty base lets Herdr choose the starting ref.
func Create(ctx context.Context, api libagent.API, src Source, listing herdr.WorktreeListResult, branch, base, path string) (herdr.CreatedResult, error) {
	params := src.changeParams(listing)
	params["branch"] = branch
	if base != "" {
		params["base"] = base
	}
	return change(ctx, api, "worktree.create", params, path)
}

// Open asks Herdr to open the existing checkout at path unfocused.
func Open(ctx context.Context, api libagent.API, src Source, listing herdr.WorktreeListResult, path string) (herdr.CreatedResult, error) {
	return change(ctx, api, "worktree.open", src.changeParams(listing), path)
}

func change(ctx context.Context, api libagent.API, method string, params map[string]any, path string) (herdr.CreatedResult, error) {
	params["path"] = path
	params["focus"] = false
	var r herdr.CreatedResult
	err := api.Call(ctx, method, params, &r)
	if err == nil {
		validType := r.Type == "worktree_created"
		if method == "worktree.open" {
			validType = r.Type == "worktree_opened" && r.AlreadyOpen != nil
		}
		p := r.RootPane
		validTopology := libagent.ValidPane(p) && r.Tab.ID == p.TabID && r.Tab.WorkspaceID == p.WorkspaceID && r.Workspace.ID == p.WorkspaceID
		if !validType || !validTopology || r.Worktree.Path == "" {
			err = libagent.Protocol("incomplete " + method + " result")
		}
	}
	return r, err
}
