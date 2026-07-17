package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/bootstrap"
	"github.com/Harrison-Blair/fledge/internal/repo"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("init", runInit,
		"fledge init [--agent <name>]... [--dev[=<path>]] [--refresh] [--force] [--list-agents] [--json]")
}

// devFlag implements flag.Value and the boolFlag interface (IsBoolFlag) so
// both `--dev=<path>` and a bare `--dev` parse without an explicit "=value".
// A bare --dev (Set("true")) infers its source from the current repo, which
// must itself be a fledge source checkout (PLM-031 FC-3); Set never accepts a
// stray "true" as a literal directory name. The space-separated form
// (`--dev <path>`) parses as bare --dev plus a stray positional argument,
// which runInit rejects via fs.NArg().
type devFlag struct {
	path string
	bare bool
}

func (d *devFlag) String() string   { return d.path }
func (d *devFlag) IsBoolFlag() bool { return true }
func (d *devFlag) Set(v string) error {
	if v == "true" {
		d.bare = true
		return nil
	}
	d.path = v
	return nil
}

//go:embed fledgeignore.default
var defaultScanIgnore []byte

// gitignore lines fledge needs; appended as one block when any is missing.
var gitignoreLines = []string{".fledge/nest/raw/", ".fledge/broods/", ".fledge/roster/", ".fledge/scratch/", ".alloc.lock"}

