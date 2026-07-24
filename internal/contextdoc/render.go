package contextdoc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

type contextRun struct {
	scan       Scan
	requests   map[string]AnalyzerRequest
	replies    map[string]AnalyzerReply
	synthesis  Synthesis
	provenance Provenance
	inputFiles []*runArtifact
}

type rootedDirectory struct {
	root *os.Root
	name string
	info os.FileInfo
}

type runArtifact struct {
	parent *rootedDirectory
	name   string
	path   string
	info   os.FileInfo
	file   *os.File
}

type runPaths struct {
	root         string
	runName      string
	contextDir   *rootedDirectory
	runsDir      *rootedDirectory
	runRoot      *rootedDirectory
	requestDir   *rootedDirectory
	replyDir     *rootedDirectory
	scan         *runArtifact
	requestFiles []*runArtifact
	replyFiles   []*runArtifact
	synthesis    *runArtifact
	provenance   *runArtifact
}

// RenderProject validates all artifacts below runDir, atomically replaces the
// workspace's .fledge/context/project.md, and then removes the consumed JSON.
// Validation and rendering failures leave both the prior document and every
// run artifact untouched. Failures after the atomic rename are returned as
// warnings with the valid publication result.
func RenderProject(runDir string) (RenderResult, error) {
	return renderProject(runDir, time.Now().UTC())
}

func renderProject(runDir string, generatedAt time.Time) (RenderResult, error) {
	paths, err := preflightRun(runDir)
	if err != nil {
		return RenderResult{}, err
	}
	defer paths.close()
	if renderRootsOpened != nil {
		if err := renderRootsOpened(); err != nil {
			return RenderResult{}, err
		}
	}

	run, err := loadContextRun(paths)
	if err != nil {
		return RenderResult{}, err
	}
	document := renderMarkdown(run, generatedAt.UTC())
	warnings, err := writeAtomic(paths.contextDir, "project.md", []byte(document))
	if err != nil {
		return RenderResult{}, err
	}
	provenancePath := ""
	if provenanceDoc, err := json.MarshalIndent(run.provenance, "", "  "); err != nil {
		warnings = append(warnings, fmt.Sprintf("project published but provenance publication failed: %v", err))
	} else if provenanceWarnings, err := writeAtomic(paths.contextDir, "provenance.json", append(provenanceDoc, '\n')); err != nil {
		warnings = append(warnings, fmt.Sprintf("project published but provenance publication failed: %v", err))
	} else {
		provenancePath = filepath.ToSlash(filepath.Join(scaffold.DirName, "context", "provenance.json"))
		warnings = append(warnings, provenanceWarnings...)
	}
	if err := cleanupRun(paths, run.inputFiles); err != nil {
		warnings = append(warnings, fmt.Sprintf("run cleanup failed after publication: %v", err))
	}
	if warnings == nil {
		warnings = []string{}
	}
	sum := sha256.Sum256([]byte(document))
	return RenderResult{
		Path:           filepath.ToSlash(filepath.Join(scaffold.DirName, "context", "project.md")),
		SHA256:         hex.EncodeToString(sum[:]),
		ProvenancePath: provenancePath,
		Warnings:       warnings,
	}, nil
}

