package doctest

import (
	"strings"
	"testing"
)

// TestClaudeMdReferencesRoleFiles: root CLAUDE.md lists the per-role
// protocol files (incubator.md, brooder.md, skua.md) in its description of
// the embedded core content.
func TestClaudeMdReferencesRoleFiles(t *testing.T) {
	claudeMd := readRoot(t, "CLAUDE.md")

	for _, file := range []string{"incubator.md", "brooder.md", "skua.md"} {
		if !strings.Contains(claudeMd, file) {
			t.Errorf("CLAUDE.md must reference %q", file)
		}
	}
}
