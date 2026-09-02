package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedFragmentsMatchSources(t *testing.T) {
	tests := []struct {
		name     string
		embedded string
		path     string
	}{
		{"core", coreFragment, "fledge-core.md"},
		{"general", generalWorkerFragment, "fledge-general.md"},
		{"orchestrator", orchestratorRoleRules, "fledge-orchestrator.md"},
		{"worker-report", workerReportFragment, "fledge-worker-report.md"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join("profiles", test.path)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if string(source) != test.embedded {
				t.Fatalf("embedded %s differs from %s", test.name, path)
			}
		})
	}
}
