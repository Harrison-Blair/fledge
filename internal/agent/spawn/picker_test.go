package spawn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

func pickerBase() Options { return Options{Direction: "right", Timeout: 30 * time.Second} }

func pick(t *testing.T, input string, models map[string][]string, tab func(context.Context) (string, error)) (Options, string, error) {
	t.Helper()
	var out strings.Builder
	p := Picker{In: strings.NewReader(input), Out: &out, Models: func(_ context.Context, h string) []string { return models[h] }, CallerTab: tab}
	o, err := p.Pick(context.Background(), pickerBase())
	return o, out.String(), err
}

func noTab(context.Context) (string, error) { return "", errors.New("unexpected tab lookup") }

func TestPickerDefaults(t *testing.T) {
	o, out, err := pick(t, "\n\nworker\n\n", map[string][]string{"claude": {"sonnet", "opus"}}, noTab)
	if err != nil {
		t.Fatal(err)
	}
	want := pickerBase()
	want.Harness, want.Name = "claude", "worker"
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v", o)
	}
	if !strings.Contains(out, "harness default") || !strings.Contains(out, "opus") {
		t.Fatalf("model list missing: %s", out)
	}
	if !strings.HasSuffix(out, "fledge agent spawn --harness claude --name worker\n") {
		t.Fatalf("summary missing: %q", out)
	}
}

func TestPickerExplicitChoices(t *testing.T) {
	// Harness by name, model by number (sorted: default, m-a, m-b), worktree with explicit branch.
	o, out, err := pick(t, "pi\n3\nprobe\n4\nfeature/x\n", map[string][]string{"pi": {"m-b", "m-a"}}, noTab)
	if err != nil {
		t.Fatal(err)
	}
	want := pickerBase()
	want.Harness, want.Model, want.Name, want.Worktree, want.Branch = "pi", "m-b", "probe", "new", "feature/x"
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("got %+v", o)
	}
	if !strings.HasSuffix(out, "fledge agent spawn --harness pi --model m-b --name probe --worktree new --branch feature/x\n") {
		t.Fatalf("summary: %q", out)
	}
}

func TestPickerHarnessByNumberAndWorktreeDefaultBranch(t *testing.T) {
	o, _, err := pick(t, "1\n\nprobe\n4\n\n", nil, noTab)
	if err != nil {
		t.Fatal(err)
	}
	if o.Harness != libagent.Harnesses()[0] || o.Worktree != "new" || o.Branch != "probe" {
		t.Fatalf("got %+v", o)
	}
}

func TestPickerSplitUsesCallerTab(t *testing.T) {
	for choice, direction := range map[string]string{"2": "right", "3": "down"} {
		lookups := 0
		o, out, err := pick(t, "\n\nprobe\n"+choice+"\n", nil, func(context.Context) (string, error) { lookups++; return "w1:t2", nil })
		if err != nil {
			t.Fatal(err)
		}
		if lookups != 1 || o.TabID != "w1:t2" || o.Direction != direction || !o.DirectionSet {
			t.Fatalf("got %+v lookups=%d", o, lookups)
		}
		if !strings.HasSuffix(out, "--tab-id w1:t2 --direction "+direction+"\n") {
			t.Fatalf("summary: %q", out)
		}
	}
}

func TestPickerSplitTabLookupFailure(t *testing.T) {
	_, _, err := pick(t, "\n\nprobe\n2\n", nil, func(context.Context) (string, error) { return "", errors.New("no pane") })
	if err == nil || err.Error() != "no pane" {
		t.Fatalf("err=%v", err)
	}
}

func TestPickerRejectsInvalidThenAccepts(t *testing.T) {
	input := "nope\n99\nclaude\n7\nsonnet\nBad Name\n\nworker\n0\n1\n"
	o, out, err := pick(t, input, map[string][]string{"claude": {"sonnet"}}, noTab)
	if err != nil {
		t.Fatal(err)
	}
	if o.Harness != "claude" || o.Model != "sonnet" || o.Name != "worker" || o.Worktree != "" {
		t.Fatalf("got %+v", o)
	}
	for _, msg := range []string{"choose a number from 1 to", "--name must match", "name is required"} {
		if !strings.Contains(out, msg) {
			t.Fatalf("missing %q in %s", msg, out)
		}
	}
}

func TestPickerEmptyDiscoveryOffersOnlyDefault(t *testing.T) {
	// No model prompt is read: the second line is the name.
	o, out, err := pick(t, "claude\nworker\n\n", nil, noTab)
	if err != nil {
		t.Fatal(err)
	}
	if o.Model != "" || o.Name != "worker" || !strings.Contains(out, "Model: harness default") {
		t.Fatalf("got %+v\n%s", o, out)
	}
}

