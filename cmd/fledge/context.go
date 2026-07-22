package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/ignore"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
	"github.com/Harrison-Blair/fledge/internal/scan"
	"github.com/Harrison-Blair/fledge/internal/workspace"
)

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