// preflightRun resolves and verifies the complete filesystem shape, then pins
// every directory and input before any artifact content is read. Later reads,
// publication, and cleanup are relative to these roots and handles, so replacing
// a lexical path cannot redirect an operation.
func preflightRun(runDir string) (runPaths, error) {
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return runPaths{}, err
	}
	root, err := workspace.FindRoot(absRunDir)
	if err != nil {
		return runPaths{}, err
	}
	runsDir := filepath.Join(root, scaffold.DirName, "context", "runs")
	rel, err := filepath.Rel(runsDir, absRunDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return runPaths{}, fmt.Errorf("run directory must be strictly beneath %s", runsDir)
	}
	canonical, err := filepath.EvalSymlinks(absRunDir)
	if err != nil {
		return runPaths{}, err
	}
	if canonical != absRunDir {
		return runPaths{}, fmt.Errorf("run directory must be canonical and contain no symlink components: %s", runDir)
	}
	contextName := filepath.Join(root, scaffold.DirName, "context")
	paths := runPaths{
		root:    root,
		runName: filepath.ToSlash(rel),
	}
	fail := func(err error) (runPaths, error) {
		_ = paths.close()
		return runPaths{}, err
	}

	if paths.contextDir, err = openCanonicalDirectory(contextName); err != nil {
		return fail(err)
	}
	if paths.runsDir, err = openPlainDirectory(paths.contextDir, "runs", runsDir); err != nil {
		return fail(err)
	}
	if paths.runRoot, err = openPlainDirectory(paths.runsDir, paths.runName, absRunDir); err != nil {
		return fail(err)
	}
	if paths.requestDir, err = openPlainDirectory(paths.runRoot, "requests", filepath.Join(absRunDir, "requests")); err != nil {
		return fail(err)
	}
	if paths.replyDir, err = openPlainDirectory(paths.runRoot, "replies", filepath.Join(absRunDir, "replies")); err != nil {
		return fail(err)
	}
	if paths.scan, err = openPlainArtifact(paths.runRoot, "scan.json"); err != nil {
		return fail(err)
	}
	if paths.synthesis, err = openPlainArtifact(paths.runRoot, "synthesis.json"); err != nil {
		return fail(err)
	}
	if paths.provenance, err = openPlainArtifact(paths.runRoot, "provenance.json"); err != nil {
		return fail(err)
	}
	if paths.requestFiles, err = plainJSONFiles(paths.requestDir); err != nil {
		return fail(err)
	}
	if paths.replyFiles, err = plainJSONFiles(paths.replyDir); err != nil {
		return fail(err)
	}
	return paths, nil
}

func openCanonicalDirectory(name string) (*rootedDirectory, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must not be a symlink", name)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", name)
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s changed while it was opened", name)
	}
	return &rootedDirectory{root: root, name: name, info: opened}, nil
}

func openPlainDirectory(parent *rootedDirectory, name, displayName string) (*rootedDirectory, error) {
	info, err := parent.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must not be a symlink", displayName)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", displayName)
	}
	root, err := parent.root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s changed while it was opened", displayName)
	}
	return &rootedDirectory{root: root, name: displayName, info: opened}, nil
}

func openPlainArtifact(parent *rootedDirectory, name string) (*runArtifact, error) {
	displayName := filepath.Join(parent.name, filepath.FromSlash(name))
	info, err := parent.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must not be a symlink", displayName)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", displayName)
	}
	file, err := parent.root.Open(name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%s changed while it was opened", displayName)
	}
	return &runArtifact{parent: parent, name: name, path: displayName, info: opened, file: file}, nil
}

func plainJSONFiles(dir *rootedDirectory) ([]*runArtifact, error) {
	file, err := dir.root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := file.ReadDir(-1)
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, err
	}
	var files []*runArtifact
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		artifact, err := openPlainArtifact(dir, entry.Name())
		if err != nil {
			for _, opened := range files {
				_ = opened.close()
			}
			return nil, err
		}
		files = append(files, artifact)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, nil
}

