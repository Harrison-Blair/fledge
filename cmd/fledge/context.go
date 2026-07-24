package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/contextdoc"
	"github.com/Harrison-Blair/fledge/internal/ignore"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/scan"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

func runContextCompose(args []string) error {
	if len(args) == 0 {
		return printHelp("context compose")
	}
	if isHelpFlag(args[0]) {
		return printHelp("context compose")
	}
	if args[0] == "help" {
		return runHelp("context compose", args[1:])
	}
	switch args[0] {
	case "analyzer-request":
		return runContextComposeAnalyzerRequest(args[1:])
	case "worksheet":
		return runContextComposeWorksheet(args[1:])
	default:
		return usageErrorf("context compose", "unknown context compose subcommand %q", args[0])
	}
}

func runContextComposeAnalyzerRequest(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context compose analyzer-request")
	}
	inPlace, args := takeBoolFlag(args, "--in-place", "-A")
	worksheetPath, args, err := takeFlag(args, "--worksheet", "-E")
	if err != nil {
		return usageErrorFor("context compose analyzer-request", err)
	}
	if err := rejectFlags("context compose analyzer-request", args); err != nil {
		return usageErrorFor("context compose analyzer-request", err)
	}
	if len(args) == 0 {
		return usageErrorf("context compose analyzer-request",
			"context compose analyzer-request: FILE is required")
	}
	if len(args) > 1 {
		return usageErrorf("context compose analyzer-request",
			"context compose analyzer-request: unexpected argument %q", args[1])
	}
	name := args[0]

	root, err := workspaceRoot()
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %w", err)
	}
	templateName := filepath.Join(root, scaffold.DirName, filepath.FromSlash(scaffold.ContextRequestTemplateName))
	templateData, err := os.ReadFile(templateName)
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: analyzer request template %s: %w; run \"fledge init\" to scaffold it", templateName, err)
	}
	template, err := contextdoc.ParseRequestTemplate(templateData)
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %s: %w", templateName, err)
	}

	request, err := contextdoc.LoadAnalyzerRequest(name)
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %w", err)
	}
	composed, err := contextdoc.ComposeAnalyzerRequest(request, template, worksheetPath)
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %s: %w", templateName, err)
	}
	data, err := json.MarshalIndent(composed, "", "  ")
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %w", err)
	}
	data = append(data, '\n')
	if err := contextdoc.ValidateComposedAnalyzerRequest(data); err != nil {
		return fmt.Errorf("context compose analyzer-request: composed request: %w", err)
	}

	if !inPlace {
		_, err := os.Stdout.Write(data)
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+"-*")
	if err != nil {
		return fmt.Errorf("context compose analyzer-request: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, name)
	}
	if err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("context compose analyzer-request: %w", err)
	}
	return nil
}

func runContextComposeWorksheet(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context compose worksheet")
	}
	outputPath, args, err := takeFlag(args, "--output", "-U")
	if err != nil {
		return usageErrorFor("context compose worksheet", err)
	}
	if err := rejectFlags("context compose worksheet", args); err != nil {
		return usageErrorFor("context compose worksheet", err)
	}
	if len(args) == 0 {
		return usageErrorf("context compose worksheet",
			"context compose worksheet: REQUEST is required")
	}
	if len(args) > 1 {
		return usageErrorf("context compose worksheet",
			"context compose worksheet: unexpected argument %q", args[1])
	}

	root, err := workspaceRoot()
	if err != nil {
		return fmt.Errorf("context compose worksheet: %w", err)
	}
	templateName := filepath.Join(root, scaffold.DirName, filepath.FromSlash(scaffold.ContextWorksheetTemplateName))
	templateData, err := os.ReadFile(templateName)
	if err != nil {
		return fmt.Errorf("context compose worksheet: analyzer worksheet template %s: %w; run \"fledge init\" to scaffold it", templateName, err)
	}
	request, err := contextdoc.LoadAnalyzerRequest(args[0])
	if err != nil {
		return fmt.Errorf("context compose worksheet: %w", err)
	}
	worksheet, err := contextdoc.ComposeWorksheet(request, templateData)
	if err != nil {
		return fmt.Errorf("context compose worksheet: %s: %w", templateName, err)
	}

	if outputPath == "" {
		_, err := os.Stdout.WriteString(worksheet)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("context compose worksheet: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+"-*")
	if err != nil {
		return fmt.Errorf("context compose worksheet: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0o644); err == nil {
		_, err = tmp.WriteString(worksheet)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, outputPath)
	}
	if err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("context compose worksheet: %w", err)
	}
	return nil
}

// scannedContext is the common workspace view used by context commands.
// Scope and every file path are slash-separated and workspace-relative.
type scannedContext struct {
	Root  string
	Scope string
	Files []scan.File
}

// scanContext resolves the workspace, applies its one ignore matcher to a
// full scan, and optionally narrows the result to start's subtree. Filtering
// the full scan preserves scan's pruning rule: a file beneath an ignored
// directory cannot be brought back by selecting that directory explicitly.
func scanContext(start string, explicit bool) (scannedContext, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return scannedContext{}, err
	}
	// FindRoot canonicalizes the root. Canonicalize the selected directory too
	// so their relative spelling compares cleanly through symlinks.
	dir, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return scannedContext{}, err
	}
	root, err := workspace.FindRoot(dir)
	if err != nil {
		return scannedContext{}, err
	}

	m, err := ignore.ParseFile(filepath.Join(root, scaffold.DirName, scaffold.IgnoreName), root)
	if err != nil {
		return scannedContext{}, err
	}
	files, err := scan.Files(root, m)
	if err != nil {
		return scannedContext{}, err
	}
	if files == nil {
		files = []scan.File{}
	}

	scope := "."
	if explicit {
		scope, err = filepath.Rel(root, dir)
		if err != nil {
			return scannedContext{}, err
		}
		scope = filepath.ToSlash(scope)
	}
	if scope != "." {
		prefix := scope + "/"
		kept := files[:0]
		for _, file := range files {
			if strings.HasPrefix(file.Path, prefix) {
				kept = append(kept, file)
			}
		}
		files = kept
	}

	return scannedContext{Root: root, Scope: scope, Files: files}, nil
}

