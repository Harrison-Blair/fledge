// This test lives in the external test package so it may import agentcfg,
// which imports scaffold: the in-package tests cannot, and the stub is only
// worth anything if the real loader accepts it.
package scaffold_test

import (
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// The stub is written under scaffold's constant and read under agentcfg's, so
// the two must name the same file.
func TestAgentsNameMatchesAgentcfg(t *testing.T) {
	if scaffold.AgentsName != agentcfg.FileName {
		t.Errorf("scaffold.AgentsName = %q, agentcfg.FileName = %q", scaffold.AgentsName, agentcfg.FileName)
	}
}

// The catalog is gitignored under scaffold's mirrored name and read under
// agentcfg's, so the entry must track agentcfg.CatalogName.
func TestGitignoreCatalogEntryMatchesAgentcfg(t *testing.T) {
	want := scaffold.DirName + "/" + agentcfg.CatalogName
	for _, entry := range scaffold.GitignoreEntries {
		if entry == want {
			return
		}
	}
	t.Errorf("GitignoreEntries = %v, missing %q", scaffold.GitignoreEntries, want)
}

func TestAgentsStubLoadsAndValidates(t *testing.T) {
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	configs, err := agentcfg.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("got %d configs, want 1: %v", len(configs), configs)
	}
	cfg, ok := configs["example"]
	if !ok {
		t.Fatalf("stub has no \"example\" entry: %v", configs)
	}
	if err := cfg.Validate("example"); err != nil {
		t.Errorf("Validate: %v", err)
	}
}