func loadContextRun(paths runPaths) (contextRun, error) {
	var run contextRun
	run.requests = make(map[string]AnalyzerRequest)
	run.replies = make(map[string]AnalyzerReply)

	scanData, err := paths.scan.read()
	if err != nil {
		return run, err
	}
	if err := decodeExact(scanData, &run.scan); err != nil {
		return run, fmt.Errorf("%s: %w", paths.scan.path, err)
	}
	if err := validateScan(run.scan, paths.root); err != nil {
		return run, fmt.Errorf("%s: %w", paths.scan.path, err)
	}
	run.inputFiles = append(run.inputFiles, paths.scan)

	for _, artifact := range paths.requestFiles {
		groupID := strings.TrimSuffix(artifact.name, ".json")
		data, err := artifact.read()
		if err != nil {
			return run, err
		}
		var request AnalyzerRequest
		if err := decodeExact(data, &request); err != nil {
			return run, fmt.Errorf("%s: %w", artifact.path, err)
		}
		if err := validateRequest(request); err != nil {
			return run, fmt.Errorf("%s: %w", artifact.path, err)
		}
		if request.GroupID != groupID {
			return run, fmt.Errorf("%s: group_id %q does not match filename %q", artifact.path, request.GroupID, groupID)
		}
		if _, exists := run.requests[groupID]; exists {
			return run, fmt.Errorf("duplicate request group_id %q", groupID)
		}
		run.requests[groupID] = request
		run.inputFiles = append(run.inputFiles, artifact)
	}
	if err := validateOriginalOwnership(run.scan, run.requests); err != nil {
		return run, err
	}

	for _, artifact := range paths.replyFiles {
		groupID := strings.TrimSuffix(artifact.name, ".json")
		request, exists := run.requests[groupID]
		if !exists {
			return run, fmt.Errorf("%s: reply has no matching request", artifact.path)
		}
		data, err := artifact.read()
		if err != nil {
			return run, err
		}
		reply, err := decodeAnalyzerReply(data)
		if err != nil {
			return run, fmt.Errorf("%s: %w", artifact.path, err)
		}
		if err := validateReply(reply, request); err != nil {
			return run, fmt.Errorf("%s: %w", artifact.path, err)
		}
		if err := validateInternalDependencies(reply.Dependencies.Internal, run.scan); err != nil {
			return run, fmt.Errorf("%s: %w", artifact.path, err)
		}
		if reply.GroupID != groupID {
			return run, fmt.Errorf("%s: group_id %q does not match filename %q", artifact.path, reply.GroupID, groupID)
		}
		if reply.Status != "ok" {
			return run, fmt.Errorf("%s: analyzer returned %d error(s)", artifact.path, len(reply.Errors))
		}
		run.replies[groupID] = reply
		run.inputFiles = append(run.inputFiles, artifact)
	}
	for groupID := range run.requests {
		if _, exists := run.replies[groupID]; !exists {
			return run, fmt.Errorf("missing reply for group %q", groupID)
		}
	}

	synthesisData, err := paths.synthesis.read()
	if err != nil {
		return run, err
	}
	if err := decodeExact(synthesisData, &run.synthesis); err != nil {
		return run, fmt.Errorf("%s: %w", paths.synthesis.path, err)
	}
	if err := validateSynthesis(run.synthesis, run.requests); err != nil {
		return run, fmt.Errorf("%s: %w", paths.synthesis.path, err)
	}
	run.inputFiles = append(run.inputFiles, paths.synthesis)

	provenanceData, err := paths.provenance.read()
	if err != nil {
		return run, err
	}
	if err := decodeExact(provenanceData, &run.provenance); err != nil {
		return run, fmt.Errorf("%s: %w", paths.provenance.path, err)
	}
	if err := validateProvenance(run.provenance, run.requests); err != nil {
		return run, fmt.Errorf("%s: %w", paths.provenance.path, err)
	}
	run.inputFiles = append(run.inputFiles, paths.provenance)
	return run, nil
}

func (artifact *runArtifact) read() ([]byte, error) {
	if artifact.file == nil {
		return nil, fmt.Errorf("%s: artifact is closed", artifact.path)
	}
	data, readErr := io.ReadAll(artifact.file)
	closeErr := artifact.close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("%s: %w", artifact.path, err)
	}
	return data, nil
}

func (artifact *runArtifact) close() error {
	if artifact == nil || artifact.file == nil {
		return nil
	}
	err := artifact.file.Close()
	artifact.file = nil
	return err
}

