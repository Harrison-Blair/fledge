package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestErrorKeepsCodeInItsText(t *testing.T) {
	if got := (&Error{Code: "task_not_found", Message: "no task with id deadbeef"}).Error(); got != "task_not_found: no task with id deadbeef" {
		t.Fatal(got)
	}
}

func TestCodedFindsFledgeAndHerdrCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
		ok   bool
	}{
		{"fledge", &Error{Code: "task_not_found", Message: "x"}, "task_not_found", true},
		{"herdr", &herdr.Error{Code: "agent_not_found", Message: "x"}, "agent_not_found", true},
		{"located fledge", AtPhase("identity", &Error{Code: "agent_record_not_found", Message: "x"}), "agent_record_not_found", true},
		{"wrapped herdr", fmt.Errorf("read: %w", &herdr.Error{Code: "timeout", Message: "x"}), "timeout", true},
		{"input", Invalid("bad"), "", false},
		{"plain", errors.New("boom"), "", false},
		{"nil", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code, ok := Coded(tc.err); code != tc.code || ok != tc.ok {
				t.Fatalf("got %q %v", code, ok)
			}
		})
	}
}