// stringListFlag implements flag.Value for a repeatable, comma-separated list.
type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }
func (s *stringListFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if name := strings.TrimSpace(part); name != "" {
			*s = append(*s, name)
		}
	}
	return nil
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var agents stringListFlag
	fs.Var(&agents, "agent", "agent harness to scaffold (repeatable, comma-separated)")
	var dev devFlag
	fs.Var(&dev, "dev", "scaffold copy-type files as symlinks into a local fledge source tree (--dev=<path>, or bare --dev inside a fledge source checkout)")
	refresh := fs.Bool("refresh", false, "reset all fledge-owned files to the shipped versions (confirms before overwriting user-edited files)")
	force := fs.Bool("force", false, "with --refresh: skip the confirmation prompt and overwrite user-edited files")
	listAgents := fs.Bool("list-agents", false, "list available agent adapters and exit")
	jsonOut := fs.Bool("json", false, "machine-readable output")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}

	if *listAgents {
		return listAdapters(false, *jsonOut)
	}

	// fs.NArg() > 0: fledge init takes no positional arguments. This matters
	// most for --dev, an IsBoolFlag: the natural-looking space form
	// (`--dev <path>`) parses as bare --dev plus a stray positional, which
	// would otherwise silently link the wrong target inside a source checkout
	// (bare --dev now self-links) rather than erroring. Name the likely cause
	// when --dev was given bare; a generic usage error otherwise (PLM-031
	// FC-3 footgun guard).
	if fs.NArg() > 0 {
		if dev.bare {
			return usageErr("unexpected argument %q after a bare --dev — use --dev=%s, not --dev %s", fs.Arg(0), fs.Arg(0), fs.Arg(0))
		}
		return usageErr("unexpected argument %q", fs.Arg(0))
	}

	r, err := repo.Find()
	if err != nil {
		return envErr("%v", err)
	}

	// --dev: resolve and validate the source before touching anything else.
	// A bare --dev infers the source from the current repo, which must itself
	// be a fledge source checkout (PLM-031 FC-3); outside one it fails rather
	// than guessing.
	var devSource string
	selfLink := false
	switch {
	case dev.bare:
		abs, vErr := bootstrap.ValidateDevSource(r.Root)
		if vErr != nil {
			return fail("--dev requires a source path outside a fledge source checkout: %v", vErr)
		}
		devSource = abs
		selfLink = true
	case dev.path != "":
		abs, vErr := bootstrap.ValidateDevSource(dev.path)
		if vErr != nil {
			return fail("%v", vErr)
		}
		devSource = abs
	}

	// Version skew (PLM-031 FC-7): a report, not a gate — say nothing when the
	// source has no VERSION file (unknown) or matches the running binary.
	if devSource != "" {
		if srcVersion := (&repo.Repo{Root: devSource}).Version(""); srcVersion != "" && srcVersion != binaryVersion {
			fmt.Fprintf(os.Stderr, "fledge: dev source is fledge %s, binary is %s — linked skill/agent prose may reference CLI behavior this binary lacks\n", srcVersion, binaryVersion)
		}
	}

	// Load old stamp now: refresh detection and the stamp's agents union need it.
	oldStamp, _ := bootstrap.LoadStamp(r.Root)

	// Resolve which adapters to scaffold (Q7).
	selected, defaulted, err := resolveAgents(r.Root, agents)
	if err != nil {
		return fail("%v", err)
	}

	// Resolve manifests and build the expected-file map (base + core + adapters)
	// before any write: refresh needs it to detect user edits up front, and the
	// stamp is built from it afterwards.
	allFiles := baseScaffoldEntries()
	manifests := make([]*bootstrap.Manifest, 0, len(selected))
	for _, name := range selected {
		m, err := bootstrap.FindAdapter(name)
		if err != nil {
			return fail("%v", err)
		}
		if m == nil {
			return usageErr("unknown agent %q (run `fledge init --list-agents`)", name)
		}
		manifests = append(manifests, m)
		ef, efErr := bootstrap.ExpectedFilesDev(m, commandOrder, devSource, selfLink)
		if efErr != nil {
			return fail("build expected files for %s: %v", name, efErr)
		}
		for k, v := range ef {
			allFiles[k] = v
		}
	}

	// Duplicate guard (Q10): refuse a broken state before writing anything.
	if err := bootstrap.CheckDuplicateSkills(r.Root); err != nil {
		return fail("%v", err)
	}

	// Git hygiene (PLM-031 FC-6): refuse outright, before any write, if a
	// dev-linked path is already tracked by git — the whole point of a
	// refusal is that it leaves the tree exactly as found.
	var devLinkedPaths []string
	if devSource != "" {
		for p, e := range allFiles {
			if e.Policy == "dev-link" {
				devLinkedPaths = append(devLinkedPaths, p)
			}
		}
		sort.Strings(devLinkedPaths)
	}
	if len(devLinkedPaths) > 0 {
		if tracked := trackedDevPaths(r.Root, devLinkedPaths); len(tracked) > 0 {
			fmt.Fprintln(os.Stderr, "refusing --dev: the following dev-linked path(s) are already tracked by git:")
			for _, p := range tracked {
				fmt.Fprintf(os.Stderr, "  %s\n", p)
			}
			fmt.Fprintf(os.Stderr, "untrack them first: git rm --cached %s\n", strings.Join(tracked, " "))
			return ExitFail
		}
	}

	// Refresh is a reset-to-shipped: confirm before overwriting user edits.
	if *refresh && !*force {
		edited := bootstrap.EditedOnRefresh(r.Root, oldStamp, allFiles)
		if len(edited) > 0 {
			fmt.Fprintf(os.Stderr, "refresh will overwrite %d user-edited file(s):\n", len(edited))
			for _, p := range edited {
				fmt.Fprintf(os.Stderr, "  %s\n", p)
			}
			if !*jsonOut && stdinIsTTY() {
				if !promptYesNo(os.Stdin, os.Stderr, fmt.Sprintf("overwrite %d user-edited file(s)? [y/N] ", len(edited))) {
					fmt.Fprintln(os.Stderr, "aborted; nothing written")
					return ExitFail
				}
			} else {
				fmt.Fprintln(os.Stderr, "refusing to overwrite; rerun with --force")
				return ExitFail
			}
		}
	}

	var created, skipped, updated []string
	note := func(rel string, state int) {
		switch state {
		case 0:
			created = append(created, rel)
		case 1:
			skipped = append(skipped, rel)
		case 2:
			updated = append(updated, rel)
		}
	}

	// Base fledge scaffold (agent-agnostic, additive).
	baseFiles := []struct {
		rel     string
		content []byte
	}{
		{".fledge/nest/raw/.gitkeep", nil},
		{".fledge/broods/.gitkeep", nil},
		{".fledgeignore", defaultScanIgnore},
		{".fledge/pluma/plumage/.gitkeep", nil},
		{".fledge/pluma/feathers/.gitkeep", nil},
	}
	for _, f := range baseFiles {
		path := filepath.Join(r.Root, f.rel)
		exists := fileExists(path)
		if exists && !*refresh {
			note(f.rel, 1)
			continue
		}
		if exists {
			if cur, rErr := os.ReadFile(path); rErr == nil && bytes.Equal(cur, f.content) {
				note(f.rel, 1)
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fail("%v", err)
		}
		if err := spec.WriteFileAtomic(path, f.content); err != nil {
			return fail("%v", err)
		}
		if exists {
			note(f.rel, 2)
		} else {
			note(f.rel, 0)
		}
	}

	gitignoreChanged, err := ensureGitignore(filepath.Join(r.Root, ".gitignore"))
	if err != nil {
		return fail("%v", err)
	}
	if len(devLinkedPaths) > 0 {
		devChanged, dErr := ensureDevGitignore(filepath.Join(r.Root, ".gitignore"), devLinkedPaths)
		if dErr != nil {
			return fail("%v", dErr)
		}
		gitignoreChanged = gitignoreChanged || devChanged
	}
	if gitignoreChanged {
		note(".gitignore", 0)
	} else {
		note(".gitignore", 1)
	}

	// Core skill: agent-neutral, written to .fledge/skills/ (Q2).
	opts := bootstrap.WriteOpts{Refresh: *refresh, DevSource: devSource, SelfLink: selfLink}
	cCreated, cUpdated, cSkipped, err := bootstrap.WriteCore(r.Root, opts)
	if err != nil {
		return fail("%v", err)
	}
	created = append(created, cCreated...)
	updated = append(updated, cUpdated...)
	skipped = append(skipped, cSkipped...)

	// Adapters: per-harness, written to their native paths.
	var scaffolded []string
	for _, m := range manifests {
		aCreated, aUpdated, aSkipped, err := m.WriteAdapter(r.Root, commandOrder, opts)
		if err != nil {
			return fail("%v", err)
		}
		created = append(created, aCreated...)
		updated = append(updated, aUpdated...)
		skipped = append(skipped, aSkipped...)
		scaffolded = append(scaffolded, m.Name)
	}
	sort.Strings(scaffolded)

	// Build stamp: agents = this run's adapters ∪ old stamp's agents.
	agentSet := map[string]bool{}
	for _, a := range scaffolded {
		agentSet[a] = true
	}
	if oldStamp != nil {
		for _, a := range oldStamp.Agents {
			agentSet[a] = true
		}
	}
	var stampAgents []string
	for a := range agentSet {
		stampAgents = append(stampAgents, a)
	}
	sort.Strings(stampAgents)

	const stampRel = ".fledge/scaffold.json"
	stampPreexists := fileExists(filepath.Join(r.Root, stampRel))
	stamp := &bootstrap.Stamp{
		FledgeVersion: binaryVersion,
		Agents:        stampAgents,
		Files:         allFiles,
		DevSource:     devSource,
	}
	stampWrote, sErr := stamp.Write(r.Root)
	if sErr != nil {
		return fail("write scaffold stamp: %v", sErr)
	}
	switch {
	case !stampPreexists:
		created = append(created, stampRel)
	case stampWrote:
		updated = append(updated, stampRel)
	default:
		skipped = append(skipped, stampRel)
	}

	// Prune pass (refresh only): remove obsolete files (in the old stamp,
	// absent from the new expected tree). User-edited ones were confirmed above.
	var pruned []string
	if *refresh && oldStamp != nil {
		newExpected := make(map[string]bool, len(allFiles)+1)
		for k := range allFiles {
			newExpected[k] = true
		}
		newExpected[stampRel] = true

		var pruneTargets []string
		for p := range oldStamp.Files {
			if !newExpected[p] {
				pruneTargets = append(pruneTargets, p)
			}
		}
		sort.Strings(pruneTargets)

		for _, p := range pruneTargets {
			del, pErr := bootstrap.PruneObsolete(r.Root, p, oldStamp.Files[p])
			if pErr != nil {
				return fail("prune %s: %v", p, pErr)
			}
			if del {
				pruned = append(pruned, p)
				// Remove now-empty ancestor dirs within .fledge/skills/ and .claude/.
				removeEmptyParents(r.Root, p)
			}
		}
	}

	if *refresh && len(updated) > 0 {
		fmt.Fprintf(os.Stderr, "note: refreshed %d file(s) to the shipped versions — `git diff` to review; your edits are recoverable via git.\n", len(updated))
	}

	if defaulted && !*jsonOut {
		fmt.Fprintf(os.Stderr, "note: no agent harness detected; scaffolded the claude adapter by default. Run `fledge init --agent <name>` to add another (see `fledge init --list-agents`).\n")
	}

	if *jsonOut {
		return emitJSON(initJSON{
			Created: nonEmpty(created),
			Skipped: nonEmpty(skipped),
			Updated: nonEmpty(updated),
			Agents:  nonEmpty(scaffolded),
			Removed: nonEmpty(pruned),
		})
	}
	for _, rel := range created {
		fmt.Printf("created %s\n", rel)
	}
	for _, rel := range updated {
		fmt.Printf("updated %s\n", rel)
	}
	for _, rel := range pruned {
		fmt.Printf("removed %s\n", rel)
	}
	for _, rel := range skipped {
		fmt.Printf("exists %s\n", rel)
	}
	if len(scaffolded) > 0 {
		fmt.Printf("scaffolded agents: %s\n", strings.Join(scaffolded, ", "))
	}
	return ExitOK
}

