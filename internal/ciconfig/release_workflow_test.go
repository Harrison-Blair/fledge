// Package ciconfig validates the structure of fledge's GitHub Actions
// workflow files by parsing them as YAML rather than asserting on raw text.
package ciconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// loadReleaseWorkflow parses .github/workflows/release.yml relative to the
// repo root (two levels up from this package) into a generic structure.
func loadReleaseWorkflow(t *testing.T) map[string]any {
	t.Helper()
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing %s as YAML: %v", path, err)
	}
	return doc
}

// allSteps flattens every step's `run` string across every job in the
// workflow, so assertions can check "does this command appear anywhere"
// without hard-coding job/step layout.
func allRunCommands(doc map[string]any) []string {
	var cmds []string
	jobs, _ := doc["jobs"].(map[string]any)
	for _, j := range jobs {
		job, ok := j.(map[string]any)
		if !ok {
			continue
		}
		steps, _ := job["steps"].([]any)
		for _, s := range steps {
			step, ok := s.(map[string]any)
			if !ok {
				continue
			}
			if run, ok := step["run"].(string); ok {
				cmds = append(cmds, run)
			}
		}
	}
	return cmds
}

func containsSubstring(cmds []string, substr string) bool {
	for _, c := range cmds {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestReleaseWorkflow_TriggersOnPushToMain(t *testing.T) {
	doc := loadReleaseWorkflow(t)

	// YAML unmarshals the bare key `on` as the string "on" in Go's
	// map[string]any (goccy/go-yaml preserves it as written, not as a bool
	// key), so look it up directly.
	on, ok := doc["on"]
	if !ok {
		t.Fatalf("workflow missing top-level `on` trigger: %#v", doc)
	}
	onMap, ok := on.(map[string]any)
	if !ok {
		t.Fatalf("`on` is not a map: %#v", on)
	}
	push, ok := onMap["push"].(map[string]any)
	if !ok {
		t.Fatalf("`on.push` missing or not a map: %#v", onMap)
	}
	branches, ok := push["branches"].([]any)
	if !ok {
		t.Fatalf("`on.push.branches` missing or not a list: %#v", push)
	}
	found := false
	for _, b := range branches {
		if b == "main" {
			found = true
		}
	}
	if !found {
		t.Errorf("`on.push.branches` does not include \"main\": %#v", branches)
	}
}

func TestReleaseWorkflow_RunsSafetyNet(t *testing.T) {
	doc := loadReleaseWorkflow(t)
	cmds := allRunCommands(doc)

	want := []string{"gofmt -l .", "go vet ./...", "go build ./...", "go test ./..."}
	for _, w := range want {
		if !containsSubstring(cmds, w) {
			t.Errorf("safety-net command %q not found in any step's `run`; commands seen: %v", w, cmds)
		}
	}
}

func TestReleaseWorkflow_BuildsLinuxAmd64AndReleases(t *testing.T) {
	doc := loadReleaseWorkflow(t)
	cmds := allRunCommands(doc)

	if !containsSubstring(cmds, "GOOS=") || !containsSubstring(cmds, "GOARCH=") {
		t.Errorf("no step builds with a GOOS/GOARCH pair; commands seen: %v", cmds)
	}
	if !containsSubstring(cmds, "fledge_linux_amd64") {
		t.Errorf("no step references a fledge_linux_amd64 archive; commands seen: %v", cmds)
	}
	if !containsSubstring(cmds, "gh release create") {
		t.Errorf("no step runs `gh release create`; commands seen: %v", cmds)
	}
	if !containsSubstring(cmds, "--generate-notes") {
		t.Errorf("no step passes `--generate-notes` to the release-creation command; commands seen: %v", cmds)
	}
}

// matrixIncludePairs finds the first job with a `strategy.matrix.include`
// list and returns its {goos, goarch} entries.
func matrixIncludePairs(t *testing.T, doc map[string]any) []map[string]any {
	t.Helper()
	jobs, _ := doc["jobs"].(map[string]any)
	for _, j := range jobs {
		job, ok := j.(map[string]any)
		if !ok {
			continue
		}
		strategy, ok := job["strategy"].(map[string]any)
		if !ok {
			continue
		}
		matrix, ok := strategy["matrix"].(map[string]any)
		if !ok {
			continue
		}
		include, ok := matrix["include"].([]any)
		if !ok {
			continue
		}
		var pairs []map[string]any
		for _, e := range include {
			entry, ok := e.(map[string]any)
			if !ok {
				continue
			}
			pairs = append(pairs, entry)
		}
		return pairs
	}
	return nil
}

func TestReleaseWorkflow_BuildsAllFivePlatforms(t *testing.T) {
	doc := loadReleaseWorkflow(t)
	pairs := matrixIncludePairs(t, doc)

	want := []string{
		"linux/amd64",
		"linux/arm64",
		"darwin/amd64",
		"darwin/arm64",
		"windows/amd64",
	}

	got := make(map[string]bool)
	for _, p := range pairs {
		goos, _ := p["goos"].(string)
		goarch, _ := p["goarch"].(string)
		got[goos+"/"+goarch] = true
	}

	for _, w := range want {
		if !got[w] {
			t.Errorf("matrix.include missing %q; entries seen: %#v", w, pairs)
		}
	}
}
