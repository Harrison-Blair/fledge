package orchestratorcontext

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSynchronizeCreatesManagedContextAndIsIdempotent(t *testing.T) {
	root := contextProject(t)
	instructions := "Use inherited Fledge.\nWait atomically.\n"
	if err := Synchronize(root, instructions); err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(root, "AGENTS.md")
	claudePath := filepath.Join(root, "CLAUDE.md")
	wantAgents := beginMarker + "\n## Fledge Orchestrator (managed)\n\n" +
		"Use inherited Fledge.\nWait atomically.\n" + endMarker + "\n"
	wantClaude := beginMarker + "\n@AGENTS.md\n" + endMarker + "\n"
	if got := readContext(t, agentsPath); got != wantAgents {
		t.Fatalf("AGENTS.md = %q, want %q", got, wantAgents)
	}
	if got := readContext(t, claudePath); got != wantClaude {
		t.Fatalf("CLAUDE.md = %q, want %q", got, wantClaude)
	}
	before, err := os.Stat(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != 0o644 {
		t.Fatalf("created mode = %o, want 644", before.Mode().Perm())
	}
	if err := Synchronize(root, instructions); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("idempotent synchronization replaced an unchanged file")
	}
}

func TestSynchronizeReplacesOnlyManagedBlockAndPreservesCRLFAndMode(t *testing.T) {
	root := contextProject(t)
	path := filepath.Join(root, "AGENTS.md")
	oldBlock := withLineEnding(agentsBlock("Old policy."), "\r\n")
	original := "# User context\r\n\r\n" + oldBlock + "\r\n\r\nUser tail\r\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Synchronize(root, "New policy.\nSecond line."); err != nil {
		t.Fatal(err)
	}
	want := "# User context\r\n\r\n" +
		withLineEnding(agentsBlock("New policy.\nSecond line."), "\r\n") +
		"\r\n\r\nUser tail\r\n"
	if got := readContext(t, path); got != want {
		t.Fatalf("updated content = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("preserved mode = %o, want 640", info.Mode().Perm())
	}
}

func TestSynchronizeClaudeBridgeAvoidsExistingImportAndAgentsSymlink(t *testing.T) {
	t.Run("existing import", func(t *testing.T) {
		root := contextProject(t)
		claude := filepath.Join(root, "CLAUDE.md")
		original := "# Claude context\n\n@AGENTS.md\n"
		if err := os.WriteFile(claude, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Synchronize(root, "Managed policy."); err != nil {
			t.Fatal(err)
		}
		if got := readContext(t, claude); got != original {
			t.Fatalf("CLAUDE.md changed = %q", got)
		}
	})

	t.Run("symlink to AGENTS", func(t *testing.T) {
		root := contextProject(t)
		agents := filepath.Join(root, "AGENTS.md")
		if err := os.WriteFile(agents, []byte("# User\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		claude := filepath.Join(root, "CLAUDE.md")
		if err := os.Symlink("AGENTS.md", claude); err != nil {
			t.Fatal(err)
		}
		if err := Synchronize(root, "Managed policy."); err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(claude)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatal("CLAUDE.md symlink was replaced")
		}
		if count := strings.Count(readContext(t, agents), managedToken); count != 2 {
			t.Fatalf("managed marker token count = %d, want 2", count)
		}
	})
}

func TestSynchronizeEmptyInstructionsRemovesOnlyOwnedBlocks(t *testing.T) {
	t.Run("generated files", func(t *testing.T) {
		root := contextProject(t)
		if err := Synchronize(root, "Managed policy."); err != nil {
			t.Fatal(err)
		}
		if err := Synchronize(root, ""); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				t.Fatalf("generated-only %s still exists: %v", name, err)
			}
		}
	})

	t.Run("surrounding user content", func(t *testing.T) {
		root := contextProject(t)
		path := filepath.Join(root, "AGENTS.md")
		original := "# User\n\n" + agentsBlock("Old.") + "\n\nKeep me.\n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Synchronize(root, ""); err != nil {
			t.Fatal(err)
		}
		if got, want := readContext(t, path), "# User\n\n\n\nKeep me.\n"; got != want {
			t.Fatalf("remaining content = %q, want %q", got, want)
		}
	})
}

func TestSynchronizeRejectsMalformedMarkersWithoutChangingEitherFile(t *testing.T) {
	tests := map[string]string{
		"missing end": beginMarker + "\nmanaged\n",
		"duplicate":   beginMarker + "\n" + endMarker + "\n" + beginMarker + "\n" + endMarker + "\n",
		"reordered":   endMarker + "\n" + beginMarker + "\n",
		"inline":      "prefix " + beginMarker + "\n" + endMarker + "\n",
		"partial":     "<!-- " + managedToken + " -->\n",
	}
	for name, malformed := range tests {
		t.Run(name, func(t *testing.T) {
			root := contextProject(t)
			agents := filepath.Join(root, "AGENTS.md")
			claude := filepath.Join(root, "CLAUDE.md")
			if err := os.WriteFile(agents, []byte("unchanged\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(claude, []byte(malformed), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Synchronize(root, "Managed."); err == nil || !strings.Contains(err.Error(), "managed-block markers") {
				t.Fatalf("error = %v", err)
			}
			if got := readContext(t, agents); got != "unchanged\n" {
				t.Fatalf("AGENTS.md changed before CLAUDE.md validation: %q", got)
			}
			if got := readContext(t, claude); got != malformed {
				t.Fatalf("malformed CLAUDE.md changed: %q", got)
			}
		})
	}
}

func TestSynchronizeRejectsReservedInstructionsAndUnsafeOrUnwritableFiles(t *testing.T) {
	t.Run("reserved marker", func(t *testing.T) {
		root := contextProject(t)
		if err := Synchronize(root, "do not inject "+beginMarker); err == nil ||
			!strings.Contains(err.Error(), "reserved") {
			t.Fatalf("error = %v", err)
		}
	})

	for _, test := range []struct {
		name    string
		prepare func(*testing.T, string)
	}{
		{name: "AGENTS symlink", prepare: func(t *testing.T, root string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("safe"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "AGENTS.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "CLAUDE unsafe symlink", prepare: func(t *testing.T, root string) {
			if err := os.Symlink("outside.md", filepath.Join(root, "CLAUDE.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "CLAUDE read only", prepare: func(t *testing.T, root string) {
			contents := beginMarker + "\nold\n" + endMarker + "\n"
			if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(contents), 0o444); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := contextProject(t)
			test.prepare(t, root)
			if err := Synchronize(root, "Managed."); err == nil {
				t.Fatal("expected synchronization failure")
			}
			if _, err := os.Lstat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) && test.name != "AGENTS symlink" {
				t.Fatalf("AGENTS.md was created before preflight failure: %v", err)
			}
		})
	}
}

func TestSynchronizeSerializesConcurrentWriters(t *testing.T) {
	root := contextProject(t)
	values := []string{"alpha", "beta", "gamma", "delta"}
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for index := range 40 {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			errs <- Synchronize(root, value)
		}(values[index%len(values)])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got := readContext(t, filepath.Join(root, "AGENTS.md"))
	if strings.Count(got, beginMarker) != 1 || strings.Count(got, endMarker) != 1 {
		t.Fatalf("concurrent result has malformed markers: %q", got)
	}
	matched := false
	for _, value := range values {
		matched = matched || strings.Contains(got, "\n"+value+"\n"+endMarker)
	}
	if !matched {
		t.Fatalf("concurrent result does not contain one complete instruction value: %q", got)
	}
}

func contextProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".fledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func readContext(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