type initJSON struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
	Updated []string `json:"updated"`
	Agents  []string `json:"agents"`
	Removed []string `json:"removed"`
}

// stdinIsTTY reports whether stdin is an interactive terminal (char device).
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// promptYesNo writes prompt to w and reads one line from r; only an explicit
// "y"/"yes" (case-insensitive) answers true.
func promptYesNo(r io.Reader, w io.Writer, prompt string) bool {
	fmt.Fprint(w, prompt)
	sc := bufio.NewScanner(r)
	if !sc.Scan() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(sc.Text())) {
	case "y", "yes":
		return true
	}
	return false
}

// removeEmptyParents removes empty ancestor directories under .fledge/skills/
// and .claude/ (the two managed subtrees) after a file prune.
func removeEmptyParents(root, repoPath string) {
	managed := []string{".fledge/skills/", ".claude/"}
	for _, prefix := range managed {
		if !strings.HasPrefix(repoPath, prefix) {
			continue
		}
		dir := filepath.Join(root, filepath.FromSlash(filepath.Dir(repoPath)))
		for {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				break
			}
			rel = filepath.ToSlash(rel)
			if rel == strings.TrimSuffix(prefix, "/") || rel == "." {
				break
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) > 0 {
				break
			}
			if err := os.Remove(dir); err != nil {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
}

func nonEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// baseScaffoldEntries returns StampEntry values for the agent-agnostic base
// files that runInit writes. These are merged into the ExpectedFiles map before
// building the stamp, so init and later preen/refresh share the same complete
// picture of every scaffolded file.
func baseScaffoldEntries() map[string]bootstrap.StampEntry {
	out := make(map[string]bootstrap.StampEntry)
	for _, f := range []struct {
		rel     string
		content []byte
	}{
		{".fledge/nest/raw/.gitkeep", nil},
		{".fledge/broods/.gitkeep", nil},
		{".fledgeignore", defaultScanIgnore},
		{".fledge/pluma/plumage/.gitkeep", nil},
		{".fledge/pluma/feathers/.gitkeep", nil},
	} {
		h := sha256.Sum256(f.content)
		out[f.rel] = bootstrap.StampEntry{
			Policy: "default",
			Sha256: fmt.Sprintf("%x", h),
		}
	}
	out[".gitignore"] = bootstrap.StampEntry{
		Policy: "append",
		Lines:  gitignoreLines,
	}
	return out
}

// resolveAgents implements Q7: --agent overrides/adds; otherwise auto-detect via
// each adapter's marker; nothing detected → claude default + hint (defaulted=true).
func resolveAgents(root string, agents stringListFlag) (selected []string, defaulted bool, err error) {
	if len(agents) > 0 {
		// Deduplicate, preserve order.
		seen := map[string]bool{}
		for _, a := range agents {
			if !seen[a] {
				seen[a] = true
				selected = append(selected, a)
			}
		}
		return selected, false, nil
	}
	detected, err := bootstrap.DetectAdapters(root)
	if err != nil {
		return nil, false, err
	}
	if len(detected) == 0 {
		return []string{"claude"}, true, nil
	}
	for _, m := range detected {
		selected = append(selected, m.Name)
	}
	sort.Strings(selected)
	return selected, false, nil
}

// listAdapters prints available adapters (name, tier) and, when inRepo is true,
// which are scaffolded in the current repo. Used by --list-agents.
func listAdapters(inRepo bool, jsonOut bool) int {
	adapters, err := bootstrap.LoadAdapters()
	if err != nil {
		return fail("%v", err)
	}
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Name < adapters[j].Name })
	if jsonOut {
		out := make([]adapterInfo, 0, len(adapters))
		for _, m := range adapters {
			out = append(out, manifestInfo(m, false))
		}
		return emitJSON(map[string]any{"agents": out})
	}
	for _, m := range adapters {
		fmt.Printf("%s\ttier %s\n", m.Name, tierLabel(m.Tier()))
	}
	return ExitOK
}

