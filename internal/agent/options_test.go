package agent

import (
	"reflect"
	"testing"
	"time"
)

func validOptions() SpawnOptions {
	return SpawnOptions{Name: "worker", Harness: "claude", Timeout: 30 * time.Second, Direction: "right"}
}
func TestSpawnValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SpawnOptions)
	}{
		{"missing name", func(o *SpawnOptions) { o.Name = "" }},
		{"invalid name", func(o *SpawnOptions) { o.Name = "Upper" }},
		{"harness", func(o *SpawnOptions) { o.Harness = "nope" }},
		{"timeout", func(o *SpawnOptions) { o.Timeout = 3000 * time.Millisecond }},
		{"workspace selectors", func(o *SpawnOptions) { o.Workspace = "a"; o.WorkspaceID = "w1" }},
		{"pane direction", func(o *SpawnOptions) { o.Pane = "p"; o.DirectionSet = true }},
		{"worktree env", func(o *SpawnOptions) { o.Worktree = "new"; o.Env = []string{"K=V"} }},
		{"branch ordinary", func(o *SpawnOptions) { o.Branch = "x" }},
		{"bad env", func(o *SpawnOptions) { o.Env = []string{"bad"} }},
		{"bad ratio", func(o *SpawnOptions) { r := 2.0; o.Ratio = &r }},
		{"no-wait with prompt", func(o *SpawnOptions) { o.NoWait = true; o.PromptSet = true }},
		{"no-wait with file", func(o *SpawnOptions) { o.NoWait = true; o.FileSet = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := validOptions()
			tc.change(&o)
			if _, err := o.Validate(); err == nil {
				t.Fatal("accepted invalid flags")
			}
		})
	}
}
func TestModelArguments(t *testing.T) {
	for _, h := range []string{"pi", "claude", "codex", "gemini", "cursor", "devin", "agy", "cline", "omp", "opencode", "copilot", "kimi", "droid", "grok", "kilo", "qwen", "letta", "maki", "hermes", "kiro", "amp", "muse", "mastracode", "qodercli"} {
		t.Run(h, func(t *testing.T) {
			o := validOptions()
			o.Harness = h
			o.Model = "a model"
			o.Args = []string{"--flag=a,b", "two words"}
			got, err := o.Validate()
			unsupported := h == "kiro" || h == "amp" || h == "muse" || h == "mastracode" || h == "qodercli"
			if unsupported {
				if err == nil {
					t.Fatal("unverified model accepted")
				}
				o.Model = ""
				if _, err = o.Validate(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"--model", "a model", "--flag=a,b", "two words"}
			if h == "hermes" {
				want = append([]string{"chat"}, want...)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%q want %q", got, want)
			}
		})
	}
}
func TestModelConflicts(t *testing.T) {
	for _, args := range [][]string{{"--model=x"}, {"-m", "x"}, {"-mx"}, {"-m=x"}, {"-c", "model='x'"}, {"--config=model='x'"}, {"-cmodel='x'"}, {"--config", `"model" = 'x'`}} {
		o := validOptions()
		o.Harness = "codex"
		o.Model = "mine"
		o.Args = args
		if _, err := o.Validate(); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	o := validOptions()
	o.Model = "mine"
	o.Args = []string{"--", "--model=x"}
	if _, err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	o.Harness = "hermes"
	o.Args = []string{"chat", "hello"}
	got, err := o.Validate()
	if err != nil || !reflect.DeepEqual(got, []string{"chat", "--model", "mine", "hello"}) {
		t.Fatalf("%q %v", got, err)
	}
}
