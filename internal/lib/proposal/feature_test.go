package proposal

import (
	"strings"
	"testing"
)

// feature is a proposal whose [parent] has shape feature, branch feat/x, and
// delivery_mode pr, and whose tasks are tasks.
func feature(tasks ...string) []byte {
	return []byte(header + parentTable + "shape = \"feature\"\nbranch = \"feat/x\"\ndelivery_mode = \"pr\"\nbrief = '''\n" + valid() + "'''\n\n" + strings.Join(tasks, ""))
}

// member is task a with lines added after its key.
func member(lines string) string {
	return strings.Replace(task("a", ""), "key = \"a\"\n", "key = \"a\"\n"+lines, 1)
}

func TestDecodeRefusesAFeatureParentWithoutBranch(t *testing.T) {
	want := "[parent]: shape feature needs a branch"
	if _, err := Decode(stack("feature", task("a", ""))); err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestDecodeDefaultsFeatureMembers(t *testing.T) {
	p, order, err := DecodeOrdered(feature(task("a", ""), task("b", `"a"`)))
	if err != nil {
		t.Fatal(err)
	}
	for _, ts := range [][]Task{p.Tasks, order} {
		for _, m := range ts {
			if m.DeliveryMode == nil || *m.DeliveryMode != "local" || m.BaseBranch == nil || *m.BaseBranch != "feat/x" {
				t.Fatalf("%s: %+v", m.Key, m)
			}
		}
	}
	if *p.Parent.DeliveryMode != "pr" {
		t.Fatalf("parent: %+v", p.Parent)
	}
	if _, err := Decode(feature(member("delivery_mode = \"local\"\nbase_branch = \"feat/x\"\n"))); err != nil {
		t.Fatal(err)
	}
	p, err = Decode(stack("stack", task("a", "")))
	if err != nil || p.Tasks[0].DeliveryMode != nil || p.Tasks[0].BaseBranch != nil {
		t.Fatalf("%v %+v", err, p.Tasks)
	}
}

func TestDecodeRefusesAnotherModeOrBaseOnAFeatureMember(t *testing.T) {
	for lines, want := range map[string]string{
		"delivery_mode = \"pr\"\n":   `task "a": a member of feature branch "feat/x" needs delivery_mode local, not "pr"`,
		"base_branch = \"dev\"\n":    `task "a": a member of feature branch "feat/x" needs base_branch "feat/x", not "dev"`,
		"delivery_mode = \"none\"\n": `task "a": a member of feature branch "feat/x" needs delivery_mode local, not "none"`,
	} {
		if _, err := Decode(feature(member(lines))); err == nil || err.Error() != want {
			t.Fatalf("%q: want %q, got %v", lines, want, err)
		}
	}
}
