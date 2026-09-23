package checks_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
)

func TestConnectivityOK(t *testing.T) {
	// The stored JSON detail keeps the original wording; only the human line is
	// shortened.
	c := checks.Connectivity(nil)
	want := report.Check{Name: "herdr_connectivity", Status: report.OK, Detail: "Herdr socket responded to ping", Human: "socket responded to ping"}
	if c != want {
		t.Fatalf("connectivity = %+v, want %+v", c, want)
	}
}

func TestConnectivityFailure(t *testing.T) {
	c := checks.Connectivity(errors.New("connection refused"))
	if c.Name != "herdr_connectivity" || c.Status != report.Fail || !strings.Contains(c.Detail, "connection refused") {
		t.Fatalf("connectivity %+v", c)
	}
	if c.Detail != "ping failed: connection refused" {
		t.Fatalf("detail = %q", c.Detail)
	}
}