type graphNode struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Size      int64  `json:"size"`
	FileCount *int   `json:"file_count,omitempty"`
}

type graphEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

type contextGraph struct {
	Root  string      `json:"root"`
	Scope string      `json:"scope"`
	Nodes []graphNode `json:"nodes"`
	Edges []graphEdge `json:"edges"`
}

type graphDir struct {
	path      string
	dirs      map[string]*graphDir
	files     []scan.File
	size      int64
	fileCount int
}

func runContextGraph(args []string) error {
	if hasHelpFlag(args) {
		return printHelp("context graph")
	}
	asJSON, args := takeBoolFlag(args, "--json", "-J")
	if err := rejectFlags("context graph", args); err != nil {
		return usageErrorFor("context graph", err)
	}
	if len(args) > 1 {
		return usageErrorf("context graph", "context graph: unexpected argument %q", args[1])
	}

	start := "."
	if len(args) == 1 {
		start = args[0]
	}
	context, err := scanContext(start, len(args) == 1)
	if err != nil {
		return err
	}
	graph, tree := buildContextGraph(context)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(graph)
	}
	printContextGraph(tree)
	return nil
}

func buildContextGraph(context scannedContext) (contextGraph, *graphDir) {
	root := &graphDir{path: context.Scope, dirs: make(map[string]*graphDir)}
	for _, file := range context.Files {
		rel := file.Path
		if context.Scope != "." {
			rel = strings.TrimPrefix(rel, context.Scope+"/")
		}
		parts := strings.Split(rel, "/")
		dir := root
		for _, name := range parts[:len(parts)-1] {
			child := dir.dirs[name]
			if child == nil {
				childPath := path.Join(dir.path, name)
				if dir.path == "." {
					childPath = name
				}
				child = &graphDir{path: childPath, dirs: make(map[string]*graphDir)}
				dir.dirs[name] = child
			}
			dir = child
		}
		dir.files = append(dir.files, file)
	}
	measureGraphDir(root)

	graph := contextGraph{
		Root:  context.Root,
		Scope: context.Scope,
		Nodes: make([]graphNode, 0, len(context.Files)+1),
		Edges: make([]graphEdge, 0),
	}
	appendGraphDir(&graph, root)
	return graph, root
}

func measureGraphDir(dir *graphDir) {
	for _, child := range dir.dirs {
		measureGraphDir(child)
		dir.size += child.size
		dir.fileCount += child.fileCount
	}
	for _, file := range dir.files {
		dir.size += file.Size
		dir.fileCount++
	}
}

func appendGraphDir(graph *contextGraph, dir *graphDir) {
	fileCount := dir.fileCount
	graph.Nodes = append(graph.Nodes, graphNode{
		Path: dir.path, Kind: "directory", Size: dir.size, FileCount: &fileCount,
	})

	for _, name := range sortedGraphDirNames(dir) {
		child := dir.dirs[name]
		graph.Edges = append(graph.Edges, graphEdge{dir.path, child.path, "contains"})
		appendGraphDir(graph, child)
	}
	sort.Slice(dir.files, func(i, j int) bool { return dir.files[i].Path < dir.files[j].Path })
	for _, file := range dir.files {
		graph.Edges = append(graph.Edges, graphEdge{dir.path, file.Path, "contains"})
		graph.Nodes = append(graph.Nodes, graphNode{Path: file.Path, Kind: "file", Size: file.Size})
	}
}

func sortedGraphDirNames(dir *graphDir) []string {
	names := make([]string, 0, len(dir.dirs))
	for name := range dir.dirs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func printContextGraph(root *graphDir) {
	fmt.Printf("%s (%s, %s)\n", graphDirLabel(root.path), humanSize(root.size), fileCountLabel(root.fileCount))
	printGraphChildren(root, "")
}

func printGraphChildren(dir *graphDir, prefix string) {
	type child struct {
		dir  *graphDir
		file *scan.File
	}
	children := make([]child, 0, len(dir.dirs)+len(dir.files))
	for _, name := range sortedGraphDirNames(dir) {
		children = append(children, child{dir: dir.dirs[name]})
	}
	for i := range dir.files {
		children = append(children, child{file: &dir.files[i]})
	}

	for i, child := range children {
		last := i == len(children)-1
		connector, continuation := "├── ", "│   "
		if last {
			connector, continuation = "└── ", "    "
		}
		if child.dir != nil {
			fmt.Printf("%s%s%s (%s, %s)\n", prefix, connector, graphDirLabel(child.dir.path), humanSize(child.dir.size), fileCountLabel(child.dir.fileCount))
			printGraphChildren(child.dir, prefix+continuation)
			continue
		}
		fmt.Printf("%s%s%s (%s)\n", prefix, connector, path.Base(child.file.Path), humanSize(child.file.Size))
	}
}

func graphDirLabel(name string) string {
	if name == "." {
		return "./"
	}
	return path.Base(name) + "/"
}

func fileCountLabel(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}