func (paths *runPaths) close() error {
	var errs []error
	for _, artifact := range append(append([]*runArtifact{
		paths.scan, paths.synthesis, paths.provenance,
	}, paths.requestFiles...), paths.replyFiles...) {
		if err := artifact.close(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, dir := range []*rootedDirectory{
		paths.replyDir, paths.requestDir, paths.runRoot, paths.runsDir, paths.contextDir,
	} {
		if dir != nil && dir.root != nil {
			if err := dir.root.Close(); err != nil {
				errs = append(errs, err)
			}
			dir.root = nil
		}
	}
	return errors.Join(errs...)
}

func validateScan(scan Scan, root string) error {
	if scan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if scan.Root != root {
		return fmt.Errorf("root is %q, want canonical workspace root %q", scan.Root, root)
	}
	if scan.Files == nil {
		return errors.New("files array must be present")
	}
	if scan.FileCount < 0 || scan.TotalSize < 0 {
		return errors.New("file_count and total_size must be nonnegative")
	}
	total, _, err := validateFiles(scan.Files)
	if err != nil {
		return err
	}
	if scan.FileCount != len(scan.Files) {
		return fmt.Errorf("file_count is %d, want %d", scan.FileCount, len(scan.Files))
	}
	if scan.TotalSize != total {
		return fmt.Errorf("total_size is %d, want %d", scan.TotalSize, total)
	}
	return nil
}

func validateInternalDependencies(dependencies []InternalDependency, scan Scan) error {
	for i, dependency := range dependencies {
		matched := false
		for _, file := range scan.Files {
			if pathHasPrefix(file.Path, dependency.Path) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("dependencies.internal[%d] path %q matches no scanned path", i, dependency.Path)
		}
	}
	return nil
}

func validateOriginalOwnership(scan Scan, requests map[string]AnalyzerRequest) error {
	_, scanned, _ := validateFiles(scan.Files)
	owned := make(map[string]string, len(scanned))
	for groupID, request := range requests {
		for _, file := range request.Files {
			size, exists := scanned[file.Path]
			if !exists {
				return fmt.Errorf("request %q contains unscanned path %q", groupID, file.Path)
			}
			if size != file.Size {
				return fmt.Errorf("request %q gives %q size %d, scan gives %d", groupID, file.Path, file.Size, size)
			}
			if owner, exists := owned[file.Path]; exists {
				return fmt.Errorf("path %q is assigned to both %q and %q", file.Path, owner, groupID)
			}
			owned[file.Path] = groupID
		}
	}
	for file := range scanned {
		if _, exists := owned[file]; !exists {
			return fmt.Errorf("scanned path %q is not assigned to a request", file)
		}
	}
	return nil
}

func validateSynthesis(synthesis Synthesis, requests map[string]AnalyzerRequest) error {
	if synthesis.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if strings.TrimSpace(synthesis.ProjectOverview) == "" {
		return errors.New("project_overview must be nonempty")
	}
	if synthesis.Routing == nil || synthesis.CrossGroupFlows == nil || synthesis.GlobalInvariants == nil {
		return errors.New("routing, cross_group_flows, and global_invariants arrays must be present")
	}
	owners := make(map[string]string)
	for groupID, request := range requests {
		for _, file := range request.Files {
			owners[file.Path] = groupID
		}
	}
	routed := make(map[string]bool, len(synthesis.Routing))
	for i, route := range synthesis.Routing {
		if !validPathPrefix(route.PathPrefix) {
			return fmt.Errorf("routing[%d] path_prefix %q is not safe and normalized", i, route.PathPrefix)
		}
		if _, exists := requests[route.GroupID]; !exists {
			return fmt.Errorf("routing[%d] references unknown group_id %q", i, route.GroupID)
		}
		if strings.TrimSpace(route.Guidance) == "" {
			return fmt.Errorf("routing[%d] guidance must be nonempty", i)
		}
		if routed[route.PathPrefix] {
			return fmt.Errorf("routing contains duplicate path_prefix %q", route.PathPrefix)
		}
		routed[route.PathPrefix] = true
		matched := false
		for file, owner := range owners {
			if !pathHasPrefix(file, route.PathPrefix) {
				continue
			}
			matched = true
			if owner != route.GroupID {
				return fmt.Errorf("routing[%d] path_prefix %q includes path %q owned by group %q, not %q",
					i, route.PathPrefix, file, owner, route.GroupID)
			}
		}
		if !matched {
			return fmt.Errorf("routing[%d] path_prefix %q matches no scanned path", i, route.PathPrefix)
		}
	}
	for i, flow := range synthesis.CrossGroupFlows {
		if _, exists := requests[flow.FromGroup]; !exists {
			return fmt.Errorf("cross_group_flows[%d] references unknown from_group %q", i, flow.FromGroup)
		}
		if _, exists := requests[flow.ToGroup]; !exists {
			return fmt.Errorf("cross_group_flows[%d] references unknown to_group %q", i, flow.ToGroup)
		}
		if strings.TrimSpace(flow.Description) == "" {
			return fmt.Errorf("cross_group_flows[%d] description must be nonempty", i)
		}
	}
	for i, invariant := range synthesis.GlobalInvariants {
		if strings.TrimSpace(invariant) == "" {
			return fmt.Errorf("global_invariants[%d] must be nonempty", i)
		}
	}
	return nil
}

func pathHasPrefix(name, prefix string) bool {
	return prefix == "." || name == prefix || strings.HasPrefix(name, prefix+"/")
}

func validPathPrefix(prefix string) bool {
	if prefix == "." {
		return true
	}
	return validRelativePath(prefix)
}

func validateProvenance(provenance Provenance, requests map[string]AnalyzerRequest) error {
	if provenance.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if err := validateIdentity("forager", provenance.Forager.Name, provenance.Forager.Profile, provenance.Forager.Model); err != nil {
		return err
	}
	seen := make(map[string]bool, len(provenance.Analyzers))
	seenNames := map[string]bool{provenance.Forager.Name: true}
	for i, analyzer := range provenance.Analyzers {
		if _, exists := requests[analyzer.GroupID]; !exists {
			return fmt.Errorf("analyzers[%d] references unknown group_id %q", i, analyzer.GroupID)
		}
		if seen[analyzer.GroupID] {
			return fmt.Errorf("analyzers contains duplicate group_id %q", analyzer.GroupID)
		}
		seen[analyzer.GroupID] = true
		if err := validateIdentity(fmt.Sprintf("analyzers[%d]", i), analyzer.Name, analyzer.Profile, analyzer.Model); err != nil {
			return err
		}
		if seenNames[analyzer.Name] {
			return fmt.Errorf("analyzers[%d] reuses agent name %q", i, analyzer.Name)
		}
		seenNames[analyzer.Name] = true
	}
	for groupID := range requests {
		if !seen[groupID] {
			return fmt.Errorf("analyzers is missing group_id %q", groupID)
		}
	}
	return nil
}

func validateIdentity(label, name, profile, model string) error {
	for _, item := range []struct {
		field string
		value string
	}{
		{"name", name}, {"profile", profile}, {"model", model},
	} {
		field, value := item.field, item.value
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s %s must be nonempty", label, field)
		}
		if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s %s must be a single trimmed value", label, field)
		}
	}
	return nil
}

