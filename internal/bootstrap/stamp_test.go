package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStampRoundTrip: Write then LoadStamp reproduces the struct.
func TestStampRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := &Stamp{
		FledgeVersion: "0.2.1",
		Agents:        []string{"claude", "pi"},
		Files: map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {
				Policy: "core",
				Sha256: "abc123",
			},
			".claude/skills/fledge-orchestrate": {
				Policy: "symlink",
				Target: "../../.fledge/skills/fledge-orchestrate",
			},
			".gitignore": {
				Policy: "append",
				Lines:  []string{".fledge/nest/raw/", ".fledge/broods/"},
			},
		},
	}

	wrote, err := want.Write(root)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("Write reported not written on fresh root")
	}

	got, err := LoadStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("LoadStamp returned nil after Write")
	}
	if got.FledgeVersion != want.FledgeVersion {
		t.Errorf("FledgeVersion: got %q want %q", got.FledgeVersion, want.FledgeVersion)
	}
	if strings.Join(got.Agents, ",") != strings.Join(want.Agents, ",") {
		t.Errorf("Agents: got %v want %v", got.Agents, want.Agents)
	}
	for path, wEntry := range want.Files {
		gEntry, ok := got.Files[path]
		if !ok {
			t.Errorf("Files[%q]: missing after round-trip", path)
			continue
		}
		if gEntry.Policy != wEntry.Policy {
			t.Errorf("Files[%q].Policy: got %q want %q", path, gEntry.Policy, wEntry.Policy)
		}
		if gEntry.Sha256 != wEntry.Sha256 {
			t.Errorf("Files[%q].Sha256: got %q want %q", path, gEntry.Sha256, wEntry.Sha256)
		}
		if gEntry.Target != wEntry.Target {
			t.Errorf("Files[%q].Target: got %q want %q", path, gEntry.Target, wEntry.Target)
		}
		if strings.Join(gEntry.Lines, ",") != strings.Join(wEntry.Lines, ",") {
			t.Errorf("Files[%q].Lines: got %v want %v", path, gEntry.Lines, wEntry.Lines)
		}
	}
	if len(got.Files) != len(want.Files) {
		t.Errorf("Files count: got %d want %d", len(got.Files), len(want.Files))
	}
}

// TestStampAbsent: LoadStamp returns (nil, nil) when the file does not exist.
func TestStampAbsent(t *testing.T) {
	root := t.TempDir()
	got, err := LoadStamp(root)
	if err != nil {
		t.Fatalf("LoadStamp on absent file: unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadStamp on absent file: got %+v, want nil", got)
	}
}

// TestStampDeterministic: marshaling the same Stamp twice yields identical
// bytes, and the output ends with a trailing newline.
func TestStampDeterministic(t *testing.T) {
	s := &Stamp{
		FledgeVersion: "0.2.1",
		Agents:        []string{"claude"},
		Files: map[string]StampEntry{
			".fledge/skills/fledge-orchestrate/SKILL.md": {Policy: "core", Sha256: "deadbeef"},
			".claude/fledge-adapter.md":                  {Policy: "primitive_map", Sha256: "cafebabe"},
		},
	}
	b1, err := marshalStamp(s)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := marshalStamp(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("two marshals differ:\n%s\n---\n%s", b1, b2)
	}
	if len(b1) == 0 || b1[len(b1)-1] != '\n' {
		t.Errorf("marshaled stamp does not end with a trailing newline: %q", b1)
	}
}

// TestExpectedFilesCoversAllPolicies: for the claude manifest + core tree,
// every entry has the right shape (hash vs target vs lines), the stamp file
// itself is absent, and all expected policy kinds appear.
func TestExpectedFilesCoversAllPolicies(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	files, err := ExpectedFiles(m, nil)
	if err != nil {
		t.Fatal(err)
	}

	// stamp itself must not appear
	if _, ok := files[stampPath]; ok {
		t.Errorf(".fledge/scaffold.json present in ExpectedFiles output (must be excluded)")
	}

	gotPolicies := map[string]bool{}

	// every entry must have the right shape for its policy
	for p, e := range files {
		gotPolicies[e.Policy] = true
		switch e.Policy {
		case "core", "default", "generate", "primitive_map", "overwrite":
			if e.Sha256 == "" {
				t.Errorf("%s: policy=%q but no sha256", p, e.Policy)
			}
			if e.Target != "" || e.Lines != nil {
				t.Errorf("%s: policy=%q has unexpected target/lines", p, e.Policy)
			}
		case "symlink":
			if e.Target == "" {
				t.Errorf("%s: policy=symlink but no target", p)
			}
			if e.Sha256 != "" || e.Lines != nil {
				t.Errorf("%s: policy=symlink has unexpected sha256/lines", p)
			}
		case "append":
			if len(e.Lines) == 0 {
				t.Errorf("%s: policy=append but no lines", p)
			}
			if e.Sha256 != "" || e.Target != "" {
				t.Errorf("%s: policy=append has unexpected sha256/target", p)
			}
		default:
			t.Errorf("%s: unknown policy %q", p, e.Policy)
		}
	}

	// verify all policies that the claude adapter + core tree should produce
	for _, want := range []string{"core", "default", "overwrite", "generate", "primitive_map", "symlink"} {
		if !gotPolicies[want] {
			t.Errorf("no entry with policy=%q in ExpectedFiles output", want)
		}
	}
}

// TestRenderEntryMatchesWritePath: bytes from renderEntry equal the bytes
// writeFileEntry writes for generate/primitive_map/overwrite/default files.
func TestRenderEntryMatchesWritePath(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	ctx := m.renderContext(nil)

	for _, f := range m.Files {
		f := f // capture
		// only content-bearing files (not symlink or append_if_missing)
		if f.Symlink != "" || f.AppendIfMissing != "" {
			continue
		}
		t.Run(f.Dst, func(t *testing.T) {
			got, err := renderEntry(m, f, ctx)
			if err != nil {
				t.Fatalf("renderEntry: %v", err)
			}

			// Write via writeFileEntry to a fresh dir, then read back.
			fileRoot := t.TempDir()
			if err := os.MkdirAll(filepath.Join(fileRoot, filepath.Dir(filepath.FromSlash(f.Dst))), 0o755); err != nil {
				t.Fatal(err)
			}
			_, _, _, err = m.writeFileEntry(fileRoot, f, ctx, false)
			if err != nil {
				t.Fatalf("writeFileEntry: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(fileRoot, filepath.FromSlash(f.Dst)))
			if err != nil {
				t.Fatalf("read written file: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("renderEntry bytes (%d) differ from writeFileEntry output (%d) for %s",
					len(got), len(want), f.Dst)
			}
		})
	}
}
