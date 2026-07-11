// Package ciconfig validates the static structure of repo CI workflow files.
// It contains no production code — it exists solely to pin FC-1 of FTHR-022
// (PR check workflow lint/build/test with required branch protection) by
// asserting the shape of .github/workflows/pr-check.yml.
package ciconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// workflow is a minimal structural view of a GitHub Actions workflow file,
// capturing only the fields this test needs to assert on.
type workflow struct {
	On struct {
		PullRequest struct {
			Branches []string `yaml:"branches"`
		} `yaml:"pull_request"`
	} `yaml:"on"`
	Jobs map[string]struct {
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadPRCheckWorkflow(t *testing.T) workflow {
	t.Helper()

	path := filepath.Join("..", "..", ".github", "workflows", "pr-check.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var w workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		t.Fatalf("unmarshaling %s: %v", path, err)
	}
	return w
}

func TestPRCheckWorkflow_TriggersOnMainPRs(t *testing.T) {
	w := loadPRCheckWorkflow(t)

	found := false
	for _, b := range w.On.PullRequest.Branches {
		if b == "main" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected on.pull_request.branches to include %q, got %v", "main", w.On.PullRequest.Branches)
	}
}

func TestPRCheckWorkflow_RunsLintBuildTest(t *testing.T) {
	w := loadPRCheckWorkflow(t)

	var allRuns []string
	for _, job := range w.Jobs {
		for _, step := range job.Steps {
			allRuns = append(allRuns, step.Run)
		}
	}
	combined := strings.Join(allRuns, "\n")

	wantSubstrings := []string{
		"gofmt -l",
		"go vet ./...",
		"go build ./...",
		"go test ./...",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(combined, want) {
			t.Errorf("expected some step's run command to contain %q; run commands were: %v", want, allRuns)
		}
	}
}
