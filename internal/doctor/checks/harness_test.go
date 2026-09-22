package checks_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestHarnessAvailable(t *testing.T) {
	c := checks.Harness(integrationList("opencode"), nil)
	if c.Name != "harness_installations" || c.Status != report.OK || c.Detail != "6 targets, 1 available: opencode" {
		t.Fatalf("harness %+v", c)
	}
	if data, ok := c.Data.(report.HarnessData); !ok || len(data.Integrations) != 6 {
		t.Fatalf("data = %#v", c.Data)
	}
}

func TestHarnessNoneAvailableWarns(t *testing.T) {
	c := checks.Harness(herdr.IntegrationListResult{}, nil)
	if c.Status != report.Warn || c.Detail != "0 targets, none available" {
		t.Fatalf("harness %+v", c)
	}
}

func TestHarnessListFailure(t *testing.T) {
	c := checks.Harness(herdr.IntegrationListResult{}, errors.New("boom"))
	if c.Status != report.Fail || !strings.Contains(c.Detail, "boom") || c.Data != nil {
		t.Fatalf("harness %+v", c)
	}
}
