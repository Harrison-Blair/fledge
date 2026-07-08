package bootstrap

import (
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
// core prose is one of the 7 (no phantom capabilities).
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
	// the 7 must be exact — catches renames drifting between contract and prose.
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
		// shipped adapter (claude declares all 7) — trivially true unless the
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
// (content-compare); without refresh local edits are preserved; a refresh
// rewrites only the files whose bytes differ.
func TestWriteCoreClassification(t *testing.T) {
	root := t.TempDir()
	created, updated, skipped, err := WriteCore(root, true)
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
	created2, updated2, skipped2, err := WriteCore(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(created2) != 0 || len(updated2) != 0 || len(skipped2) != len(created) {
		t.Fatalf("second refresh run: created=%d updated=%d skipped=%d, want all skipped", len(created2), len(updated2), len(skipped2))
	}

	// Without refresh, a local edit is preserved and everything is skipped.
	if err := os.WriteFile(skill, []byte("local edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	created3, updated3, skipped3, err := WriteCore(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(created3) != 0 || len(updated3) != 0 || len(skipped3) != len(created) {
		t.Fatalf("no-refresh run: created=%d updated=%d skipped=%d, want all skipped", len(created3), len(updated3), len(skipped3))
	}
	if data, _ := os.ReadFile(skill); string(data) != "local edit" {
		t.Fatalf("no-refresh run clobbered a local edit: %q", data)
	}

	// Refresh rewrites exactly the edited file, restoring embedded bytes.
	created4, updated4, skipped4, err := WriteCore(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(created4) != 0 || len(updated4) != 1 || len(skipped4) != len(created)-1 {
		t.Fatalf("refresh after edit: created=%d updated=%d skipped=%d, want exactly 1 updated", len(created4), len(updated4), len(skipped4))
	}
	if data, _ := os.ReadFile(skill); string(data) == "local edit" {
		t.Fatal("refresh did not restore the edited file")
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
	if _, _, _, err := WriteCore(root, false); err != nil {
		t.Fatal(err)
	}
	created, _, _, err := m.WriteAdapter(root, nil, false)
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
	_, updated2, skipped2, err := m.WriteAdapter(root, nil, false)
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

// TestWriteAdapterRefresh: default-policy files (user may customize) are
// preserved on plain runs and synced to the embedded bytes on refresh;
// byte-identical files are skipped so a second refresh updates nothing.
func TestWriteAdapterRefresh(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	root := t.TempDir()
	if _, _, _, err := m.WriteAdapter(root, nil, false); err != nil {
		t.Fatal(err)
	}

	brooder := filepath.Join(root, ".claude", "agents", "fledge-brooder.md") // default policy
	teamLoop := filepath.Join(root, ".claude", "team-loop.md")               // overwrite policy
	for _, p := range []string{brooder, teamLoop} {
		if err := os.WriteFile(p, []byte("local edit"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Plain run: default-policy edit preserved, overwrite-policy file repaired.
	_, updated, _, err := m.WriteAdapter(root, nil, false)
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

	// Refresh: default-policy file synced to embedded bytes.
	_, updated2, _, err := m.WriteAdapter(root, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(updated2, ".claude/agents/fledge-brooder.md") {
		t.Errorf("refresh did not update the edited agent file (updated=%v)", updated2)
	}
	if data, _ := os.ReadFile(brooder); string(data) == "local edit" {
		t.Fatal("refresh did not restore the edited agent file")
	}

	// Second refresh over identical files updates nothing.
	_, updated3, _, err := m.WriteAdapter(root, nil, true)
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

// TestClaudeAllowListGenerated: the generated Claude settings.local.json embeds
// a Bash(fledge …) allow entry per CLI command fed via commandOrder (Q23).
func TestClaudeAllowListGenerated(t *testing.T) {
	m, err := FindAdapter("claude")
	if err != nil || m == nil {
		t.Fatalf("claude adapter: %v", err)
	}
	root := t.TempDir()
	commands := []string{"init", "preen", "brood"}
	if _, _, _, err := m.WriteAdapter(root, commands, false); err != nil {
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
