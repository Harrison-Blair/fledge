package bootstrap

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// WriteOpts controls the behavior of WriteCore and WriteAdapter. Refresh
// resets every fledge-owned file to the shipped version (the CLI confirms
// with the user before calling when user-edited files would be overwritten —
// see EditedOnRefresh); without it, existing files are skipped. DevSource, when
// non-empty, is the absolute path to a validated fledge source checkout
// (ValidateDevSource); every copy-type file (core skill docs, default-policy
// adapter files) is written as a symlink into DevSource instead of a copy —
// PLM-031 dev install mode. Files that are generated, merged, or already
// symlinked are unaffected.
// SelfLink marks a bare `--dev` (PLM-031 FC-3): DevSource is the same repo
// being scaffolded, so dev-link targets are written as relative symlinks
// (computed with filepath.Rel) instead of DevSource-absolute ones, and
// survive the tree being moved or cloned.
type WriteOpts struct {
	Refresh   bool
	DevSource string
	SelfLink  bool
}

// Manifest is an adapter's single source of truth: detector, the 6-row
// primitive coverage (which derives the tier), the file map (source → target),
// and the optional harness piping file. Stays in the binary; never clutters the
// target repo. Adding a new harness = adding a manifest.yaml, zero Go code.
type Manifest struct {
	Name           string            `yaml:"name"`
	Detector       ManifestDetector  `yaml:"detector"`
	TierPrimitives map[string]string `yaml:"tier_primitives"`
	Files          []ManifestFile    `yaml:"files"`
	PipingFile     string            `yaml:"piping_file"`

	// dir is the adapter's directory in the embed FS (e.g. "adapters/claude").
	dir string
}

// ManifestDetector tells init how to auto-sense a harness in a repo root.
type ManifestDetector struct {
	Exists string `yaml:"exists"`
}

// ManifestFile maps one source file (relative to the adapter dir) to a target
// path (relative to the repo root) with a write policy:
//   - generate: true          → src is a text/template; rendered, rewritten when content differs
//   - primitive_map: true     → the generated file inlines the primitive map (implies generate)
//   - overwrite: true         → copied verbatim; rewritten when content differs (fledge-managed entry files)
//   - append_if_missing: line → no src; ensures the line is present in dst (additive, never clobbers)
//   - symlink: target         → no src; dst is a symlink to target (relative to dst's parent)
//   - (none)                  → copied verbatim; skip-if-exists (user may customize); synced by `init --refresh`
type ManifestFile struct {
	Src             string `yaml:"src"`
	Dst             string `yaml:"dst"`
	Generate        bool   `yaml:"generate"`
	PrimitiveMap    bool   `yaml:"primitive_map"`
	Overwrite       bool   `yaml:"overwrite"`
	AppendIfMissing string `yaml:"append_if_missing"`
	Symlink         string `yaml:"symlink"`
}

// primitiveRow is one row of the generated primitive map.
type primitiveRow struct {
	Name      string
	Desc      string
	Mechanism string
	Provided  bool
	Tier      string
}

// renderContext is the template data for generated adapter files.
type renderContext struct {
	Adapter      string
	Tier         string
	Rows         []primitiveRow
	Provided     []string
	NotProvided  []string
	PipingFile   string
	CommandOrder []string
}

