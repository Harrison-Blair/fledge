package proposal

import (
	"strings"
	"testing"
)

// stack is a proposal whose [parent] has shape and whose tasks are tasks.
func stack(shape string, tasks ...string) []byte {
	return []byte(header + parentTable + "shape = \"" + shape + "\"\nbrief = '''\n" + valid() + "'''\n\n" + strings.Join(tasks, ""))
}

func TestDecodeAcceptsAStackChain(t *testing.T) {
	src := stack("stack", task("c", `"b"`), task("a", `"0123abcd"`), task("b", `"a"`))
	if _, err := Decode(src); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRefusesAStackFork(t *testing.T) {
	for src, want := range map[string]string{
		string(stack("stack", task("a", ""), task("b", `"a"`), task("c", `"a"`))): "stack layers must form one after chain: layer a has more than one dependent layer: b, c",
		string(stack("stack", task("a", ""), task("b", ""))):                      "stack layers must form one after chain: more than one layer has no prerequisite layer: a, b",
	} {
		if _, err := Decode([]byte(src)); err == nil || err.Error() != want {
			t.Fatalf("want %q, got %v", want, err)
		}
	}
	if _, err := Decode(feature(task("a", ""), task("b", `"a"`), task("c", `"a"`))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode([]byte(header + task("a", "") + task("b", `"a"`) + task("c", `"a"`))); err != nil {
		t.Fatal(err)
	}
}
