package spawn

import (
	"reflect"
	"testing"
)

func TestSpawnPermissionDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, harness, model string
		args, want           []string
	}{
		{"codex", "codex", "", nil, []string{"--yolo"}},
		{"claude", "claude", "", nil, []string{"--permission-mode", "bypassPermissions"}},
		{"codex model", "codex", "chosen", []string{"--search"}, []string{"--yolo", "--model", "chosen", "--search"}},
		{"claude model", "claude", "chosen", []string{"two words", "x,y"}, []string{"--permission-mode", "bypassPermissions", "--model", "chosen", "two words", "x,y"}},
		{"codex terminator", "codex", "", []string{"--", "--sandbox", "read-only"}, []string{"--yolo", "--", "--sandbox", "read-only"}},
		{"claude terminator", "claude", "", []string{"--", "--permission-mode", "plan"}, []string{"--permission-mode", "bypassPermissions", "--", "--permission-mode", "plan"}},
		{"model value is not an option", "codex", "--sandbox", nil, []string{"--yolo", "--model", "--sandbox"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			o.Harness, o.Model, o.Args = tc.harness, tc.model, tc.args
			got, err := o.Validate()
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestSpawnPermissionOptOut(t *testing.T) {
	for _, kind := range []string{"claude", "codex", "pi"} {
		for _, args := range [][]string{nil, {"two words", "x,y"}, {"--", "--yolo"}} {
			o := validOptions()
			o.Harness, o.Model, o.Args, o.NoPermissionBypass = kind, "chosen", args, true
			got, err := o.Validate()
			want := append([]string{"--model", "chosen"}, args...)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: got %q, %v; want %q", kind, got, err, want)
			}
		}
	}
}

func TestSpawnNativePermissionsOverrideDefaults(t *testing.T) {
	cases := map[string][][]string{
		"claude": {{"--permission-mode", "plan"}, {"--permission-mode=auto"}, {"--permission-mode", "bypassPermissions"}, {"--dangerously-skip-permissions"}, {"-p", "--permission-mode", "plan"}},
		"codex": {
			{"--yolo"}, {"--dangerously-bypass-approvals-and-sandbox"},
			{"--sandbox", "read-only"}, {"--sandbox=workspace-write"}, {"-s", "read-only"}, {"-sread-only"}, {"-s=read-only"},
			{"--ask-for-approval", "on-request"}, {"--ask-for-approval=never"}, {"-a", "never"}, {"-anever"}, {"-a=never"},
			{"--approve-for-me"}, {"--full-auto"},
			{"-c", "approval_policy='on-request'"}, {"--config", "sandbox_mode='read-only'"},
			{"--config=default_permissions='workspace'"}, {"-csandbox_mode='read-only'"}, {"-c=approval_policy='never'"},
			{"--config", `"approval_policy" = 'never'`}, {"-c", "permissions.workspace.filesystem.default='read'"},
		},
	}
	for harness, cases := range cases {
		for _, args := range cases {
			t.Run(harness+" "+args[0]+" "+args[len(args)-1], func(t *testing.T) {
				o := validOptions()
				o.Harness, o.Args = harness, args
				got, err := o.Validate()
				if err != nil || !reflect.DeepEqual(got, args) {
					t.Fatalf("got %q, %v; want unchanged %q", got, err, args)
				}
			})
		}
	}
}

func TestSpawnPermissionDefaultsPreserveUnrelatedOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--config", "model_reasoning_effort='high'"},
		{"-c", "not_approval_policy='never'"},
		{"--config=web_search='live'"},
		{"-cmodel_reasoning_effort='high'"},
		{"--config", "note='--yolo'"},
	} {
		o := validOptions()
		o.Harness, o.Args = "codex", args
		got, err := o.Validate()
		want := append([]string{"--yolo"}, args...)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
	}
	for _, kind := range []string{"pi", "gemini", "hermes"} {
		o := validOptions()
		o.Harness, o.Args = kind, []string{"two words", "x,y"}
		got, err := o.Validate()
		if err != nil || !reflect.DeepEqual(got, o.Args) {
			t.Fatalf("%s: got %q, %v; want %q", kind, got, err, o.Args)
		}
	}
}
