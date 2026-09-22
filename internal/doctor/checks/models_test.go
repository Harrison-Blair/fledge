package checks_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/doctor/checks"
	"github.com/Harrison-Blair/fledge/internal/doctor/report"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

func TestModelAvailableDiscoveryErrorFails(t *testing.T) {
	// pi is available but its store is malformed -> fail, without running commands.
	home := homeWithPi(t, `{"broken":`)
	m := checks.Models(context.Background(), fatalRunDiscovery(t, home), integrationList("pi"), nil)
	if m.Name != "model_discovery" || m.Status != report.Fail {
		t.Fatalf("model %+v", m)
	}
	if !strings.Contains(m.Detail, "pi:fail") {
		t.Fatalf("detail should name pi: %+v", m)
	}
}

func TestModelAvailableWithModelsIsOK(t *testing.T) {
	// pi is available and returns models; the other read-only kinds are unavailable.
	home := homeWithPi(t, `{"prov":{"models":[{"id":"m1","name":"M1"}]}}`)
	m := checks.Models(context.Background(), fatalRunDiscovery(t, home), integrationList("pi"), nil)
	// pi ok, codex/claude warn (unavailable) -> overall warn, never fail.
	if m.Status != report.Warn || m.Detail != "pi:ok(1) codex:warn(0) claude:warn(0)" {
		t.Fatalf("model %+v", m)
	}
	pi := m.Data.(report.ModelData).Harnesses[0]
	if pi.Status != report.OK || pi.Count != 1 || pi.Detail != "1 models" || pi.Available == nil || !*pi.Available {
		t.Fatalf("pi = %+v", pi)
	}
}

func TestModelDiscoveryIsReadOnlyAndExcludesCommandKinds(t *testing.T) {
	// Even with every kind marked available, doctor covers only the read-only
	// file sources and never invokes the command runner.
	disc := fatalRunDiscovery(t, homeWithPi(t, `{"prov":{"models":[{"id":"m1"}]}}`))
	m := checks.Models(context.Background(), disc, integrationList("pi", "codex", "claude", "opencode", "cursor"), nil)
	var kinds []string
	for _, h := range m.Data.(report.ModelData).Harnesses {
		kinds = append(kinds, h.Harness)
	}
	if !reflect.DeepEqual(kinds, []string{"pi", "codex", "claude"}) {
		t.Fatalf("model kinds = %q, want pi/codex/claude only", kinds)
	}
}

func TestModelUnavailableIsWarnNotFail(t *testing.T) {
	// Nothing available: every model kind is warn, none fail.
	m := checks.Models(context.Background(), discovery(), integrationList(), nil)
	if m.Status != report.Warn {
		t.Fatalf("model %+v", m)
	}
	for _, h := range m.Data.(report.ModelData).Harnesses {
		if h.Status != report.Warn || h.Detail != "harness not installed" {
			t.Fatalf("harness %+v", h)
		}
	}
}

func TestModelAvailabilityUnknownWhenListFails(t *testing.T) {
	// integration.list failed: availability is unknown, discovery still attempted.
	m := checks.Models(context.Background(), discovery(), herdr.IntegrationListResult{}, errors.New("down"))
	if !strings.Contains(m.Detail, "pi") {
		t.Fatalf("model %+v", m)
	}
	// With availability unknown, a discovery error is a warning, never a failure.
	if m.Status == report.Fail {
		t.Fatalf("unknown availability must not fail: %+v", m)
	}
	pi := m.Data.(report.ModelData).Harnesses[0]
	if pi.Available != nil || !strings.HasPrefix(pi.Detail, "availability unknown; discovery error: ") {
		t.Fatalf("pi = %+v", pi)
	}
}

func TestModelAvailabilityUnknownWithModels(t *testing.T) {
	home := homeWithPi(t, `{"prov":{"models":[{"id":"m1"}]}}`)
	m := checks.Models(context.Background(), fatalRunDiscovery(t, home), herdr.IntegrationListResult{}, errors.New("down"))
	if pi := m.Data.(report.ModelData).Harnesses[0]; pi.Status != report.OK || pi.Detail != "1 models (availability unknown)" {
		t.Fatalf("pi = %+v", pi)
	}
}