func TestPickerHarnessWithoutModelMapping(t *testing.T) {
	called := false
	var out strings.Builder
	p := Picker{In: strings.NewReader("amp\nworker\n\n"), Out: &out, Models: func(context.Context, string) []string { called = true; return []string{"x"} }, CallerTab: noTab}
	o, err := p.Pick(context.Background(), pickerBase())
	if err != nil {
		t.Fatal(err)
	}
	if called || o.Harness != "amp" || o.Model != "" || o.Name != "worker" {
		t.Fatalf("got %+v called=%v", o, called)
	}
}

func TestPickerEOFAborts(t *testing.T) {
	for _, input := range []string{"", "claude\n", "claude\n\n", "claude\n\nworker\n", "claude\n\nworker\n4\n"} {
		_, _, err := pick(t, input, map[string][]string{"claude": {"sonnet"}}, noTab)
		var invalid *libagent.InputError
		if !errors.As(err, &invalid) || err.Error() != "spawn canceled" {
			t.Fatalf("input %q: err=%v", input, err)
		}
	}
}

func TestPickerContextCancelAborts(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := Picker{In: r, Out: io.Discard, Models: func(context.Context, string) []string { return nil }, CallerTab: noTab}
	_, err := p.Pick(ctx, pickerBase())
	var invalid *libagent.InputError
	if !errors.As(err, &invalid) || err.Error() != "spawn canceled" {
		t.Fatalf("err=%v", err)
	}
}

func TestPickerPartialFinalAnswerAborts(t *testing.T) {
	for _, input := range []string{"amp\nworker\n1", "amp\nworker\n4\nfeature"} {
		_, _, err := pick(t, input, nil, noTab)
		var invalid *libagent.InputError
		if !errors.As(err, &invalid) || err.Error() != "spawn canceled" {
			t.Fatalf("input %q: err=%v", input, err)
		}
	}
}

func TestPickerRejectsInvalidBranch(t *testing.T) {
	o, out, err := pick(t, "amp\nworker\n4\nbad\x00branch\n\xff\ngood\n", nil, noTab)
	if err != nil {
		t.Fatal(err)
	}
	if o.Worktree != "new" || o.Branch != "good" {
		t.Fatalf("got %+v", o)
	}
	if strings.Count(out, "arguments must be valid UTF-8 without NUL") != 2 {
		t.Fatalf("missing branch errors: %s", out)
	}
	if _, err := o.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPickerRejectsBranchGitRejects(t *testing.T) {
	o, out, err := pick(t, "amp\nworker\n4\na..b\nhas space\n@{-1}\ngood\n", nil, noTab)
	if err != nil {
		t.Fatal(err)
	}
	if o.Branch != "good" {
		t.Fatalf("got %+v", o)
	}
	for _, branch := range []string{"a..b", "has space", "@{-1}"} {
		if !strings.Contains(out, fmt.Sprintf("invalid exact branch name %q", branch)) {
			t.Fatalf("missing %q error: %s", branch, out)
		}
	}
}

func TestPickerBranchCheckRuntimeError(t *testing.T) {
	t.Setenv("PATH", "")
	_, _, err := pick(t, "amp\nworker\n4\ngood\n", nil, noTab)
	var invalid *libagent.InputError
	if err == nil || errors.As(err, &invalid) {
		t.Fatalf("err=%v", err)
	}
}

// TestPickerCancelDuringBranchCheck cancels while a stalled git checks the
// branch, which must abort like any other cancellation.
func TestPickerCancelDuringBranchCheck(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\n: >"+marker+"\nexec "+sleep+" 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			if _, err := os.Stat(marker); err == nil {
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	p := Picker{In: strings.NewReader("amp\nworker\n4\ngood\n"), Out: io.Discard, Models: func(context.Context, string) []string { return nil }, CallerTab: noTab}
	_, err = p.Pick(ctx, pickerBase())
	var invalid *libagent.InputError
	if !errors.As(err, &invalid) || err.Error() != "spawn canceled" {
		t.Fatalf("err=%v", err)
	}
}

func TestPickerFinalValidationRejectsBadTab(t *testing.T) {
	_, out, err := pick(t, "amp\nworker\n2\n", nil, func(context.Context) (string, error) { return "bad\x00tab", nil })
	var invalid *libagent.InputError
	if !errors.As(err, &invalid) || strings.Contains(out, "fledge agent spawn") {
		t.Fatalf("err=%v out=%q", err, out)
	}
}
