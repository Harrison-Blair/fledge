package reviewsize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  Kind
		diff  Diff
		check func(Report) Check
		want  Status
	}{
		{"task lines 200", KindTask, Diff{SourceLines: 200}, sourceLines, Within},
		{"task lines 201", KindTask, Diff{SourceLines: 201}, sourceLines, OverCeiling},
		{"pr lines 200", KindPR, Diff{SourceLines: 200}, sourceLines, Within},
		{"pr lines 201", KindPR, Diff{SourceLines: 201}, sourceLines, OverTarget},
		{"pr lines 300", KindPR, Diff{SourceLines: 300}, sourceLines, OverTarget},
		{"pr lines 301", KindPR, Diff{SourceLines: 301}, sourceLines, OverCeiling},
		{"task files 20", KindTask, Diff{Files: 20}, files, Within},
		{"task files 21", KindTask, Diff{Files: 21}, files, OverCeiling},
		{"pr files 21", KindPR, Diff{Files: 21}, files, OverTarget},
		{"pr files 30", KindPR, Diff{Files: 30}, files, OverTarget},
		{"pr files 31", KindPR, Diff{Files: 31}, files, OverCeiling},
		{"task domains 3", KindTask, Diff{CodeDomains: make([]string, 3)}, domains, Within},
		{"task domains 4", KindTask, Diff{CodeDomains: make([]string, 4)}, domains, OverCeiling},
		{"pr domains 4", KindPR, Diff{CodeDomains: make([]string, 4)}, domains, OverTarget},
		{"pr domains 5", KindPR, Diff{CodeDomains: make([]string, 5)}, domains, OverTarget},
		{"pr domains 6", KindPR, Diff{CodeDomains: make([]string, 6)}, domains, OverCeiling},
		{"task total 750", KindTask, Diff{TotalLines: 750}, total, Within},
		{"task total 751", KindTask, Diff{TotalLines: 751}, total, OverCeiling},
		{"pr total 750", KindPR, Diff{TotalLines: 750}, total, Within},
		{"pr total 751", KindPR, Diff{TotalLines: 751}, total, OverCeiling},
	} {
		if got := tc.check(Compare(tc.diff, tc.kind)).Status; got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func sourceLines(r Report) Check { return r.SourceLines }
func files(r Report) Check       { return r.Files }
func domains(r Report) Check     { return r.CodeDomains }
func total(r Report) Check       { return r.TotalLines }

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("output differs from %s:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

var goldenDiff = Diff{SourceLines: 234, Files: 14, CodeDomains: []string{"agent", "cmd root"}, DocAreas: []string{"reference/usage"}, TotalLines: 810}

func TestRender(t *testing.T) {
	var b strings.Builder
	Render(&b, Compare(goldenDiff, KindPR))
	golden(t, "render.golden", b.String())
}

func TestReportJSON(t *testing.T) {
	b, err := json.MarshalIndent(Compare(goldenDiff, KindTask), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "report.json.golden", string(b)+"\n")
}
