package landing

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/state"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
)

// PatchID returns the git patch-id --verbatim of the diff base...head in the
// repository at dir. It returns an empty string for an empty diff.
// --verbatim keeps whitespace, so a change only in whitespace gives another
// patch id.
func PatchID(ctx context.Context, dir, base, head string) (string, error) {
	diff, err := exec.CommandContext(ctx, "git", "-C", dir, "diff", "--no-color", "--no-ext-diff", base+"..."+head).Output()
	if err != nil {
		return "", fmt.Errorf("diff %s...%s: %w", base, head, err)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "patch-id", "--verbatim")
	cmd.Stdin = bytes.NewReader(diff)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("patch id of %s...%s: %w", base, head, err)
	}
	if fields := strings.Fields(string(out)); len(fields) > 0 {
		return fields[0], nil
	}
	return "", nil
}

// TaskPatchID returns the PatchID of head against the base ref of mode for
// the LayerBase of r. Task size and task land measure r against this base.
func TaskPatchID(ctx context.Context, s *state.Store, root string, r task.Record, mode, head, prefix string) (string, error) {
	target, err := LayerBase(ctx, s, root, r, prefix)
	if err != nil {
		return "", err
	}
	base, _, err := BaseRef(ctx, root, mode, target, prefix)
	if err != nil {
		return "", err
	}
	return PatchID(ctx, root, base, head)
}

// CheckCarry refuses with land_head_mismatch when head does not have the
// verified patch id of r. Then a carry-forward of the verification to head
// is not permitted.
func CheckCarry(ctx context.Context, s *state.Store, root string, r task.Record, mode, head, prefix string) error {
	id, err := TaskPatchID(ctx, s, root, r, mode, head, prefix)
	reason := "its patch id is different"
	switch {
	case err != nil:
		reason = fmt.Sprintf("its patch id is unknown (%v)", err)
	case r.VerifiedPatchID == nil:
		reason = "the verification stored no patch id"
	case id != "" && id == *r.VerifiedPatchID:
		return nil
	}
	return refuse("land_head_mismatch", "head %s of task %s is not the verified commit %s, and %s; verify the new head again", head, r.ID, *r.VerifiedCommit, reason)
}
