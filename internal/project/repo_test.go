package project

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCommittedProjectFilesMatchGeneratedContents pins the files this
// repository commits for its own coordinator to what fledge init generates, so
// they cannot drift from the guidance a freshly initialized project receives.
func TestCommittedProjectFilesMatchGeneratedContents(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	assertFileContents(t, filepath.Join(root, stateDirectory, profilesDir, profileFilename), defaultProfileContents)
	assertFileContents(t, filepath.Join(root, ".codex", "rules", "fledge.rules"), codexRulesContents)
	if _, err := os.Stat(filepath.Join(root, stateDirectory, "watch.json")); !os.IsNotExist(err) {
		t.Fatalf("removed watch.json still exists or cannot be inspected: %v", err)
	}

	profile, err := LoadOrchestratorProfile(root)
	if err != nil {
		t.Fatalf("LoadOrchestratorProfile() error = %v", err)
	}
	if profile.SchemaVersion != SchemaVersion || profile.Instructions != DefaultOrchestratorInstructions {
		t.Errorf("committed profile = %#v, want default profile", profile)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
}