func renderMarkdown(run contextRun, generatedAt time.Time) string {
	var out strings.Builder
	fmt.Fprintln(&out, "# Project Context")
	fmt.Fprintln(&out)
	fmt.Fprintf(&out, "_Generated at %s UTC._\n\n", generatedAt.UTC().Format("2006-01-02 15:04:05"))

	groupIDs := sortedGroupIDs(run.requests)
	fmt.Fprintln(&out, "## Project Overview")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, run.synthesis.ProjectOverview)

	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Routing")
	fmt.Fprintln(&out)
	if len(run.synthesis.Routing) == 0 {
		fmt.Fprintln(&out, "_None._")
	} else {
		for _, route := range run.synthesis.Routing {
			fmt.Fprintf(&out, "- %s → %s: %s\n", mdCode(route.PathPrefix), mdCode(route.GroupID), route.Guidance)
		}
	}

	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Cross-Group Flows")
	fmt.Fprintln(&out)
	if len(run.synthesis.CrossGroupFlows) == 0 {
		fmt.Fprintln(&out, "_None._")
	} else {
		for _, flow := range run.synthesis.CrossGroupFlows {
			fmt.Fprintf(&out, "- %s → %s: %s\n", mdCode(flow.FromGroup), mdCode(flow.ToGroup), flow.Description)
		}
	}

	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## Global Invariants")
	fmt.Fprintln(&out)
	writeStrings(&out, run.synthesis.GlobalInvariants)

	for _, groupID := range groupIDs {
		request := run.requests[groupID]
		reply := run.replies[groupID]
		fmt.Fprintln(&out)
		fmt.Fprintf(&out, "## Subsystem: %s\n\n", groupID)
		fmt.Fprintln(&out, reply.SubsystemSummary)
		fmt.Fprintln(&out)
		fmt.Fprintf(&out, "**Purpose:** %s\n", request.Purpose)

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Entry Points")
		fmt.Fprintln(&out)
		if len(reply.EntryPoints) == 0 {
			fmt.Fprintln(&out, "_None._")
		} else {
			for _, item := range reply.EntryPoints {
				fmt.Fprintf(&out, "- %s: %s\n", mdCode(item.Path), item.Description)
			}
		}

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Key Symbols")
		fmt.Fprintln(&out)
		if len(reply.KeySymbols) == 0 {
			fmt.Fprintln(&out, "_None._")
		} else {
			for _, item := range reply.KeySymbols {
				fmt.Fprintf(&out, "- %s in %s (%s): %s\n",
					mdCode(item.Name), mdCode(item.Path), item.Kind, item.Description)
			}
		}

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Dependencies")
		fmt.Fprintln(&out)
		if len(reply.Dependencies.Internal) == 0 && len(reply.Dependencies.External) == 0 {
			fmt.Fprintln(&out, "_None._")
		} else {
			for _, item := range reply.Dependencies.Internal {
				fmt.Fprintf(&out, "- Internal %s: %s\n", mdCode(item.Path), item.Description)
			}
			for _, item := range reply.Dependencies.External {
				fmt.Fprintf(&out, "- External %s: %s\n", mdCode(item.Name), item.Description)
			}
		}

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Data Flows")
		fmt.Fprintln(&out)
		if len(reply.DataFlows) == 0 {
			fmt.Fprintln(&out, "_None._")
		} else {
			for _, flow := range reply.DataFlows {
				fmt.Fprintf(&out, "- %s → %s: %s\n", mdCode(flow.From), mdCode(flow.To), flow.Description)
			}
		}

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Invariants")
		fmt.Fprintln(&out)
		writeStrings(&out, reply.Invariants)

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Tests")
		fmt.Fprintln(&out)
		if len(reply.Tests) == 0 {
			fmt.Fprintln(&out, "_None._")
		} else {
			for _, item := range reply.Tests {
				fmt.Fprintf(&out, "- %s: %s\n", mdCode(item.Path), item.Description)
			}
		}

		fmt.Fprintln(&out)
		fmt.Fprintln(&out, "### Files")
		summaries := append([]FileSummary(nil), reply.Files...)
		sort.Slice(summaries, func(i, j int) bool { return summaries[i].Path < summaries[j].Path })
		for _, file := range summaries {
			fmt.Fprintln(&out)
			fmt.Fprintf(&out, "#### %s\n\n", file.Path)
			fmt.Fprintf(&out, "_%s._ %s\n", file.ContentKind, file.Summary)
		}
	}
	return out.String()
}

