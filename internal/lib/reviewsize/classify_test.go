package reviewsize

import "testing"

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		path                 string
		source, test, guards bool
	}{
		{"internal/agent/spawn/daemon.go", true, false, true},
		{"internal/agent/spawn/daemon_test.go", false, true, true},
		{"internal/x/testdata/a.go", false, false, false},
		{"internal/lib/profiles/builtin/orchestrator.md", true, false, true},
		{".github/scripts/x.py", true, false, true},
		{".github/scripts/x_test.py", false, true, true},
		{".github/workflows/ci.yml", true, false, true},
		{"README.md", false, false, true},
		{"reference/usage/tasks.md", false, false, true},
		{"reference/herdr/raw/x.txt", false, false, false},
		{"testdata/ste-baseline.json", false, false, false},
	} {
		c := classify(tc.path)
		if c.source != tc.source || c.test != tc.test || c.guarded != tc.guards {
			t.Errorf("%s: source %v test %v guarded %v, want %v %v %v", tc.path, c.source, c.test, c.guarded, tc.source, tc.test, tc.guards)
		}
	}
}

func TestDomains(t *testing.T) {
	for _, tc := range []struct {
		path, code, doc string
	}{
		{"cmd/agent/spawn/spawn.go", "agent", ""},
		{"internal/agent/spawn/daemon.go", "agent", ""},
		{"internal/lib/task/task.go", "lib/task", ""},
		{"cmd/help_test.go", "cmd root", ""},
		{"main.go", "module root", ""},
		{"ste_test.go", "module root", ""},
		{".github/workflows/ci.yml", ".github", ""},
		{"testdata/ste-baseline.json", "", ""},
		{"reference/usage/tasks.md", "", "reference/usage"},
		{"README.md", "", ""},
		{"AGENTS.md", "", ""},
		{"internal/x/testdata/a.go", "", ""},
	} {
		c := classify(tc.path)
		if c.domain != tc.code || c.docArea != tc.doc {
			t.Errorf("%s: domain %q area %q, want %q %q", tc.path, c.domain, c.docArea, tc.code, tc.doc)
		}
	}
}
