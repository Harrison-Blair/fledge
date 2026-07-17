package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// makeHash returns the sha256 hex string of b.
func makeHash(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h)
}

// TestDriftReport is a table test over a temp tree seeding each status.
func TestDriftReport(t *testing.T) {
	// Shared content values.
	origContent := []byte("original content")
	newContent := []byte("new content from binary")
	editedContent := []byte("user has edited this file")

	origHash := makeHash(origContent)
	newHash := makeHash(newContent)

	tests := []struct {
		name string
		// setup writes files to the temp root.
		setup func(root string)
		// stamp entries (nil map means no stamp — use stamp=nil).
		stampFiles  map[string]StampEntry
		useNilStamp bool
		// expected entries.
		expected map[string]StampEntry
		// wantStatus maps path → expected DriftStatus.
		wantStatus map[string]DriftStatus
	}{
		{
			name: "up-to-date: disk matches expected bytes",
			setup: func(root string) {
				writeTestFile(t, root, "some/file.md", newContent)
			},
			stampFiles: map[string]StampEntry{
				"some/file.md": {Policy: "core", Sha256: origHash},
			},
			expected: map[string]StampEntry{
				"some/file.md": {Policy: "core", Sha256: newHash},
			},
			wantStatus: map[string]DriftStatus{
				"some/file.md": StatusUpToDate,
			},
		},
		{
			name: "stale: disk matches stamp hash but expected has moved",
			setup: func(root string) {
				writeTestFile(t, root, "some/file.md", origContent)
			},
			stampFiles: map[string]StampEntry{
				"some/file.md": {Policy: "default", Sha256: origHash},
			},
			expected: map[string]StampEntry{
				"some/file.md": {Policy: "default", Sha256: newHash},
			},
			wantStatus: map[string]DriftStatus{
				"some/file.md": StatusStale,
			},
		},
		{
			name: "modified: disk differs from both stamp and expected",
			setup: func(root string) {
				writeTestFile(t, root, "some/file.md", editedContent)
			},
			stampFiles: map[string]StampEntry{
				"some/file.md": {Policy: "default", Sha256: origHash},
			},
			expected: map[string]StampEntry{
				"some/file.md": {Policy: "default", Sha256: newHash},
			},
			wantStatus: map[string]DriftStatus{
				"some/file.md": StatusModified,
			},
		},
		{
			name:  "missing: file absent from disk",
			setup: func(root string) {},
			stampFiles: map[string]StampEntry{
				"some/file.md": {Policy: "core", Sha256: origHash},
			},
			expected: map[string]StampEntry{
				"some/file.md": {Policy: "core", Sha256: newHash},
			},
			wantStatus: map[string]DriftStatus{
				"some/file.md": StatusMissing,
			},
		},
		{
			name: "obsolete: in stamp but not in expected",
			setup: func(root string) {
				writeTestFile(t, root, "old/file.md", origContent)
			},
			stampFiles: map[string]StampEntry{
				"old/file.md": {Policy: "default", Sha256: origHash},
			},
			expected: map[string]StampEntry{},
			wantStatus: map[string]DriftStatus{
				"old/file.md": StatusObsolete,
			},
		},
		{
			name: "symlink up-to-date: readlink matches expected target",
			setup: func(root string) {
				dir := filepath.Join(root, "some")
				_ = os.MkdirAll(dir, 0o755)
				_ = os.Symlink("../../target/dir", filepath.Join(dir, "link"))
			},
			stampFiles: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../target/dir"},
			},
			expected: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../target/dir"},
			},
			wantStatus: map[string]DriftStatus{
				"some/link": StatusUpToDate,
			},
		},
		{
			name: "symlink stale: disk matches stamp target, expected has moved",
			setup: func(root string) {
				dir := filepath.Join(root, "some")
				_ = os.MkdirAll(dir, 0o755)
				_ = os.Symlink("../../old/dir", filepath.Join(dir, "link"))
			},
			stampFiles: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../old/dir"},
			},
			expected: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../new/dir"},
			},
			wantStatus: map[string]DriftStatus{
				"some/link": StatusStale,
			},
		},
		{
			name: "symlink modified: disk differs from both stamp and expected targets",
			setup: func(root string) {
				dir := filepath.Join(root, "some")
				_ = os.MkdirAll(dir, 0o755)
				_ = os.Symlink("../../user/edited/dir", filepath.Join(dir, "link"))
			},
			stampFiles: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../old/dir"},
			},
			expected: map[string]StampEntry{
				"some/link": {Policy: "symlink", Target: "../../new/dir"},
			},
			wantStatus: map[string]DriftStatus{
				"some/link": StatusModified,
			},
		},
		{
			name: "append up-to-date: all lines present",
			setup: func(root string) {
				writeTestFile(t, root, ".gitignore", []byte("# comment\n.fledge/nest/raw/\n.fledge/broods/\n"))
			},
			stampFiles: map[string]StampEntry{
				".gitignore": {Policy: "append", Lines: []string{".fledge/nest/raw/", ".fledge/broods/"}},
			},
			expected: map[string]StampEntry{
				".gitignore": {Policy: "append", Lines: []string{".fledge/nest/raw/", ".fledge/broods/"}},
			},
			wantStatus: map[string]DriftStatus{
				".gitignore": StatusUpToDate,
			},
		},
		{
			name: "append missing: a required line is absent",
			setup: func(root string) {
				writeTestFile(t, root, ".gitignore", []byte("# comment\n.fledge/nest/raw/\n"))
			},
			stampFiles: map[string]StampEntry{
				".gitignore": {Policy: "append", Lines: []string{".fledge/nest/raw/", ".fledge/broods/"}},
			},
			expected: map[string]StampEntry{
				".gitignore": {Policy: "append", Lines: []string{".fledge/nest/raw/", ".fledge/broods/"}},
			},
			wantStatus: map[string]DriftStatus{
				".gitignore": StatusMissing,
			},
		},
		{
			name:        "no-stamp: nil stamp sentinel — no obsolete entries, all disk-based",
			useNilStamp: true,
			setup: func(root string) {
				writeTestFile(t, root, "some/file.md", newContent)
			},
			stampFiles: nil,
			expected: map[string]StampEntry{
				"some/file.md": {Policy: "core", Sha256: newHash},
			},
			wantStatus: map[string]DriftStatus{
				"some/file.md": StatusUpToDate,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(root)

			var stamp *Stamp
			if !tc.useNilStamp {
				stamp = &Stamp{
					FledgeVersion: "0.2.0",
					Files:         tc.stampFiles,
				}
			}

			got := DriftReport(root, stamp, tc.expected)

			// Index results by path.
			byPath := map[string]Drift{}
			for _, d := range got {
				byPath[d.Path] = d
			}

			for path, want := range tc.wantStatus {
				d, ok := byPath[path]
				if !ok {
					t.Errorf("path %q: missing from DriftReport output", path)
					continue
				}
				if d.Status != want {
					t.Errorf("path %q: status = %q, want %q", path, d.Status, want)
				}
			}

			// No extra unexpected paths.
			for path := range byPath {
				if _, expected := tc.wantStatus[path]; !expected {
					t.Errorf("unexpected path %q in DriftReport output (status %q)", path, byPath[path].Status)
				}
			}
		})
	}
}

