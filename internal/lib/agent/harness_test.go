package agent

import (
	"errors"
	"testing"
)

func TestHarnessMembership(t *testing.T) {
	all := Harnesses()
	if len(all) != 24 || all[0] != "pi" || all[len(all)-1] != "muse" {
		t.Fatalf("%q", all)
	}
	all[0] = "mutated"
	if Harnesses()[0] != "pi" {
		t.Fatal("Harnesses leaked its backing array")
	}
	for _, kind := range Harnesses() {
		if !IsHarness(kind) || ValidateHarness(kind) != nil {
			t.Fatal(kind)
		}
	}
	err := ValidateHarness("nope")
	var input *InputError
	if IsHarness("nope") || IsHarness("") || !errors.As(err, &input) || err.Error() != "--harness must be a documented Herdr harness kind" {
		t.Fatalf("%v", err)
	}
}
