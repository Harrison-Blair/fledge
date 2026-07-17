package bootstrap

import (
	"io/fs"
	"strings"
	"testing"
)

// TestCoreSkillsNoPIDLiveness guards FTHR-090 (PLM-035 FC-11): PID liveness
// was never a sound liveness signal (PIDs recycle, and the CLI's own PID is
// dead the instant the command returns), so no scaffolded prose may direct
// agents to consult it. Walks the entire embedded tree — both the
// agent-neutral core/ and every adapters/<harness>/ — because pid-alive
// prose in a shipped adapter reaches a repo just as surely as core prose does.
func TestCoreSkillsNoPIDLiveness(t *testing.T) {
	err := fs.WalkDir(FS, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, rErr := FS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		if strings.Contains(string(data), "pid-alive") {
			t.Errorf("%s: references pid-alive liveness prose, which FTHR-090 removed", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
