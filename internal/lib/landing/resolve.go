package landing

import (
	"context"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
	"github.com/Harrison-Blair/fledge/internal/lib/gitstatus"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// The resolvers below refuse with codes named <prefix>_branch_unknown,
// <prefix>_target_missing, <prefix>_target_invalid, and <prefix>_stack_fork,
// where prefix is the calling command, such as "land" or "size".

// TaskBranch returns the stored branch of r, else the branch checked out in
// its owner's recorded checkout, else the branch its owner's spawn created.
func TaskBranch(ctx context.Context, s *state.Store, r task.Record, prefix string) (string, error) {
	if r.Branch != nil {
		return *r.Branch, nil
	}
	if r.Owner != nil {
		var owner identity.Record
		if err := s.Get(identity.Kind, *r.Owner, &owner); err == nil {
			if owner.WorktreePath != nil {
				if b := gitstatus.Branch(ctx, *owner.WorktreePath); b != nil {
					return *b, nil
				}
			}
			if owner.WorktreeBranch != nil {
				return *owner.WorktreeBranch, nil
			}
		}
	}
	return "", refuse(prefix+"_branch_unknown", "cannot tell which branch task %s is on: its owner has no recorded checkout branch", r.ID)
}

// TaskTarget returns the short name of the base branch of r, else of the
// integration branch of the repository at root, once it is a valid branch
// name.
func TaskTarget(ctx context.Context, root string, r task.Record, prefix string) (string, error) {
	ref := ""
	if r.BaseBranch != nil {
		ref = *r.BaseBranch
	} else {
		var err error
		if ref, err = gitstatus.DefaultBranch(ctx, root); err != nil {
			return "", refuse(prefix+"_target_missing", "%v", err)
		}
		if ref == "" {
			return "", refuse(prefix+"_target_missing", "no integration branch: set [branches] dev in .fledge/config.toml or create the task with --base-branch")
		}
	}
	if err := ValidTarget(ctx, "target", ref); err != nil {
		return "", refuse(prefix+"_target_invalid", "%v", err)
	}
	return ShortBranch(ref), nil
}

// BaseRef returns the ref of target that a task in mode is compared with:
// refs/remotes/origin/<target> for pr and auto, refs/heads/<target>
// otherwise. When that ref does not exist in the repository at dir, it
// returns the other one and names the missing ref in missing.
func BaseRef(ctx context.Context, dir, mode, target, prefix string) (ref, missing string, err error) {
	local, remote := "refs/heads/"+target, "refs/remotes/origin/"+target
	ref, other := local, remote
	if mode == task.ModePR || mode == task.ModeAuto {
		ref, other = remote, local
	}
	switch {
	case RefCommit(ctx, dir, ref) != "":
		return ref, "", nil
	case RefCommit(ctx, dir, other) != "":
		return other, ref, nil
	}
	return "", "", refuse(prefix+"_target_missing", "target branch %s exists as neither %s nor %s", target, ref, other)
}

// LayerBase returns the short name of the branch that task size and task land
// compare r with.
// For a layer of a stack parent, it is the stored branch of the layer below.
// For the bottom layer, and for a layer above a landed layer, it is the stack
// base: the TaskTarget of the parent. For any other task, it is the
// TaskTarget of r.
func LayerBase(ctx context.Context, s *state.Store, root string, r task.Record, prefix string) (string, error) {
	if r.Parent == nil {
		return TaskTarget(ctx, root, r, prefix)
	}
	p, err := task.Get(s, *r.Parent)
	if err != nil {
		return "", err
	}
	if p.Shape != task.ShapeStack {
		return TaskTarget(ctx, root, r, prefix)
	}
	layers, err := StackLayers(s, p, prefix)
	if err != nil {
		return "", err
	}
	for i := 1; i < len(layers); i++ {
		below := layers[i-1]
		if layers[i].ID != r.ID || below.Status == task.Landed {
			continue
		}
		if below.Branch == nil {
			return "", refuse(prefix+"_branch_unknown", "layer %s below task %s has no stored branch", below.ID, r.ID)
		}
		return *below.Branch, nil
	}
	return TaskTarget(ctx, root, p, prefix)
}

// StackLayers returns the layers of stack parent p, bottom first.
func StackLayers(s *state.Store, p task.Record, prefix string) ([]task.Record, error) {
	rs, err := task.List(s)
	if err != nil {
		return nil, err
	}
	layers, err := task.Layers(p.ID, rs)
	if err != nil {
		return nil, refuse(prefix+"_stack_fork", "stack %s: %v", p.ID, err)
	}
	return layers, nil
}

func refuse(code, format string, args ...any) error {
	return &cli.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