// tierLabel renders a tier, marking sub-A adapters.
func tierLabel(t string) string {
	if t == "" {
		return "—"
	}
	return t
}

// ensureGitignore appends fledge's ignore lines when missing; reports change.
func ensureGitignore(path string) (bool, error) {
	return ensureGitignoreBlock(path, "# fledge — per-run intermediates, regenerable", gitignoreLines)
}

// ensureDevGitignore appends devPaths (PLM-031 FC-6) as their own, separate,
// conditional block — present only in a dev-linked repo — unlike
// ensureGitignore's unconditional block. Writes path patterns, not
// machine-specific locations, so the block is valid for any clone; the actual
// dev-source location is recorded only in .fledge/scaffold.json's devSource.
func ensureDevGitignore(path string, devPaths []string) (bool, error) {
	return ensureGitignoreBlock(path, "# fledge dev mode — symlinked into a local fledge source checkout (fledge init --dev)", devPaths)
}

// ensureGitignoreBlock appends any of lines missing from path as one labeled
// block, idempotently; reports whether it wrote anything. Shared by
// ensureGitignore (always-on) and ensureDevGitignore (dev-mode only).
func ensureGitignoreBlock(path, header string, lines []string) (bool, error) {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	have := map[string]bool{}
	for _, l := range strings.Split(string(existing), "\n") {
		have[strings.TrimSpace(l)] = true
	}
	var missing []string
	for _, l := range lines {
		if !have[l] {
			missing = append(missing, l)
		}
	}
	if len(missing) == 0 {
		return false, nil
	}
	var b strings.Builder
	b.Write(existing)
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(header + "\n")
	for _, l := range missing {
		b.WriteString(l + "\n")
	}
	return true, spec.WriteFileAtomic(path, []byte(b.String()))
}

// trackedDevPaths returns which of paths are already tracked by git in the
// repo at root (PLM-031 FC-6): a dev-linked path must not already be a
// tracked file, or --dev would silently turn a real tracked file into an
// untracked symlink. Not-a-git-repo or no git binary is not an error state
// for fledge generally, so any failure degrades to "none tracked" rather than
// aborting the command.
func trackedDevPaths(root string, paths []string) []string {
	args := append([]string{"-C", root, "ls-files", "--"}, paths...)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