func sortedGroupIDs(requests map[string]AnalyzerRequest) []string {
	groupIDs := make([]string, 0, len(requests))
	for groupID := range requests {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	return groupIDs
}

func mdCode(value string) string {
	fence := "`"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	return fence + value + fence
}

func writeStrings(out *strings.Builder, values []string) {
	if len(values) == 0 {
		fmt.Fprintln(out, "_None._")
		return
	}
	for _, value := range values {
		fmt.Fprintf(out, "- %s\n", value)
	}
}

var (
	syncPublishedDirectory = syncDirectory
	removeRunPath          = func(root *os.Root, name string) error { return root.Remove(name) }
	renderRootsOpened      func() error
)

func writeAtomic(dir *rootedDirectory, name string, data []byte) (warnings []string, returnErr error) {
	mode := os.FileMode(0o644)
	if info, err := dir.root.Lstat(name); err == nil {
		if info.Mode().IsRegular() {
			mode = info.Mode().Perm()
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	file, tmpName, err := createRootTemp(dir.root, "."+name+"-")
	if err != nil {
		return nil, err
	}
	renamed := false
	defer func() {
		if renamed {
			return
		}
		if err := dir.root.Remove(tmpName); err != nil && !errors.Is(err, os.ErrNotExist) && returnErr == nil {
			returnErr = err
		}
	}()
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := dir.root.Rename(tmpName, name); err != nil {
		return nil, err
	}
	renamed = true
	if err := syncPublishedDirectory(dir.root); err != nil {
		warnings = append(warnings, fmt.Sprintf("project published but directory durability sync failed: %v", err))
	}
	return warnings, nil
}

func createRootTemp(root *os.Root, prefix string) (*os.File, string, error) {
	var random [8]byte
	for range 100 {
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := prefix + hex.EncodeToString(random[:])
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not create temporary project file")
}

func syncDirectory(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	return errors.Join(syncErr, closeErr)
}

func cleanupRun(paths runPaths, inputFiles []*runArtifact) error {
	var cleanupErrors []error
	for _, artifact := range inputFiles {
		if err := removeUnchanged(artifact.parent, artifact.name, artifact.info, false); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	for _, dir := range []struct {
		parent *rootedDirectory
		name   string
		child  *rootedDirectory
	}{
		{paths.runRoot, "requests", paths.requestDir},
		{paths.runRoot, "replies", paths.replyDir},
		{paths.runsDir, paths.runName, paths.runRoot},
	} {
		// Retained evidence such as analyzer worksheets keeps a run
		// directory alive; a non-empty directory is not a cleanup failure.
		if err := removeUnchanged(dir.parent, dir.name, dir.child.info, true); err != nil &&
			!errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func removeUnchanged(parent *rootedDirectory, name string, want os.FileInfo, directory bool) error {
	current, err := parent.root.Lstat(name)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(current, want) {
		return fmt.Errorf("%s changed after it was opened; refusing cleanup", filepath.Join(parent.name, filepath.FromSlash(name)))
	}
	if directory != current.IsDir() {
		return fmt.Errorf("%s changed file type after it was opened; refusing cleanup", filepath.Join(parent.name, filepath.FromSlash(name)))
	}
	return removeRunPath(parent.root, name)
}
