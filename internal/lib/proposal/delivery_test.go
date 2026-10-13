package proposal

import (
	"strings"
	"testing"
)

func TestDecodeDeliveryKeys(t *testing.T) {
	entry := task("a", "")
	entry = strings.Replace(entry, "key = \"a\"\n", "key = \"a\"\ndelivery_mode = \"pr\"\nbase_branch = \"dev\"\n", 1)
	p, err := Decode([]byte(header + entry + task("b", "")))
	if err != nil {
		t.Fatal(err)
	}
	if *p.Tasks[0].DeliveryMode != "pr" || *p.Tasks[0].BaseBranch != "dev" || p.Tasks[1].DeliveryMode != nil || p.Tasks[1].BaseBranch != nil {
		t.Fatalf("%+v", p.Tasks)
	}
	for _, mode := range []string{"merge", ""} {
		bad := strings.Replace(task("a", ""), "key = \"a\"\n", "key = \"a\"\ndelivery_mode = \""+mode+"\"\n", 1)
		if _, err := Decode([]byte(header + bad)); err == nil || !strings.Contains(err.Error(), "delivery_mode") {
			t.Fatalf("%q: %v", mode, err)
		}
	}
}

func TestDecodeDeliveryModeAuto(t *testing.T) {
	entry := strings.Replace(task("a", ""), "key = \"a\"\n", "key = \"a\"\ndelivery_mode = \"auto\"\n", 1)
	p, err := Decode([]byte(header + entry))
	if err != nil {
		t.Fatal(err)
	}
	if *p.Tasks[0].DeliveryMode != "auto" {
		t.Fatalf("%+v", p.Tasks[0])
	}
	bad := strings.Replace(task("a", ""), "key = \"a\"\n", "key = \"a\"\ndelivery_mode = \"pr-human\"\n", 1)
	if _, err := Decode([]byte(header + bad)); err == nil || err.Error() != `task "a": delivery_mode must be one of local, pr, auto, none` {
		t.Fatalf("%v", err)
	}
}
