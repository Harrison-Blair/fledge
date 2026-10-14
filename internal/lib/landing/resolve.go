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
// <prefix>_target_missing, and <prefix>_target_invalid, where prefix is the
// calling command, such as "land" or "size".

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
			return "", refuse(prefix+"_target_missing", "no integration branch: set base_branch in .fledge/config.toml or create the task with --base-branch")
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

func refuse(code, format string, args ...any) error {
	return &cli.Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
