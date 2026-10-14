package land

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpNamesAutoMode(t *testing.T) {
	cmd := New()
	var b bytes.Buffer
	cmd.SetOut(&b)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"auto does the same\nsteps and merges in one command, with no review stop", "local, pr, or auto", "pr lands in two steps",
		"stops with the pull request open for the owner's\nreview", "task land\n--merge runs every guard", "--merge applies only to pr mode", "--merge ",
		"A review-stage pull\nrequest merged in GitHub is recorded by the next task land",
		"Every attempt that passes the head check prints the advisory review-size\nreport", "the size never\nrefuses a landing",
		"A stack parent lands in pr mode only", "pushes each layer branch to origin, bottom first, without force",
		"--branch and --mode are\ninvalid input for a stack", "fledge task land --id <parent>",
		"local merges the verified commit with a merge commit in the checkout that has\nthe target checked out: the primary checkout or a managed worktree"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help lacks %q: %s", want, b.String())
		}
	}
}
