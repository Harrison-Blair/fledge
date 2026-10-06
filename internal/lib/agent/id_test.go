package agent

import (
	"errors"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

func TestValidateID(t *testing.T) {
	if err := ValidateID("parent", "task", "0123abcd"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "0123ABCD", "0123abc", "0123abcde", "0123abcg"} {
		err := ValidateID("parent", "task", id)
		var input *cli.InputError
		if !errors.As(err, &input) || err.Error() != "--parent must be an 8 lowercase hexadecimal task id" {
			t.Fatalf("%q: %v", id, err)
		}
	}
	if err := ValidateID("owner", "agent", "nope"); err == nil || err.Error() != "--owner must be an 8 lowercase hexadecimal agent id" {
		t.Fatal(err)
	}
}
