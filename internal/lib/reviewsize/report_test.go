package reviewsize

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCompareBoundaries(t *testing.T) {
	domains := func(n int) []string { return make([]string, n) }
	for _, tc := range []struct {
		name string
		kind Kind
		diff Diff
		pick func(Report) Check
		want Status
	}{
		{"task lines 200", KindTask, Diff{SourceLines: 200}, srcLines, Within},
		{"task lines 201", KindTask, Diff{SourceLines: 201}, srcLines, OverCeiling},
		{"task files 20", KindTask, Diff{Files: 20}, files, Within},
		{"task files 21", KindTask, Diff{Files: 21}, files, OverCeiling},
		{"task domains 3", KindTask, Diff{CodeDomains: domains(3)}, codeDomains, Within},
		{"task domains 4", KindTask, Diff{CodeDomains: domains(4)}, codeDomains, OverCeiling},
		{"pr lines 200", KindPR, Diff{SourceLines: 200}, srcLines, Within},
		{"pr lines 201", KindPR, Diff{SourceLines: 201}, srcLines, OverTarget},
		{"pr lines 300", KindPR, Diff{SourceLines: 300}, srcLines, OverTarget},
		{"pr lines 301", KindPR, Diff{SourceLines: 301}, srcLines, OverCeiling},
		{"pr files 20", KindPR, Diff{Files: 20}, files, Within},
		{"pr files 21", KindPR, Diff{Files: 21}, files, OverTarget},
		{"pr files 30", KindPR, Diff{Files: 30}, files, OverTarget},
		{"pr files 31", KindPR, Diff{Files: 31}, files, OverCeiling},
		{"pr domains 3", KindPR, Diff{CodeDomains: domains(3)}, codeDomains, Within},
		{"pr domains 4", KindPR, Diff{CodeDomains: domains(4)}, codeDomains, OverTarget},
		{"pr domains 5", KindPR, Diff{CodeDomains: domains(5)}, codeDomains, OverTarget},
		{"pr domains 6", KindPR, Diff{CodeDomains: domains(6)}, codeDomains, OverCeiling},
		{"task total 750", KindTask, Diff{TotalLines: 750}, totalLines, Within},
		{"task total 751", KindTask, Diff{TotalLines: 751}, totalLines, OverCeiling},
		{"pr total 750", KindPR, Diff{TotalLines: 750}, totalLines, Within},
		{"pr total 751", KindPR, Diff{TotalLines: 751}, totalLines, OverCeiling},
	} {
		if got := tc.pick(Compare(tc.diff, tc.kind)).Status; got != tc.want {
			t.Errorf("%s: status %q, want %q", tc.name, got, tc.want)
		}
	}
}

func srcLines(r Report) Check    { return r.SourceLines }
func files(r Report) Check       { return r.Files }
func codeDomains(r Report) Check { return r.CodeDomains }
func totalLines(r Report) Check  { return r.TotalLines }

var goldenDiff = Diff{SourceLines: 250, Files: 12, CodeDomains: []string{"lib/reviewsize", "task"}, DocAreas: []string{"reference/plan"}, TotalLines: 800}

func TestReportJSONGolden(t *testing.T) {
	got, err := json.MarshalIndent(Compare(goldenDiff, KindPR), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "testdata/report.json.golden", string(got)+"\n")
}

func TestRenderGolden(t *testing.T) {
	var b strings.Builder
	Render(&b, Compare(goldenDiff, KindPR))
	Render(&b, Compare(goldenDiff, KindTask))
	assertGolden(t, "testdata/render.golden", b.String())
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("output differs from %s:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}
