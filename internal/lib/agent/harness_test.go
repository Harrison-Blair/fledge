package agent

import (
	"errors"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/harness"
)

func TestValidateHarness(t *testing.T) {
	for _, kind := range harness.Kinds() {
		if err := ValidateHarness(kind); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	for _, kind := range []string{"nope", ""} {
		err := ValidateHarness(kind)
		var input *InputError
		if !errors.As(err, &input) || err.Error() != "--harness must be a documented Herdr harness kind" {
			t.Fatalf("%q: %v", kind, err)
		}
	}
}
