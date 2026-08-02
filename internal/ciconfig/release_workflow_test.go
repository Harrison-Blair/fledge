package ciconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseWorkflowContract(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	workflow := string(contents)
	required := []string{
		"name: Release",
		"branches: [main]",
		"bash scripts/check-release-version.sh",
		"go test -trimpath -buildvcs=true -race ./...",
		"go vet ./...",
		"for arch in amd64 arm64",
		"gh release create",
		"--generate-notes",
		"contents: write",
	}
	for _, fragment := range required {
		if !strings.Contains(workflow, fragment) {
			t.Errorf("release workflow is missing %q", fragment)
		}
	}
}

func TestReleaseLicenseContract(t *testing.T) {
	path := filepath.Join("..", "..", "LICENSE")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release license: %v", err)
	}

	license := string(contents)
	for _, fragment := range []string{
		"GNU AFFERO GENERAL PUBLIC LICENSE",
		"Version 3, 19 November 2007",
	} {
		if !strings.Contains(license, fragment) {
			t.Errorf("release license is missing %q", fragment)
		}
	}
}