// LoadAdapters reads every adapter manifest from the embedded adapters/ tree.
// Adapter directories whose name starts with "_" are skipped (shared assets).
func LoadAdapters() ([]*Manifest, error) {
	var dirs []string
	err := fs.WalkDir(FS, "adapters", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		if p == "adapters" {
			return nil
		}
		if path.Dir(p) != "adapters" {
			// prune deeper directories
			return filepath.SkipDir
		}
		if strings.HasPrefix(d.Name(), "_") {
			return filepath.SkipDir
		}
		dirs = append(dirs, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk adapters: %w", err)
	}
	var out []*Manifest
	for _, dir := range dirs {
		m, err := loadManifest(dir)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func loadManifest(dir string) (*Manifest, error) {
	data, err := FS.ReadFile(path.Join(dir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read %s/manifest.yaml: %w", dir, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s/manifest.yaml: %w", dir, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf("%s/manifest.yaml: missing name", dir)
	}
	if m.TierPrimitives == nil {
		m.TierPrimitives = map[string]string{}
	}
	m.dir = dir
	return &m, nil
}

// FindAdapter returns the named adapter manifest or nil.
func FindAdapter(name string) (*Manifest, error) {
	adapters, err := LoadAdapters()
	if err != nil {
		return nil, err
	}
	for _, m := range adapters {
		if m.Name == name {
			return m, nil
		}
	}
	return nil, nil
}

// Provides reports whether the adapter provides primitive p.
func (m *Manifest) Provides(p string) bool {
	_, ok := m.TierPrimitives[p]
	return ok
}

// providedSet returns the provided primitives as a set.
func (m *Manifest) providedSet() map[string]bool {
	set := make(map[string]bool, len(m.TierPrimitives))
	for k := range m.TierPrimitives {
		set[k] = true
	}
	return set
}

// Tier derives the tier from primitive coverage (Q5); "" if below Tier A.
func (m *Manifest) Tier() string {
	return DeriveTier(m.providedSet())
}

// renderContext builds the template data for this adapter.
func (m *Manifest) renderContext(commandOrder []string) renderContext {
	provided := m.providedSet()
	var rows []primitiveRow
	for _, p := range PrimitiveOrder {
		mech, ok := m.TierPrimitives[p]
		rows = append(rows, primitiveRow{
			Name:      p,
			Desc:      PrimitiveDesc(p),
			Mechanism: mech,
			Provided:  ok,
			Tier:      PrimitiveTier(p),
		})
	}
	var prov, notProv []string
	for _, p := range PrimitiveOrder {
		if provided[p] {
			prov = append(prov, p)
		} else {
			notProv = append(notProv, p)
		}
	}
	return renderContext{
		Adapter:      m.Name,
		Tier:         m.Tier(),
		Rows:         rows,
		Provided:     prov,
		NotProvided:  notProv,
		PipingFile:   m.PipingFile,
		CommandOrder: commandOrder,
	}
}

// DetectAdapters returns the adapters whose detector marker exists in root
// (auto-detect for `fledge init` with no --agent).
func DetectAdapters(root string) ([]*Manifest, error) {
	adapters, err := LoadAdapters()
	if err != nil {
		return nil, err
	}
	var out []*Manifest
	for _, m := range adapters {
		if m.Detector.Exists == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, m.Detector.Exists)); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// Scaffolded reports whether this adapter appears scaffolded in root: its
// generated primitive-map file exists on disk.
func (m *Manifest) Scaffolded(root string) bool {
	for _, f := range m.Files {
		if f.PrimitiveMap {
			if _, err := os.Stat(filepath.Join(root, f.Dst)); err == nil {
				return true
			}
		}
	}
	return false
}

// nativeSkillsDir is the harness's native skills directory derived from the
// detector marker (e.g. ".claude/" → ".claude/skills"), used by the duplicate
// guard.
func (m *Manifest) nativeSkillsDir() string {
	if m.Detector.Exists == "" {
		return ""
	}
	return strings.TrimSuffix(m.Detector.Exists, "/") + "/skills"
}

// CoreSkillNames lists the core skill directory names (immediate subdirs of
// core/skills in the embed FS), in sorted order.
func CoreSkillNames() ([]string, error) {
	entries, err := fs.ReadDir(FS, "core/skills")
	if err != nil {
		return nil, fmt.Errorf("read core/skills: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// CheckDuplicateSkills implements the Q10 duplicate guard: for every core
// skill, refuse if a same-name skill already lives in any adapter's native
// skills directory (e.g. a leftover .claude/skills/fledge-orchestrate/ from
// 0.1.0). Returns the first conflict as an error.
func CheckDuplicateSkills(root string) error {
	adapters, err := LoadAdapters()
	if err != nil {
		return err
	}
	names, err := CoreSkillNames()
	if err != nil {
		return err
	}
	for _, m := range adapters {
		nsd := m.nativeSkillsDir()
		if nsd == "" {
			continue
		}
		for _, skill := range names {
			skillDir := filepath.Join(root, nsd, skill)
			if fi, err := os.Lstat(skillDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				// The sanctioned pointer to the core skill, not a copy.
				continue
			}
			if pathIsFile(filepath.Join(skillDir, "SKILL.md")) {
				return fmt.Errorf("duplicate skill %q at %s — remove the old copy first; see MIGRATION.md", skill, filepath.ToSlash(skillDir))
			}
		}
	}
	return nil
}

// pathIsFile reports whether p exists and is a regular file (not a directory).
// Used by the guard to detect a real SKILL.md.
func pathIsFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// fileExists reports whether path exists (file or directory).
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// devModulePath is the module identity a --dev source tree's go.mod must
// declare (interrogation Q2: go.mod identity, not a compiled-in path or an
// environment variable — PLM-031 FC-2).
const devModulePath = "github.com/Harrison-Blair/fledge"

// ValidateDevSource resolves path to an absolute directory and confirms it is
// a usable fledge source tree: its go.mod declares the fledge module. It must
// be called, and must succeed, before any scaffold file is written (PLM-031
// FC-4/AC-3 — a bad path leaves the scaffold untouched). The returned path is
// absolute so symlink targets never depend on the process's working
// directory.
func ValidateDevSource(sourcePath string) (string, error) {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve dev source %q: %w", sourcePath, err)
	}
	data, err := os.ReadFile(filepath.Join(abs, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("%s is not a fledge source tree (could not read go.mod): %w", abs, err)
	}
	want := "module " + devModulePath
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == want {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("%s is not a fledge source tree (go.mod does not declare module %s)", abs, devModulePath)
	}
	return abs, nil
}

// devLinkResult classifies the outcome of writeDevLink.
type devLinkResult int

const (
	devLinkCreated devLinkResult = iota
	devLinkUpdated
	devLinkUnchanged
)

// writeDevLink ensures dst is a symlink to the absolute path target, creating
// parent directories as needed. Always managed, like the manifest's symlink
// policy: an existing regular file/dir or a symlink pointing elsewhere is
// replaced (os.Symlink fails on an existing path, so removal comes first) —
// this makes re-running --dev idempotent (PLM-031 AC-7 test) rather than an
// error, and lets --dev convert a previously-copied file to a link.
func writeDevLink(dst, target string) (devLinkResult, error) {
	if fi, lErr := os.Lstat(dst); lErr == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if cur, rlErr := os.Readlink(dst); rlErr == nil && cur == target {
				return devLinkUnchanged, nil
			}
		}
		if rmErr := os.Remove(dst); rmErr != nil {
			return devLinkUnchanged, rmErr
		}
		if slErr := makeSymlink(target, dst); slErr != nil {
			return devLinkUnchanged, slErr
		}
		return devLinkUpdated, nil
	}
	if slErr := makeSymlink(target, dst); slErr != nil {
		return devLinkUnchanged, slErr
	}
	return devLinkCreated, nil
}

// devCoreTarget returns the absolute dev-source path a core skill file at
// core-relative rel (e.g. "skills/fledge-orchestrate/SKILL.md") should link
// to.
func devCoreTarget(devSource, rel string) string {
	return filepath.ToSlash(filepath.Join(devSource, "internal", "bootstrap", "core", filepath.FromSlash(rel)))
}

// devAdapterTarget returns the absolute dev-source path an adapter file entry
// should link to, given the adapter's embed-FS directory (e.g.
// "adapters/claude") and its manifest-relative src (e.g.
// "agents/fledge-brooder.md").
func devAdapterTarget(devSource, adapterDir, src string) string {
	return filepath.ToSlash(filepath.Join(devSource, "internal", "bootstrap", filepath.FromSlash(adapterDir), filepath.FromSlash(src)))
}

// relDevTarget expresses the absolute dev-link target abs as a path relative
// to the directory of the linked file at repoRelDst (repo-relative), for a
// self-link (bare --dev, PLM-031 FC-3) so the link survives the tree being
// moved or cloned. Only valid when devSource is the on-disk repo root itself
// (i.e. opts.SelfLink), which relDevTarget's callers guarantee.
func relDevTarget(devSource, repoRelDst, abs string) (string, error) {
	dst := filepath.Join(devSource, filepath.FromSlash(repoRelDst))
	rel, err := filepath.Rel(filepath.Dir(dst), abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// WriteCore writes the embedded core/skills tree into root/.fledge/skills/.
// Without refresh, existing files are skipped. With refresh, every file is
// reset to the shipped version. Returns created/updated/skipped repo-relative
// paths.
func WriteCore(root string, opts WriteOpts) (created, updated, skipped []string, err error) {
	err = fs.WalkDir(FS, "core", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "core/")      // skills/fledge-…/SKILL.md
		dst := filepath.Join(root, ".fledge", rel) // .fledge/skills/…
		relRepo := filepath.ToSlash(filepath.Join(".fledge", rel))

		if opts.DevSource != "" {
			target := devCoreTarget(opts.DevSource, rel)
			if opts.SelfLink {
				rt, rErr := relDevTarget(opts.DevSource, relRepo, target)
				if rErr != nil {
					return rErr
				}
				target = rt
			}
			state, dErr := writeDevLink(dst, target)
			if dErr != nil {
				return dErr
			}
			switch state {
			case devLinkCreated:
				created = append(created, relRepo)
			case devLinkUpdated:
				updated = append(updated, relRepo)
			default:
				skipped = append(skipped, relRepo)
			}
			return nil
		}

		exists := fileExists(dst)
		if exists && !opts.Refresh {
			skipped = append(skipped, relRepo)
			return nil
		}
		data, rErr := FS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		wrote, wErr := writeIfChanged(dst, data)
		if wErr != nil {
			return wErr
		}
		switch {
		case !exists:
			created = append(created, relRepo)
		case wrote:
			updated = append(updated, relRepo)
		default:
			skipped = append(skipped, relRepo)
		}
		return nil
	})
	return created, updated, skipped, err
}

// WriteAdapter writes one adapter's files into root per their policies.
// commandOrder is the CLI's command order, fed to generated templates (e.g. the
// Claude allow-list, Q23). Returns created/updated/skipped paths.
func (m *Manifest) WriteAdapter(root string, commandOrder []string, opts WriteOpts) (created, updated, skipped []string, err error) {
	ctx := m.renderContext(commandOrder)
	for _, f := range m.Files {
		c, u, s, wErr := m.writeFileEntry(root, f, ctx, opts)
		if wErr != nil {
			return created, updated, skipped, fmt.Errorf("adapter %s: %w", m.Name, wErr)
		}
		created = append(created, c...)
		updated = append(updated, u...)
		skipped = append(skipped, s...)
	}
	return created, updated, skipped, nil
}

func (m *Manifest) writeFileEntry(root string, f ManifestFile, ctx renderContext, opts WriteOpts) (created, updated, skipped []string, err error) {
	dst := filepath.Join(root, filepath.FromSlash(f.Dst))
	exists := fileExists(dst)

	// Symlink: dst points at target; created or repointed, never copied.
	// Always managed — no preserve check.
	if f.Symlink != "" {
		if fi, lErr := os.Lstat(dst); lErr == nil {
			if fi.Mode()&os.ModeSymlink == 0 {
				// A real file/dir is in the way; leave it to the user (the
				// duplicate guard already refuses core-skill copies).
				return nil, nil, []string{f.Dst}, nil
			}
			if cur, rlErr := os.Readlink(dst); rlErr == nil && cur == filepath.FromSlash(f.Symlink) {
				return nil, nil, []string{f.Dst}, nil
			}
			if rmErr := os.Remove(dst); rmErr != nil {
				return nil, nil, nil, rmErr
			}
			if slErr := makeSymlink(f.Symlink, dst); slErr != nil {
				return nil, nil, nil, slErr
			}
			return nil, []string{f.Dst}, nil, nil
		}
		if slErr := makeSymlink(f.Symlink, dst); slErr != nil {
			return nil, nil, nil, slErr
		}
		return []string{f.Dst}, nil, nil, nil
	}

	// Additive append: ensure the line is present; never clobber.
	if f.AppendIfMissing != "" || (f.Src == "" && f.Dst != "") {
		line := f.AppendIfMissing
		if line == "" {
			return nil, nil, nil, fmt.Errorf("file entry for %q has no src and no append_if_missing", f.Dst)
		}
		had, aErr := ensureLine(dst, line)
		if aErr != nil {
			return nil, nil, nil, aErr
		}
		switch {
		case !exists:
			return []string{f.Dst}, nil, nil, nil
		case had:
			return nil, nil, []string{f.Dst}, nil
		default:
			return nil, []string{f.Dst}, nil, nil
		}
	}

	// Generated (template) file: always managed — render; rewritten when content differs.
	if f.Generate || f.PrimitiveMap {
		data, rErr := renderEntry(m, f, ctx)
		if rErr != nil {
			return nil, nil, nil, rErr
		}
		return classifyWrite(dst, f.Dst, data, exists)
	}

	// Overwrite policy: always managed — copy verbatim; rewritten when content differs.
	if f.Overwrite {
		data, rErr := renderEntry(m, f, ctx)
		if rErr != nil {
			return nil, nil, nil, rErr
		}
		return classifyWrite(dst, f.Dst, data, exists)
	}

	// Default policy, dev mode: this is the copy-type file set dev mode
	// overrides (PLM-031 FC-5) — link into the source tree instead of
	// copying. Always managed, like the symlink policy above.
	if opts.DevSource != "" {
		target := devAdapterTarget(opts.DevSource, m.dir, f.Src)
		if opts.SelfLink {
			rt, rErr := relDevTarget(opts.DevSource, f.Dst, target)
			if rErr != nil {
				return nil, nil, nil, rErr
			}
			target = rt
		}
		state, dErr := writeDevLink(dst, target)
		if dErr != nil {
			return nil, nil, nil, dErr
		}
		switch state {
		case devLinkCreated:
			return []string{f.Dst}, nil, nil, nil
		case devLinkUpdated:
			return nil, []string{f.Dst}, nil, nil
		default:
			return nil, nil, []string{f.Dst}, nil
		}
	}

	// Default policy: copy verbatim, skip-if-exists (user may customize);
	// reset to the embedded version by `init --refresh`.
	if exists && !opts.Refresh {
		return nil, nil, []string{f.Dst}, nil
	}
	data, rErr := renderEntry(m, f, ctx)
	if rErr != nil {
		return nil, nil, nil, rErr
	}
	return classifyWrite(dst, f.Dst, data, exists)
}

// PruneObsolete removes one obsolete path (present in the old stamp, absent
// from the new expected tree). Refresh is a reset-to-shipped operation and the
// CLI confirms user-edited overwrites up front (EditedOnRefresh), so the file
// is removed regardless of its content. Append-policy entries are never
// deleted — fledge only ever added lines to a file it does not own. Returns
// deleted=false when the path does not exist (or is append-policy).
func PruneObsolete(root, repoPath string, entry StampEntry) (deleted bool, err error) {
	if entry.Policy == "append" {
		return false, nil
	}
	abs := filepath.Join(root, filepath.FromSlash(repoPath))
	if _, lErr := os.Lstat(abs); lErr != nil {
		if os.IsNotExist(lErr) {
			return false, nil
		}
		return false, lErr
	}
	if rmErr := os.Remove(abs); rmErr != nil {
		return false, rmErr
	}
	return true, nil
}

// classifyWrite writes data to dst when its content differs and classifies the
// outcome: created (absent), updated (bytes differed), or skipped (identical).
func classifyWrite(dst, rel string, data []byte, exists bool) (created, updated, skipped []string, err error) {
	wrote, wErr := writeIfChanged(dst, data)
	if wErr != nil {
		return nil, nil, nil, wErr
	}
	switch {
	case !exists:
		return []string{rel}, nil, nil, nil
	case wrote:
		return nil, []string{rel}, nil, nil
	default:
		return nil, nil, []string{rel}, nil
	}
}

// makeSymlink creates parent dirs and a symlink at dst pointing to target
// (slash-separated in the manifest, converted for the host OS).
func makeSymlink(target, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Symlink(filepath.FromSlash(target), dst)
}

// writeFile writes data, truncating any existing file.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// writeIfChanged writes data to dst unless dst already holds exactly these
// bytes. Creates parent dirs. Returns wrote=false when byte-identical.
func writeIfChanged(dst string, data []byte) (wrote bool, err error) {
	if existing, rErr := os.ReadFile(dst); rErr == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	return true, writeFile(dst, data)
}

// ensureLine appends line to the file at path if the line is not already
// present (line-based match, trimmed). Creates the file (and parent dirs) if it
// does not exist. Returns had=true when the line was already present.
func ensureLine(path, line string) (had bool, err error) {
	existing, rErr := os.ReadFile(path)
	if rErr != nil && !os.IsNotExist(rErr) {
		return false, rErr
	}
	for _, l := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(l) == strings.TrimSpace(line) {
			return true, nil
		}
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return false, mkErr
	}
	var b []byte
	b = append(b, existing...)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b = append(b, '\n')
	}
	b = append(b, []byte(line)...)
	b = append(b, '\n')
	return false, os.WriteFile(path, b, 0o644)
}
