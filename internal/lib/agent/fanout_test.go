package agent

import (
	"reflect"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

func TestFanOutFailureSummarizesFailedTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		failed []TargetFailure
		want   *cli.Failure
	}{
		{"none", nil, nil},
		{"one shared code", []TargetFailure{{"a", "timeout"}, {"c", "timeout"}}, &cli.Failure{Code: "timeout", Message: "2 of 3 targets not stopped cleanly: a (timeout), c (timeout)", Phase: "agent.stop"}},
		{"several codes", []TargetFailure{{"a", "timeout"}, {"b", "agent_not_found"}}, &cli.Failure{Code: "operation_failed", Message: "2 of 3 targets not stopped cleanly: a (timeout), b (agent_not_found)", Phase: "agent.stop"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := FanOutFailure(tc.failed, 3, "not stopped cleanly", "agent.stop"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("FanOutFailure = %+v, want %+v", got, tc.want)
			}
		})
	}
}
