package fledge

import (
	"context"
	"sort"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
)

type SessionResolution struct {
	Session     string
	Source      string
	WorkspaceID string
	Installed   herdr.BinaryInfo
}

// ResolveSession deterministically selects the single Herdr session managed
// by Fledge for root. It deliberately does not inspect other Herdr sessions or
// consult legacy project/session associations.
func ResolveSession(
	ctx context.Context,
	root string,
	binary herdr.Binary,
) (SessionResolution, error) {
	installed, err := binary.Inspect(ctx)
	if err != nil {
		return SessionResolution{}, Wrap("herdr_incompatible", err.Error(), err)
	}
	return SessionResolution{Session: project.SessionName(root), Source: "derived", Installed: installed}, nil
}

func matchingWorkspace(snapshot herdr.Snapshot, root string) (herdr.WorkspaceInfo, bool, error) {
	matches := make([]herdr.WorkspaceInfo, 0)
	for _, workspace := range snapshot.Workspaces {
		if workspace.Worktree == nil {
			continue
		}
		matched := false
		for _, candidate := range []string{workspace.Worktree.CheckoutPath, workspace.Worktree.RepoRoot} {
			if candidate == "" {
				continue
			}
			canonical, err := project.Canonical(candidate)
			if err != nil {
				return herdr.WorkspaceInfo{}, false, err
			}
			if canonical == root {
				matched = true
				break
			}
		}
		if matched {
			matches = append(matches, workspace)
		}
	}
	if len(matches) == 0 {
		return herdr.WorkspaceInfo{}, false, nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Focused != matches[j].Focused {
			return matches[i].Focused
		}
		if matches[i].Number != matches[j].Number {
			return matches[i].Number < matches[j].Number
		}
		return matches[i].WorkspaceID < matches[j].WorkspaceID
	})
	return matches[0], true, nil
}
