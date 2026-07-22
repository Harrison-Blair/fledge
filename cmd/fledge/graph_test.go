package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/workspace"
)

func writeSizedFile(t *testing.T, root, name string, size int) {
	t.Helper()
	name = filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func graphWorkspace(t *testing.T) string {
	t.Helper()
	root, _ := scaffoldedWorkspace(t)
	writeSizedFile(t, root, "root.txt", 1024)
	writeSizedFile(t, root, "alpha/a.txt", 1)
	writeSizedFile(t, root, "alpha/nested/z.txt", 2)
	writeSizedFile(t, root, "beta/b.txt", 4)
	return root
}

func graphJSON(t *testing.T, args ...string) contextGraph {
	t.Helper()
	cmd := append([]string{"context", "graph"}, args...)
	cmd = append(cmd, "--json")
	out, err := captureRun(t, cmd...)
	if err != nil {
		t.Fatalf("context graph: %v", err)
	}
	var got contextGraph
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return got
}

func TestContextGraphHumanTree(t *testing.T) {
	root := graphWorkspace(t)
	t.Chdir(root)

	out, err := captureRun(t, "context", "graph")
	if err != nil {
		t.Fatal(err)
	}
	want := `./ (1.0K, 4 files)
├── alpha/ (3B, 2 files)
│   ├── nested/ (2B, 1 file)
│   │   └── z.txt (2B)
│   └── a.txt (1B)
├── beta/ (4B, 1 file)
│   └── b.txt (4B)
└── root.txt (1.0K)
`
	if out != want {
		t.Errorf("graph output:\n%s\nwant:\n%s", out, want)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("graph output contains ANSI escapes: %q", out)
	}
}

func TestContextGraphJSONSchemaTotalsAndOrder(t *testing.T) {
	root := graphWorkspace(t)
	t.Chdir(root)

	got := graphJSON(t)
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != canonical || got.Scope != "." {
		t.Fatalf("root, scope = %q, %q; want %q, .", got.Root, got.Scope, canonical)
	}

	var nodeRows []any
	for _, node := range got.Nodes {
		count := any(nil)
		if node.FileCount != nil {
			count = *node.FileCount
		}
		nodeRows = append(nodeRows, []any{node.Path, node.Kind, node.Size, count})
	}
	wantNodes := []any{
		[]any{".", "directory", int64(1031), 4},
		[]any{"alpha", "directory", int64(3), 2},
		[]any{"alpha/nested", "directory", int64(2), 1},
		[]any{"alpha/nested/z.txt", "file", int64(2), nil},
		[]any{"alpha/a.txt", "file", int64(1), nil},
		[]any{"beta", "directory", int64(4), 1},
		[]any{"beta/b.txt", "file", int64(4), nil},
		[]any{"root.txt", "file", int64(1024), nil},
	}
	if !reflect.DeepEqual(nodeRows, wantNodes) {
		t.Errorf("nodes = %#v, want %#v", nodeRows, wantNodes)
	}

	var edgeRows [][]string
	for _, edge := range got.Edges {
		edgeRows = append(edgeRows, []string{edge.From, edge.To, edge.Relation})
	}
	wantEdges := [][]string{
		{".", "alpha", "contains"},
		{"alpha", "alpha/nested", "contains"},
		{"alpha/nested", "alpha/nested/z.txt", "contains"},
		{"alpha", "alpha/a.txt", "contains"},
		{".", "beta", "contains"},
		{"beta", "beta/b.txt", "contains"},
		{".", "root.txt", "contains"},
	}
	if !reflect.DeepEqual(edgeRows, wantEdges) {
		t.Errorf("edges = %#v, want %#v", edgeRows, wantEdges)
	}
}

func TestContextGraphDefaultAndExplicitScopes(t *testing.T) {
	root := graphWorkspace(t)
	t.Chdir(filepath.Join(root, "alpha", "nested"))

	whole := graphJSON(t)
	if whole.Scope != "." || whole.Nodes[0].Path != "." || whole.Nodes[0].FileCount == nil || *whole.Nodes[0].FileCount != 4 {
		t.Fatalf("default graph = %+v", whole)
	}

	subtree := graphJSON(t, filepath.Join(root, "alpha"))
	paths := make([]string, len(subtree.Nodes))
	for i, node := range subtree.Nodes {
		paths[i] = node.Path
	}
	want := []string{"alpha", "alpha/nested", "alpha/nested/z.txt", "alpha/a.txt"}
	if subtree.Scope != "alpha" || !reflect.DeepEqual(paths, want) {
		t.Fatalf("explicit graph scope, paths = %q, %q; want alpha, %q", subtree.Scope, paths, want)
	}
}

func TestContextGraphUsesScanIgnoreSemantics(t *testing.T) {
	root := graphWorkspace(t)
	writeSizedFile(t, root, "ignored/hidden.txt", 100)
	writeSizedFile(t, root, "ignored/keep.txt", 200)
	writeSizedFile(t, root, "visible/drop.tmp", 100)
	writeSizedFile(t, root, "visible/keep.tmp", 200)
	if err := os.WriteFile(filepath.Join(root, ".fledge", "graph-ignore"), []byte("*.tmp\n!visible/keep.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appendIgnore(t, root, "ignored/\n!ignored/keep.txt\n#include .fledge/graph-ignore\n")
	t.Chdir(root)

	got := graphJSON(t)
	for _, node := range got.Nodes {
		if strings.HasPrefix(node.Path, "ignored") {
			t.Fatalf("ignored pruned path appears in graph: %+v", node)
		}
		if node.Path == "visible/drop.tmp" {
			t.Fatalf("included ignore pattern did not exclude %q", node.Path)
		}
	}
	if got.Nodes[0].Size != 1231 || got.Nodes[0].FileCount == nil || *got.Nodes[0].FileCount != 5 {
		t.Fatalf("ignored files affected totals: %+v", got.Nodes[0])
	}
}

func TestContextGraphAlwaysEmitsEmptySelectedScope(t *testing.T) {
	root, _ := scaffoldedWorkspace(t)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	got := graphJSON(t, "empty")
	if len(got.Nodes) != 1 || got.Nodes[0].Path != "empty" || got.Nodes[0].Kind != "directory" ||
		got.Nodes[0].Size != 0 || got.Nodes[0].FileCount == nil || *got.Nodes[0].FileCount != 0 {
		t.Fatalf("empty graph nodes = %+v", got.Nodes)
	}
	if got.Edges == nil || len(got.Edges) != 0 {
		t.Fatalf("empty graph edges = %#v, want non-nil empty slice", got.Edges)
	}

	out, err := captureRun(t, "context", "graph", "empty")
	if err != nil || out != "empty/ (0B, 0 files)\n" {
		t.Fatalf("empty human graph = %q, %v", out, err)
	}
}

func TestContextGraphFullyIgnoredScopeStillHasRoot(t *testing.T) {
	root := graphWorkspace(t)
	appendIgnore(t, root, "alpha/\n")
	t.Chdir(root)

	got := graphJSON(t, "alpha")
	if len(got.Nodes) != 1 || got.Nodes[0].Path != "alpha" || got.Nodes[0].FileCount == nil || *got.Nodes[0].FileCount != 0 {
		t.Fatalf("fully ignored graph = %+v", got)
	}
}

func TestContextGraphCLIValidationAndHelp(t *testing.T) {
	for _, args := range [][]string{
		{"context", "graph", "--unknown"},
		{"context", "graph", "one", "two"},
	} {
		_, err := captureRun(t, args...)
		if err == nil || !strings.Contains(err.Error(), helpPages["context graph"]) {
			t.Errorf("%q error = %v, want graph help", args, err)
		}
	}

	for _, args := range [][]string{
		{"context", "graph", "--help"},
		{"context", "help", "graph"},
		{"help", "context", "graph"},
	} {
		out, err := captureRun(t, args...)
		if err != nil || out != helpPages["context graph"] {
			t.Errorf("%q output, error = %q, %v", args, out, err)
		}
	}
}

func TestContextGraphOutsideWorkspaceErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := captureRun(t, "context", "graph")
	if !errors.Is(err, workspace.ErrNotFound) {
		t.Fatalf("err = %v, want workspace.ErrNotFound", err)
	}
}
