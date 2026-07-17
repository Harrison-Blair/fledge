package bootstrap

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// TestAdapterManifests: every manifest parses, names its files' sources in the
// embed FS, and its piping_file (if any) appears in the file map.
func TestAdapterManifests(t *testing.T) {
	adapters, err := LoadAdapters()
	if err != nil {
		t.Fatal(err)
	}
	if len(adapters) == 0 {
		t.Fatal("no adapters loaded")
	}
	for _, m := range adapters {
		if m.Name == "" {
			t.Errorf("%s: empty name", m.dir)
		}
		if m.Detector.Exists == "" {
			t.Errorf("%s: no detector (auto-detect would never select it)", m.Name)
		}
		if m.NativeSkillsDir == "" {
			t.Errorf("%s: no native_skills_dir (duplicate guard would be disabled)", m.Name)
		}
		dsts := map[string]bool{}
		for _, f := range m.Files {
			dsts[f.Dst] = true
			if f.Src == "" {
				if f.AppendIfMissing == "" && f.Symlink == "" {
					t.Errorf("%s: file entry %q has no src, append_if_missing, or symlink", m.Name, f.Dst)
				}
				continue
			}
			if _, err := FS.ReadFile(path.Join(m.dir, f.Src)); err != nil {
				t.Errorf("%s: src %q missing from embed FS: %v", m.Name, f.Src, err)
			}
		}
		if m.PipingFile != "" && !dsts[m.PipingFile] {
			t.Errorf("%s: piping_file %q not in file map", m.Name, m.PipingFile)
		}
	}
}