// TestDriftReportDevLink covers PLM-031/FTHR-080: a dev-linked repo must not
// report false drift for dev-linked paths, while genuine problems (a
// dangling target, a clobbered link, or an unrelated user-edited file) are
// still reported. Unlike TestDriftReport's table (which uses one shared
// stamp/expected pair with no DevSource), each case here sets stamp.DevSource
// explicitly since that is exactly the bit DriftReport must switch on.
func TestDriftReportDevLink(t *testing.T) {
	shippedContent := []byte("shipped embedded content")
	shippedHash := makeHash(shippedContent)

	t.Run("up-to-date: dev-linked file whose source differs from shipped bytes", func(t *testing.T) {
		root := t.TempDir()
		srcDir := t.TempDir()
		srcFile := filepath.Join(srcDir, "SKILL.md")
		if err := os.WriteFile(srcFile, []byte("live source content, deliberately unlike shipped"), 0o644); err != nil {
			t.Fatal(err)
		}
		linkPath := filepath.Join(root, ".fledge/skills/fledge-orchestrate/SKILL.md")
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(srcFile, linkPath); err != nil {
			t.Fatal(err)
		}

		stamp := &Stamp{
			FledgeVersion: "0.2.0",
			DevSource:     srcDir,
			Files: map[string]StampEntry{
				".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "dev-link", Target: filepath.ToSlash(srcFile)},
			},
		}
		// expected carries the manifest's content expectation — Target empty —
		// exactly as scaffoldDrift builds it via the non-dev ExpectedFiles.
		expected := map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "core", Sha256: shippedHash},
		}

		got := DriftReport(root, stamp, expected)
		if len(got) != 1 || got[0].Status != StatusUpToDate {
			t.Errorf("dev-linked file with differing source content: got %+v, want a single StatusUpToDate entry", got)
		}
	})

	t.Run("missing: dev-linked file with a dangling target", func(t *testing.T) {
		root := t.TempDir()
		srcDir := t.TempDir()
		srcFile := filepath.Join(srcDir, "gone.md") // never created
		linkPath := filepath.Join(root, ".fledge/skills/fledge-orchestrate/SKILL.md")
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(srcFile, linkPath); err != nil {
			t.Fatal(err)
		}

		stamp := &Stamp{
			FledgeVersion: "0.2.0",
			DevSource:     srcDir,
			Files: map[string]StampEntry{
				".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "dev-link", Target: filepath.ToSlash(srcFile)},
			},
		}
		expected := map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "core", Sha256: shippedHash},
		}

		got := DriftReport(root, stamp, expected)
		if len(got) != 1 || got[0].Status != StatusMissing {
			t.Errorf("dev-linked file with dangling target: got %+v, want a single StatusMissing entry", got)
		}
	})

	t.Run("modified: a regular file sitting where a dev link is expected", func(t *testing.T) {
		root := t.TempDir()
		srcDir := t.TempDir()
		linkPath := filepath.Join(root, ".fledge/skills/fledge-orchestrate/SKILL.md")
		if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, ".fledge/skills/fledge-orchestrate/SKILL.md", []byte("clobbered the link"))

		stamp := &Stamp{
			FledgeVersion: "0.2.0",
			DevSource:     srcDir,
			Files: map[string]StampEntry{
				".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "dev-link", Target: filepath.Join(srcDir, "SKILL.md")},
			},
		}
		expected := map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "core", Sha256: shippedHash},
		}

		got := DriftReport(root, stamp, expected)
		if len(got) != 1 || got[0].Status != StatusModified {
			t.Errorf("regular file clobbering a dev link: got %+v, want a single StatusModified entry", got)
		}
	})

	t.Run("non-dev repo: an unrelated genuinely user-edited file still reports modified", func(t *testing.T) {
		root := t.TempDir()
		editedContent := []byte("user has edited this file")
		writeTestFile(t, root, "some/file.md", editedContent)

		// No DevSource on the stamp — the non-dev path.
		stamp := &Stamp{
			FledgeVersion: "0.2.0",
			Files: map[string]StampEntry{
				"some/file.md": {Policy: "default", Sha256: makeHash([]byte("original content"))},
			},
		}
		expected := map[string]StampEntry{
			"some/file.md": {Policy: "default", Sha256: shippedHash},
		}

		got := DriftReport(root, stamp, expected)
		if len(got) != 1 || got[0].Status != StatusModified {
			t.Errorf("non-dev genuinely edited file: got %+v, want a single StatusModified entry", got)
		}
	})
}

