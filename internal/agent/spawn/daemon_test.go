package spawn

import (
	"reflect"
	"testing"
)

func TestSpawnCodexRunsWithoutSharedDaemon(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		bypassOff   bool
		args, want  []string
	}{
		{"default", "", false, nil, []string{"--yolo", "--no-daemon"}},
		{"model", "chosen", false, []string{"--search"}, []string{"--yolo", "--no-daemon", "--model", "chosen", "--search"}},
		{"permission opt-out", "chosen", true, nil, []string{"--no-daemon", "--model", "chosen"}},
		{"native permissions", "", false, []string{"--sandbox", "read-only"}, []string{"--no-daemon", "--sandbox", "read-only"}},
		{"config override", "", false, []string{"-c", "model_reasoning_effort='high'"}, []string{"--yolo", "--no-daemon", "-c", "model_reasoning_effort='high'"}},
		{"after terminator", "", false, []string{"--", "--no-daemon"}, []string{"--yolo", "--no-daemon", "--", "--no-daemon"}},
		{"model value", "--no-daemon", false, nil, []string{"--yolo", "--no-daemon", "--model", "--no-daemon"}},
		{"profile value", "", false, []string{"-p", "--no-daemon"}, []string{"--yolo", "--no-daemon", "-p", "--no-daemon"}},
		{"resume", "", false, []string{"resume", "R"}, []string{"--yolo", "--no-daemon", "resume", "R"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Harness, o.Model, o.Args, o.NoPermissionBypass = "codex", tc.model, tc.args, tc.bypassOff
			got, err := o.Validate()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestSpawnCodexKeepsExplicitNoDaemon(t *testing.T) {
	for _, tc := range []struct {
		name       string
		bypassOff  bool
		args, want []string
	}{
		{"alone", false, []string{"--no-daemon"}, []string{"--yolo", "--no-daemon"}},
		{"later", false, []string{"--search", "--no-daemon"}, []string{"--yolo", "--search", "--no-daemon"}},
		{"permission opt-out", true, []string{"--no-daemon"}, []string{"--no-daemon"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Harness, o.Args, o.NoPermissionBypass = "codex", tc.args, tc.bypassOff
			got, err := o.Validate()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