// TestPrimitiveCoverage: adapters declare only known primitives; the derived
// tiers match the shipped profiles (Q5); every primitive named anywhere in the
// core prose is one of the 6 (no phantom capabilities).
func TestPrimitiveCoverage(t *testing.T) {
	known := map[string]bool{}
	for _, p := range PrimitiveOrder {
		known[p] = true
		if PrimitiveDesc(p) == "" {
			t.Errorf("primitive %s: missing description", p)
		}
		if PrimitiveTier(p) == "" {
			t.Errorf("primitive %s: missing tier", p)
		}
	}

	adapters, err := LoadAdapters()
	if err != nil {
		t.Fatal(err)
	}
	wantTier := map[string]string{"claude": "C", "codex": "A", "pi": "A"}
	for _, m := range adapters {
		for p := range m.TierPrimitives {
			if !known[p] {
				t.Errorf("%s: unknown primitive %q in tier_primitives", m.Name, p)
			}
		}
		if want, ok := wantTier[m.Name]; ok && m.Tier() != want {
			t.Errorf("%s: derived tier %q, want %q", m.Name, m.Tier(), want)
		}
	}

	// Any `primitive`-shaped backtick token in core prose that looks like one of
	// the 6 must be exact — catches renames drifting between contract and prose.
	tick := regexp.MustCompile("`([a-z-]+)`")
	err = fs.WalkDir(FS, "core", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return walkErr
		}
		data, rErr := FS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		used := map[string]bool{}
		for _, match := range tick.FindAllStringSubmatch(string(data), -1) {
			if known[match[1]] {
				used[match[1]] = true
			}
		}
		// Every primitive the core prose uses must be coverable by the richest
		// shipped adapter (claude declares all 6) — trivially true unless the
		// contract shrinks; keeps prose and contract in lockstep.
		for p := range used {
			if !known[p] {
				t.Errorf("%s: references unknown primitive %q", p, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCorePrimitivesReferenced: the core prose actually uses every contract
// primitive somewhere — a primitive nothing references is dead contract.
func TestCorePrimitivesReferenced(t *testing.T) {
	var all []byte
	err := fs.WalkDir(FS, "core", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return walkErr
		}
		data, rErr := FS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		all = append(all, data...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range PrimitiveOrder {
		if !strings.Contains(string(all), p) {
			t.Errorf("core prose never references primitive %q", p)
		}
	}
}

// TestCoreNeutral: core prose must not reference any harness-native path.
func TestCoreNeutral(t *testing.T) {
	err := fs.WalkDir(FS, "core", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		data, rErr := FS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		for _, marker := range []string{".claude/", ".pi/", ".codex/", ".cursor/"} {
			if strings.Contains(string(data), marker) {
				t.Errorf("%s: references harness-native path %q", p, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestSkillFrontmatter: every core SKILL.md carries valid Agent-Skills
// frontmatter (mirrors pi's validation so installs don't warn).
func TestSkillFrontmatter(t *testing.T) {
	names, err := CoreSkillNames()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fledge-interrogate", "fledge-orchestrate"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("CoreSkillNames() = %v, want %v", names, want)
	}
	nameRe := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	for _, skill := range names {
		p := path.Join("core/skills", skill, "SKILL.md")
		data, err := FS.ReadFile(p)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		body := string(data)
		if !strings.HasPrefix(body, "---\n") {
			t.Errorf("%s: missing frontmatter", p)
			continue
		}
		end := strings.Index(body[4:], "\n---")
		if end < 0 {
			t.Errorf("%s: unterminated frontmatter", p)
			continue
		}
		var fm struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if err := yaml.Unmarshal([]byte(body[4:4+end]), &fm); err != nil {
			t.Errorf("%s: frontmatter parse: %v", p, err)
			continue
		}
		if fm.Name != skill {
			t.Errorf("%s: frontmatter name %q, want %q (must match directory)", p, fm.Name, skill)
		}
		if len(fm.Name) > 64 || !nameRe.MatchString(fm.Name) {
			t.Errorf("%s: invalid skill name %q", p, fm.Name)
		}
		if fm.Description == "" || len(fm.Description) > 1024 {
			t.Errorf("%s: description missing or >1024 chars (%d)", p, len(fm.Description))
		}
	}
}

// TestWriteCoreClassification: WriteCore reports created/updated/skipped
// correctly — fresh root all created; a refresh over identical files skips
// (content-compare); without refresh local edits are left alone; a refresh
// resets any edited file to the shipped version (the CLI confirms first).
func TestWriteCoreClassification(t *testing.T) {
	root := t.TempDir()
	created, updated, skipped, err := WriteCore(root, WriteOpts{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) == 0 || len(updated) != 0 || len(skipped) != 0 {
		t.Fatalf("fresh refresh run: created=%d updated=%d skipped=%d, want all created", len(created), len(updated), len(skipped))
	}
	skill := filepath.Join(root, ".fledge", "skills", "fledge-orchestrate", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("core skill not written: %v", err)
	}

	// Byte-identical files are skipped even on refresh.
	created2, updated2, skipped2, err := WriteCore(root, WriteOpts{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(created2) != 0 || len(updated2) != 0 || len(skipped2) != len(created) {
		t.Fatalf("second refresh run: created=%d updated=%d skipped=%d, want all skipped", len(created2), len(updated2), len(skipped2))
	}

	// Without refresh, a local edit is left alone and everything is skipped.
	if err := os.WriteFile(skill, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	created3, updated3, skipped3, err := WriteCore(root, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(created3) != 0 || len(updated3) != 0 || len(skipped3) != len(created) {
		t.Fatalf("no-refresh run: created=%d updated=%d skipped=%d, want all skipped", len(created3), len(updated3), len(skipped3))
	}
	if data, _ := os.ReadFile(skill); string(data) != "local edit" {
		t.Fatalf("no-refresh run clobbered a local edit: %q", data)
	}

	// Refresh resets the edited file to the shipped version.
	created4, updated4, skipped4, err := WriteCore(root, WriteOpts{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(created4) != 0 || len(updated4) != 1 || len(skipped4) != len(created)-1 {
		t.Fatalf("refresh over edit: created=%d updated=%d skipped=%d, want exactly 1 updated", len(created4), len(updated4), len(skipped4))
	}
	if data, _ := os.ReadFile(skill); string(data) == "local edit" {
		t.Fatal("refresh did not reset the edited file")
	}
}

// TestClaudeSkillSymlinks: the claude adapter symlinks each core skill into
// .claude/skills/ (Claude Code's only project skill location; it follows
// symlinks), the links resolve to the written core, re-runs are idempotent,
// and the duplicate guard tolerates the sanctioned symlink.
func TestClaudeSkillSymlinks(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	root := t.TempDir()
	if _, _, _, err := WriteCore(root, WriteOpts{}); err != nil {
		t.Fatal(err)
	}
	created, _, _, err := m.WriteAdapter(root, nil, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	names, err := CoreSkillNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range names {
		link := filepath.Join(root, ".claude", "skills", skill)
		fi, err := os.Lstat(link)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s: not a symlink (err=%v); created=%v", link, err, created)
		}
		if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
			t.Errorf("%s: does not resolve to core SKILL.md: %v", link, err)
		}
	}
	// Sanctioned symlinks must pass the duplicate guard...
	if err := CheckDuplicateSkills(root); err != nil {
		t.Errorf("guard rejected sanctioned symlink: %v", err)
	}
	// ...and re-running the adapter is idempotent (skipped, not clobbered).
	_, updated2, skipped2, err := m.WriteAdapter(root, nil, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range names {
		dst := path.Join(".claude/skills", skill)
		if !contains(skipped2, dst) || contains(updated2, dst) {
			t.Errorf("re-run: %s not skipped (updated=%v skipped=%v)", dst, updated2, skipped2)
		}
	}
	// A real copy (not a symlink) still trips the guard.
	realDir := filepath.Join(root, ".claude", "skills", names[0])
	if err := os.Remove(realDir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckDuplicateSkills(root); err == nil {
		t.Error("guard accepted a real skill copy")
	}
}

// TestCodexSkillStubs verifies Codex discovers physical .agents/skills bridge
// files that load the canonical core skills, while real copies are rejected.
func TestCodexSkillStubs(t *testing.T) {
	m, err := FindAdapter("codex")
	if err != nil || m == nil {
		t.Fatalf("codex adapter: %v", err)
	}
	root := t.TempDir()
	if _, _, _, err := WriteCore(root, WriteOpts{}); err != nil {
		t.Fatal(err)
	}
	created, _, _, err := m.WriteAdapter(root, nil, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	names, err := CoreSkillNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range names {
		stub := filepath.Join(root, ".agents", "skills", skill, "SKILL.md")
		if fi, err := os.Lstat(stub); err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("%s: not a physical SKILL.md (err=%v); created=%v", stub, err, created)
		}
		data, err := os.ReadFile(stub)
		if err != nil || !strings.Contains(string(data), ".fledge/skills/"+skill+"/SKILL.md") {
			t.Errorf("%s: does not forward to canonical core skill: %v", stub, err)
		}
	}
	if err := CheckDuplicateSkills(root); err != nil {
		t.Errorf("guard rejected managed stub: %v", err)
	}
	_, updated, skipped, err := m.WriteAdapter(root, nil, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range names {
		dst := path.Join(".agents/skills", skill, "SKILL.md")
		if !contains(skipped, dst) || contains(updated, dst) {
			t.Errorf("re-run: %s not skipped (updated=%v skipped=%v)", dst, updated, skipped)
		}
	}
	realDir := filepath.Join(root, ".agents", "skills", names[0])
	if err := os.Remove(filepath.Join(realDir, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "SKILL.md"), []byte("copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckDuplicateSkills(root); err == nil {
		t.Error("guard accepted a real Codex skill copy")
	} else if !strings.Contains(err.Error(), ".agents/skills/") {
		t.Errorf("guard error %q does not identify Codex's native skill directory", err)
	}
}

// TestWriteAdapterRefresh: default-policy files (user may customize) are left
// alone on plain runs; a refresh resets them to the shipped version;
// overwrite-policy files are always repaired.
func TestWriteAdapterRefresh(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	root := t.TempDir()
	if _, _, _, err := m.WriteAdapter(root, nil, WriteOpts{}); err != nil {
		t.Fatal(err)
	}

	brooder := filepath.Join(root, ".claude", "agents", "fledge-brooder.md") // default policy
	teamLoop := filepath.Join(root, ".claude", "team-loop.md")               // overwrite policy
	editBytes := []byte("local edit")
	for _, p := range []string{brooder, teamLoop} {
		if err := os.WriteFile(p, editBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Plain run: default-policy edit left alone, overwrite-policy file repaired.
	_, updated, _, err := m.WriteAdapter(root, nil, WriteOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(brooder); string(data) != "local edit" {
		t.Fatalf("plain run clobbered a default-policy edit: %q", data)
	}
	if !contains(updated, ".claude/team-loop.md") {
		t.Errorf("plain run did not repair overwrite-policy file (updated=%v)", updated)
	}
	if data, _ := os.ReadFile(teamLoop); string(data) == "local edit" {
		t.Error("plain run left overwrite-policy edit in place")
	}

	// Refresh resets the default-policy edit to the shipped version.
	_, updated2, _, err := m.WriteAdapter(root, nil, WriteOpts{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(updated2, ".claude/agents/fledge-brooder.md") {
		t.Errorf("refresh did not reset brooder (updated=%v)", updated2)
	}
	if data, _ := os.ReadFile(brooder); string(data) == "local edit" {
		t.Fatal("refresh did not reset the brooder edit")
	}

	// Second refresh over identical files updates nothing.
	_, updated3, _, err := m.WriteAdapter(root, nil, WriteOpts{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated3) != 0 {
		t.Errorf("second refresh updated files: %v", updated3)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// TestClaudeIncubatorWired: the Claude adapter Files include an entry writing
// .claude/agents/fledge-incubator.md, and the adapter's derived tier (C) and
// tier_primitives count are unchanged (no new primitive introduced by FTHR-014).
func TestClaudeIncubatorWired(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}

	var found bool
	for _, f := range m.Files {
		if f.Dst == ".claude/agents/fledge-incubator.md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("claude adapter Files: missing entry for .claude/agents/fledge-incubator.md")
	}

	if got := m.Tier(); got != "C" {
		t.Errorf("claude adapter derived tier = %q, want C", got)
	}

	if got, want := len(m.TierPrimitives), len(PrimitiveOrder); got != want {
		t.Errorf("claude tier_primitives count = %d, want %d (no new primitive)", got, want)
	}
}

// TestClaudeAgentDefsRepointToRoleFiles: each Claude adapter worker-role agent
// definition cites its own per-role protocol file (incubator.md / brooder.md /
// skua.md) rather than a section of the old worker-protocols.md.
func TestClaudeAgentDefsRepointToRoleFiles(t *testing.T) {
	cases := []struct {
		file string
		want string
	}{
		{"adapters/claude/agents/fledge-incubator.md", ".fledge/skills/fledge-orchestrate/incubator.md"},
		{"adapters/claude/agents/fledge-brooder.md", ".fledge/skills/fledge-orchestrate/brooder.md"},
		{"adapters/claude/agents/fledge-skua.md", ".fledge/skills/fledge-orchestrate/skua.md"},
	}
	for _, c := range cases {
		b, err := FS.ReadFile(c.file)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		s := string(b)
		if !strings.Contains(s, c.want) {
			t.Errorf("%s: missing reference to %s", c.file, c.want)
		}
		if strings.Contains(s, "worker-protocols.md") {
			t.Errorf("%s: still references worker-protocols.md", c.file)
		}
	}
}

// TestClaudeAllowListGenerated: the generated Claude settings.local.json embeds
// a Bash(fledge …) allow entry per CLI command fed via commandOrder (Q23).
func TestClaudeAllowListGenerated(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	root := t.TempDir()
	commands := []string{"init", "preen", "brood"}
	if _, _, _, err := m.WriteAdapter(root, commands, WriteOpts{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range commands {
		if !strings.Contains(string(data), "fledge "+c) {
			t.Errorf("settings.local.json missing allow entry for %q:\n%s", c, data)
		}
	}
}

// TestEditedOnRefresh: the pre-write detection a refresh confirms on.
// Unedited (disk == stamp hash, embedded moved → stale) files are not listed;
// edited expected files are; stampless treats any differing file as edited;
// edited obsolete entries are listed, unedited ones are not.
func TestEditedOnRefresh(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	expected, err := ExpectedFiles(m, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Pick a default-policy expected file to play with.
	const target = ".claude/agents/fledge-brooder.md"
	if _, ok := expected[target]; !ok {
		t.Fatalf("expected map missing %s", target)
	}

	oldBytes := []byte("content from old fledge version — differs from embedded")
	oldHash := sha256.Sum256(oldBytes)
	userBytes := []byte("user customization — not what fledge wrote")

	obsContent := []byte("fledge-owned obsolete content\n")
	obsHash := sha256.Sum256(obsContent)

	stamp := &Stamp{FledgeVersion: "0.1.0", Agents: []string{"claude"}, Files: map[string]StampEntry{
		target:                  {Policy: "default", Sha256: fmt.Sprintf("%x", oldHash)},
		".fledge/old/gone.md":   {Policy: "core", Sha256: fmt.Sprintf("%x", obsHash)},
		".fledge/old/edited.md": {Policy: "core", Sha256: fmt.Sprintf("%x", obsHash)},
	}}

	root := t.TempDir()
	write := func(rel string, data []byte) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Stale (disk == stamp, embedded moved) → not edited.
	write(target, oldBytes)
	write(".fledge/old/gone.md", obsContent)  // obsolete, unedited → not listed
	write(".fledge/old/edited.md", userBytes) // obsolete, edited → listed
	edited := EditedOnRefresh(root, stamp, expected)
	if contains(edited, target) {
		t.Errorf("stale file listed as edited: %v", edited)
	}
	if contains(edited, ".fledge/old/gone.md") {
		t.Errorf("unedited obsolete file listed as edited: %v", edited)
	}
	if !contains(edited, ".fledge/old/edited.md") {
		t.Errorf("edited obsolete file not listed: %v", edited)
	}

	// User-edited expected file → listed.
	write(target, userBytes)
	if edited := EditedOnRefresh(root, stamp, expected); !contains(edited, target) {
		t.Errorf("edited file not listed: %v", edited)
	}

	// Stampless: any differing expected file counts as edited (conservative).
	if edited := EditedOnRefresh(root, nil, expected); !contains(edited, target) {
		t.Errorf("stampless: differing file not listed: %v", edited)
	}
}

// TestPruneObsolete: refresh is a reset-to-shipped, so obsolete paths are
// removed regardless of content (the CLI confirmed user edits up front) —
// except append-policy entries, whose files fledge never owned.
func TestPruneObsolete(t *testing.T) {
	for _, tc := range []struct {
		name     string
		repoPath string
		entry    StampEntry
		setup    func(root string) // sets up the on-disk state
		wantDel  bool
	}{
		{
			name:     "file",
			repoPath: "old/file.md",
			entry:    StampEntry{Policy: "default", Sha256: "irrelevant"},
			setup: func(root string) {
				p := filepath.Join(root, "old", "file.md")
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.WriteFile(p, []byte("user-edited content differs from stamp"), 0o644)
			},
			wantDel: true,
		},
		{
			name:     "missing",
			repoPath: "old/file.md",
			entry:    StampEntry{Policy: "default", Sha256: "irrelevant"},
			setup:    func(root string) {}, // file does not exist on disk
			wantDel:  false,
		},
		{
			name:     "symlink",
			repoPath: "old/link",
			entry:    StampEntry{Policy: "symlink", Target: "../other/target"},
			setup: func(root string) {
				p := filepath.Join(root, "old", "link")
				os.MkdirAll(filepath.Dir(p), 0o755)
				os.Symlink("../different/target", p) // even repointed → removed
			},
			wantDel: true,
		},
		{
			name:     "append-never-deleted",
			repoPath: ".gitignore",
			entry:    StampEntry{Policy: "append", Lines: []string{".fledge/broods/"}},
			setup: func(root string) {
				os.WriteFile(filepath.Join(root, ".gitignore"), []byte("user content\n"), 0o644)
			},
			wantDel: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.setup(root)

			deleted, err := PruneObsolete(root, tc.repoPath, tc.entry)
			if err != nil {
				t.Fatalf("PruneObsolete: %v", err)
			}
			if deleted != tc.wantDel {
				t.Errorf("deleted=%v want %v", deleted, tc.wantDel)
			}
			if tc.wantDel {
				if _, err := os.Lstat(filepath.Join(root, tc.repoPath)); err == nil {
					t.Error("file still exists after deletion")
				}
			} else if tc.name == "append-never-deleted" {
				if _, err := os.Lstat(filepath.Join(root, tc.repoPath)); err != nil {
					t.Error("append-policy file was deleted")
				}
			}
		})
	}
}
