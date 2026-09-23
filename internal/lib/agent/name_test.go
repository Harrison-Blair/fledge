package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	for _, name := range []string{"a", "reviewer", "r2_d-2", "a" + strings.Repeat("b", 31)} {
		if err := ValidateName(name); err != nil {
			t.Fatalf("ValidateName(%q) = %v", name, err)
		}
	}
	for _, name := range []string{"", "2a", "_a", "Reviewer", "a.b", "a" + strings.Repeat("b", 32), "ab\n"} {
		err := ValidateName(name)
		var input *InputError
		if !errors.As(err, &input) || err.Error() != "--name must match [a-z][a-z0-9_-]{0,31}" {
			t.Fatalf("ValidateName(%q) = %v", name, err)
		}
	}
}
