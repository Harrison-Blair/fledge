package bootstrap

import (
	"strings"
	"testing"
)

// TestCoreDocsRepointToRoleFiles: the core orchestration docs no longer
// reference worker-protocols.md (every former `worker-protocols.md`
// §Incubator-style site) and instead point at the per-role files
// (incubator.md). The bare filename is checked rather than the literal
// "worker-protocols.md §" pattern because the docs backtick the filename
// (`worker-protocols.md` §Incubator), which that literal never matches.
func TestCoreDocsRepointToRoleFiles(t *testing.T) {
	for _, name := range []string{"planning.md", "implementation.md", "foraging.md"} {
		data, err := FS.ReadFile("core/skills/fledge-orchestrate/" + name)
		if err != nil {
			t.Fatal(err)
		}
		doc := string(data)

		if strings.Contains(doc, "worker-protocols.md") {
			t.Errorf("%s still references %q", name, "worker-protocols.md")
		}
		if !strings.Contains(doc, "incubator.md") {
			t.Errorf("%s must reference %q", name, "incubator.md")
		}
	}
}