// TestEditedOnRefreshOmitsDevLinks pins AC-6: a dev-linked repo's dev-linked
// paths never appear in EditedOnRefresh's output, even when their live
// source content differs from the shipped bytes — so refresh raises no
// "will overwrite user-edited file(s)" prompt for them.
func TestEditedOnRefreshOmitsDevLinks(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "SKILL.md")
	if err := os.WriteFile(srcFile, []byte("live source content, deliberately unlike shipped"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, ".fledge/skills/fledge-orchestrate/SKILL.md")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(srcFile, linkPath); err != nil {
		t.Fatal(err)
	}

	stamp := &Stamp{
		FledgeVersion: "0.2.0",
		DevSource:     srcDir,
		Files: map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "dev-link", Target: filepath.ToSlash(srcFile)},
		},
	}
	expected := map[string]StampEntry{
		".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "core", Sha256: makeHash([]byte("shipped embedded content"))},
	}

	got := EditedOnRefresh(root, stamp, expected)
	for _, p := range got {
		if p == ".fledge/skills/fledge-orchestrate/SKILL.md" {
			t.Errorf("EditedOnRefresh listed dev-linked path %q as user-edited, want it omitted", p)
		}
	}
}

// TestDriftReportNilStampNoObsolete verifies that a nil stamp produces no
// obsolete entries (the no-stamp sentinel is handled at the call site in preen).
func TestDriftReportNilStampNoObsolete(t *testing.T) {
	root := t.TempDir()
	expected := map[string]StampEntry{
		"a/file.md": {Policy: "core", Sha256: makeHash([]byte("hello"))},
	}
	got := DriftReport(root, nil, expected)
	for _, d := range got {
		if d.Status == StatusObsolete {
			t.Errorf("nil stamp produced an obsolete entry for %q", d.Path)
		}
	}
}

func writeTestFile(t *testing.T, root, rel string, content []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
}
