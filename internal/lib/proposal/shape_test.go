package proposal

import (
	"strings"
	"testing"
)

const parentTable = "[parent]\ntitle = \"Epic\"\n"

func parentWith(lines string) string {
	return header + parentTable + lines + "brief = '''\n" + valid() + "'''\n\n" + task("a", "")
}

func TestDecodeParentKeysAndTaskBranch(t *testing.T) {
	entry := strings.Replace(task("a", ""), "key = \"a\"\n", "key = \"a\"\nbranch = \"stack/01-a\"\n", 1)
	src := header + parentTable + "shape = \"stack\"\nbranch = \"stack/00\"\nbase_branch = \"dev\"\ndelivery_mode = \"pr\"\nbrief = '''\n" + valid() + "'''\n\n" + entry + task("b", `"a"`)
	p, err := Decode([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pa := p.Parent
	if *pa.Shape != "stack" || *pa.Branch != "stack/00" || *pa.BaseBranch != "dev" || *pa.DeliveryMode != "pr" {
		t.Fatalf("%+v", pa)
	}
	if *p.Tasks[0].Branch != "stack/01-a" || p.Tasks[1].Branch != nil {
		t.Fatalf("%+v", p.Tasks)
	}
	p, err = Decode([]byte(parentWith("")))
	if err != nil || p.Parent.Shape != nil || p.Parent.Branch != nil || p.Parent.BaseBranch != nil || p.Parent.DeliveryMode != nil {
		t.Fatalf("%v %+v", err, p.Parent)
	}
}

func TestDecodeRejectsBadParentKeys(t *testing.T) {
	for lines, want := range map[string]string{
		"delivery_mode = \"fast\"\n": "[parent]: delivery_mode must be one of local, pr, auto, none",
		"delivery_mode = \"\"\n":     "[parent]: delivery_mode must be one of local, pr, auto, none",
		"shape = \"tower\"\n":        "[parent]: shape must be one of stack, feature",
		"colour = \"x\"\n":           `unknown key "colour" in [parent]`,
	} {
		if _, err := Decode([]byte(parentWith(lines))); err == nil || err.Error() != want {
			t.Fatalf("%q: %v", lines, err)
		}
	}
}
