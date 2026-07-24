package contextdoc

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestValidateAnalyzerRequestRejectsMalformedAndDuplicateData(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "malformed",
			json: `{"schema_version":1`,
			want: "EOF",
		},
		{
			name: "duplicate object field",
			json: `{"schema_version":1,"schema_version":1,"group_id":"core","purpose":"Core","total_size":1,"files":[{"path":"a.go","size":1}]}`,
			want: `duplicate object field "schema_version"`,
		},
		{
			name: "missing required zero field",
			json: `{"schema_version":1,"group_id":"core","purpose":"Core","files":[{"path":"a.go","size":1}]}`,
			want: `missing required field "total_size"`,
		},
		{
			name: "duplicate path",
			json: `{"schema_version":1,"group_id":"core","purpose":"Core","total_size":2,"files":[{"path":"a.go","size":1},{"path":"a.go","size":1}]}`,
			want: `duplicate path "a.go"`,
		},
		{
			name: "unsafe path",
			json: `{"schema_version":1,"group_id":"core","purpose":"Core","total_size":1,"files":[{"path":"../a.go","size":1}]}`,
			want: "safe normalized relative path",
		},
		{
			name: "bad total",
			json: `{"schema_version":1,"group_id":"core","purpose":"Core","total_size":2,"files":[{"path":"a.go","size":1}]}`,
			want: "want 1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "request.json")
			if err := os.WriteFile(name, []byte(test.json), 0o644); err != nil {
				t.Fatal(err)
			}
			err := ValidateAnalyzerRequestFile(name)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAnalyzerRequestFile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateAnalyzerRequestEnforcesGroupingBounds(t *testing.T) {
	tests := []struct {
		name  string
		files []File
		want  string
	}{
		{
			name:  "fifty files allowed",
			files: numberedFiles(50, 1),
		},
		{
			name:  "fifty one files rejected",
			files: numberedFiles(51, 1),
			want:  "maximum is 50",
		},
		{
			name:  "byte limit allowed",
			files: []File{{Path: "a.go", Size: MaxRequestBytes}},
		},
		{
			name:  "oversized singleton allowed",
			files: []File{{Path: "large.bin", Size: MaxRequestBytes + 1}},
		},
		{
			name: "oversized group rejected",
			files: []File{
				{Path: "a.bin", Size: MaxRequestBytes},
				{Path: "b.bin", Size: 1},
			},
			want: "not an oversized singleton",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var total int64
			for _, file := range test.files {
				total += file.Size
			}
			name := filepath.Join(t.TempDir(), "request.json")
			writeJSON(t, name, AnalyzerRequest{
				SchemaVersion: 1,
				GroupID:       "bounded",
				Purpose:       "Exercise grouping bounds.",
				TotalSize:     total,
				Files:         test.files,
			})
			err := ValidateAnalyzerRequestFile(name)
			if test.want == "" && err != nil {
				t.Fatalf("ValidateAnalyzerRequestFile() error = %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateAnalyzerRequestFile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func numberedFiles(count int, size int64) []File {
	files := make([]File, count)
	for i := range files {
		files[i] = File{Path: "file-" + strconv.Itoa(i) + ".go", Size: size}
	}
	return files
}

func TestValidateAnalyzerReplyCorrelatesRequestAndNonText(t *testing.T) {
	dir := t.TempDir()
	requestName := filepath.Join(dir, "request.json")
	writeJSON(t, requestName, AnalyzerRequest{
		SchemaVersion: 1,
		GroupID:       "core",
		Purpose:       "Core behavior",
		TotalSize:     3,
		Files:         []File{{Path: "a.go", Size: 1}, {Path: "image.png", Size: 2}},
	})

	valid := AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "core",
		SubsystemSummary: "Core subsystem.",
		EntryPoints:      []EntryPoint{{Path: "a.go", Description: "Starts here."}},
		KeySymbols:       []KeySymbol{},
		Dependencies:     Dependencies{Internal: []InternalDependency{}, External: []ExternalDependency{}},
		DataFlows:        []DataFlow{},
		Invariants:       []string{},
		Tests:            []TestReference{},
		Files: []FileSummary{
			{Path: "a.go", ContentKind: "text", Summary: "Go source."},
			{Path: "image.png", ContentKind: "non-text", Summary: "PNG image metadata."},
		},
	}
	replyName := filepath.Join(dir, "reply.json")
	writeJSON(t, replyName, valid)
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err != nil {
		t.Fatalf("valid reply: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*AnalyzerSuccessReply)
		want   string
	}{
		{
			name: "group mismatch",
			mutate: func(reply *AnalyzerSuccessReply) {
				reply.GroupID = "other"
			},
			want: "does not match request",
		},
		{
			name: "missing file",
			mutate: func(reply *AnalyzerSuccessReply) {
				reply.Files = reply.Files[:1]
			},
			want: `missing assigned path "image.png"`,
		},
		{
			name: "duplicate file",
			mutate: func(reply *AnalyzerSuccessReply) {
				reply.Files[1] = reply.Files[0]
			},
			want: `duplicate path "a.go"`,
		},
		{
			name: "extra file",
			mutate: func(reply *AnalyzerSuccessReply) {
				reply.Files = append(reply.Files, FileSummary{Path: "extra.go", ContentKind: "text", Summary: "Extra."})
			},
			want: "not assigned by the request",
		},
		{
			name: "non-text symbol reference",
			mutate: func(reply *AnalyzerSuccessReply) {
				reply.KeySymbols = []KeySymbol{{Path: "image.png", Name: "pixel", Kind: "data", Description: "No."}}
			},
			want: "refers to a non-text file",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reply := valid
			reply.Files = append([]FileSummary(nil), valid.Files...)
			test.mutate(&reply)
			writeJSON(t, replyName, reply)
			err := ValidateAnalyzerReplyFile(replyName, requestName)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAnalyzerReplyFile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateAnalyzerReplyAllowsSafeUnassignedInternalDependency(t *testing.T) {
	dir := t.TempDir()
	requestName := filepath.Join(dir, "request.json")
	writeJSON(t, requestName, AnalyzerRequest{
		SchemaVersion: 1, GroupID: "core", Purpose: "Core", TotalSize: 1,
		Files: []File{{Path: "a.go", Size: 1}},
	})
	replyName := filepath.Join(dir, "reply.json")
	reply := AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "core",
		SubsystemSummary: "Core.",
		EntryPoints:      []EntryPoint{},
		KeySymbols:       []KeySymbol{},
		Dependencies: Dependencies{
			Internal: []InternalDependency{{Path: "internal/scaffold", Description: "Referenced package."}},
			External: []ExternalDependency{},
		},
		DataFlows:  []DataFlow{},
		Invariants: []string{},
		Tests:      []TestReference{},
		Files:      []FileSummary{{Path: "a.go", ContentKind: "text", Summary: "Source."}},
	}
	writeJSON(t, replyName, reply)
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err != nil {
		t.Fatalf("safe unassigned internal dependency: %v", err)
	}

	reply.Files = []FileSummary{}
	writeJSON(t, replyName, reply)
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err == nil ||
		!strings.Contains(err.Error(), `files is missing assigned path "a.go"`) {
		t.Fatalf("exact file coverage: %v", err)
	}

	reply.Files = []FileSummary{{Path: "a.go", ContentKind: "text", Summary: "Source."}}
	reply.Dependencies.Internal[0].Path = "../internal/scaffold"
	writeJSON(t, replyName, reply)
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err == nil ||
		!strings.Contains(err.Error(), "safe normalized relative path") {
		t.Fatalf("traversal dependency path: %v", err)
	}
}

func TestValidateAnalyzerErrorReply(t *testing.T) {
	dir := t.TempDir()
	requestName := filepath.Join(dir, "request.json")
	writeJSON(t, requestName, AnalyzerRequest{
		SchemaVersion: 1, GroupID: "core", Purpose: "Core", TotalSize: 1,
		Files: []File{{Path: "a.go", Size: 1}},
	})
	replyName := filepath.Join(dir, "reply.json")
	writeJSON(t, replyName, AnalyzerErrorReply{
		SchemaVersion: 1,
		Status:        "error",
		GroupID:       "core",
		Errors:        []AnalyzerError{{Path: "a.go", Code: "read-failed", Message: "could not read"}},
	})
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err != nil {
		t.Fatalf("valid error reply: %v", err)
	}

	writeJSON(t, replyName, AnalyzerErrorReply{
		SchemaVersion: 1,
		Status:        "error",
		GroupID:       "core",
		Errors:        []AnalyzerError{{Path: "other.go", Code: "read-failed", Message: "could not read"}},
	})
	if err := ValidateAnalyzerReplyFile(replyName, requestName); err == nil ||
		!strings.Contains(err.Error(), "not assigned") {
		t.Fatalf("unassigned error path: %v", err)
	}
}

func TestValidateAnalyzerReplyRejectsEmptySemanticFields(t *testing.T) {
	dir := t.TempDir()
	requestName := filepath.Join(dir, "request.json")
	writeJSON(t, requestName, AnalyzerRequest{
		SchemaVersion: 1, GroupID: "core", Purpose: "Core", TotalSize: 1,
		Files: []File{{Path: "a.go", Size: 1}},
	})
	base := AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "core",
		SubsystemSummary: "Core.",
		EntryPoints:      []EntryPoint{{Path: "a.go", Description: "Entry."}},
		KeySymbols:       []KeySymbol{{Path: "a.go", Name: "Run", Kind: "function", Description: "Runs."}},
		Dependencies: Dependencies{
			Internal: []InternalDependency{{Path: "a.go", Description: "Internal."}},
			External: []ExternalDependency{{Name: "stdlib", Description: "External."}},
		},
		DataFlows:  []DataFlow{{From: "input", To: "output", Description: "Flows."}},
		Invariants: []string{"Stable."},
		Tests:      []TestReference{{Path: "a.go", Description: "Test."}},
		Files:      []FileSummary{{Path: "a.go", ContentKind: "text", Summary: "Source."}},
	}
	tests := []struct {
		name   string
		mutate func(*AnalyzerSuccessReply)
		want   string
	}{
		{"entry description", func(r *AnalyzerSuccessReply) { r.EntryPoints[0].Description = " " }, "entry_points[0]"},
		{"symbol name", func(r *AnalyzerSuccessReply) { r.KeySymbols[0].Name = "" }, "key_symbols[0]"},
		{"internal description", func(r *AnalyzerSuccessReply) { r.Dependencies.Internal[0].Description = "" }, "dependencies.internal[0]"},
		{"external name", func(r *AnalyzerSuccessReply) { r.Dependencies.External[0].Name = "" }, "dependencies.external[0]"},
		{"flow endpoint", func(r *AnalyzerSuccessReply) { r.DataFlows[0].To = " " }, "data_flows[0]"},
		{"invariant", func(r *AnalyzerSuccessReply) { r.Invariants[0] = "" }, "invariants[0]"},
		{"test description", func(r *AnalyzerSuccessReply) { r.Tests[0].Description = "" }, "tests[0]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reply := cloneJSON(t, base)
			test.mutate(&reply)
			replyName := filepath.Join(dir, strings.ReplaceAll(test.name, " ", "-")+".json")
			writeJSON(t, replyName, reply)
			err := ValidateAnalyzerReplyFile(replyName, requestName)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateAnalyzerReplyFile() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRenderProjectRendersAllSectionsAndCleansRun(t *testing.T) {
	root, runDir := validRun(t)
	generatedAt := time.Date(2026, 7, 23, 14, 5, 6, 0, time.FixedZone("offset", -4*60*60))
	result, err := renderProject(runDir, generatedAt)
	if err != nil {
		t.Fatalf("renderProject: %v", err)
	}
	if result.Path != ".fledge/context/project.md" || len(result.SHA256) != 64 ||
		result.ProvenancePath != ".fledge/context/provenance.json" || len(result.Warnings) != 0 {
		t.Fatalf("render result = %+v", result)
	}

	documentName := filepath.Join(root, ".fledge", "context", "project.md")
	data, err := os.ReadFile(documentName)
	if err != nil {
		t.Fatal(err)
	}
	document := string(data)
	for _, want := range []string{
		"# Project Context",
		"2026-07-23 18:05:06 UTC",
		"## Project Overview",
		"## Routing",
		"## Cross-Group Flows",
		"## Global Invariants",
		"## Subsystem: core",
		"### Entry Points",
		"### Data Flows",
		"### Invariants",
		"#### a.go",
		"#### image.png",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("project.md missing %q\n%s", want, document)
		}
	}
	if strings.Contains(document, "## Provenance") || strings.Contains(document, "forager-emperor") {
		t.Errorf("project.md still renders provenance:\n%s", document)
	}

	provenanceData, err := os.ReadFile(filepath.Join(root, ".fledge", "context", "provenance.json"))
	if err != nil {
		t.Fatalf("published provenance missing: %v", err)
	}
	var published Provenance
	if err := decodeExact(provenanceData, &published); err != nil {
		t.Fatalf("published provenance is not an exact provenance object: %v\n%s", err, provenanceData)
	}
	if published.Forager.Name != "forager-emperor" || len(published.Analyzers) != 1 ||
		published.Analyzers[0].Name != "analyzer-adelie" {
		t.Fatalf("published provenance = %+v", published)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("run directory should be removed after success, stat error = %v", err)
	}
}

func TestRenderProjectRetainsRunDirectoryWithWorksheets(t *testing.T) {
	_, runDir := validRun(t)
	worksheetDir := filepath.Join(runDir, "worksheets")
	if err := os.MkdirAll(worksheetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	worksheetName := filepath.Join(worksheetDir, "core.md")
	if err := os.WriteFile(worksheetName, []byte("# worksheet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := renderProject(runDir, time.Now())
	if err != nil {
		t.Fatalf("renderProject: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("retained worksheets produced warnings: %q", result.Warnings)
	}
	if _, err := os.Stat(worksheetName); err != nil {
		t.Fatalf("worksheet was not retained: %v", err)
	}
}

func TestRenderProjectCorrelatesInternalDependenciesWithScan(t *testing.T) {
	t.Run("unassigned scanned directory prefix", func(t *testing.T) {
		_, runDir := validRun(t)
		splitRunOwnership(t, runDir)

		const dependencyFile = "internal/scaffold/scaffold.go"
		var scan Scan
		scanName := filepath.Join(runDir, "scan.json")
		readJSON(t, scanName, &scan)
		scan.Files[1].Path = dependencyFile
		writeJSON(t, scanName, scan)

		var request AnalyzerRequest
		requestName := filepath.Join(runDir, "requests", "assets.json")
		readJSON(t, requestName, &request)
		request.Files[0].Path = dependencyFile
		writeJSON(t, requestName, request)

		var assetReply AnalyzerSuccessReply
		assetReplyName := filepath.Join(runDir, "replies", "assets.json")
		readJSON(t, assetReplyName, &assetReply)
		assetReply.Files[0].Path = dependencyFile
		writeJSON(t, assetReplyName, assetReply)

		var synthesis Synthesis
		synthesisName := filepath.Join(runDir, "synthesis.json")
		readJSON(t, synthesisName, &synthesis)
		synthesis.Routing = []Routing{
			{PathPrefix: "a.go", GroupID: "core", Guidance: "Route core work here."},
			{PathPrefix: "internal/scaffold", GroupID: "assets", Guidance: "Route scaffold work here."},
		}
		writeJSON(t, synthesisName, synthesis)

		var reply AnalyzerSuccessReply
		replyName := filepath.Join(runDir, "replies", "core.json")
		readJSON(t, replyName, &reply)
		reply.Dependencies.Internal = []InternalDependency{{
			Path: "internal/scaffold", Description: "Package owned by another analyzer.",
		}}
		writeJSON(t, replyName, reply)

		if _, err := renderProject(runDir, time.Now()); err != nil {
			t.Fatalf("renderProject() error = %v", err)
		}
	})

	t.Run("absent dependency", func(t *testing.T) {
		_, runDir := validRun(t)
		var reply AnalyzerSuccessReply
		replyName := filepath.Join(runDir, "replies", "core.json")
		readJSON(t, replyName, &reply)
		reply.Dependencies.Internal[0].Path = "internal/invented"
		writeJSON(t, replyName, reply)

		if _, err := renderProject(runDir, time.Now()); err == nil ||
			!strings.Contains(err.Error(), `path "internal/invented" matches no scanned path`) {
			t.Fatalf("renderProject() error = %v", err)
		}
	})
}

func TestRenderProjectRejectsUnconfinedAndSymlinkedInputs(t *testing.T) {
	t.Run("outside runs directory", func(t *testing.T) {
		root, runDir := validRun(t)
		outside := filepath.Join(root, ".fledge", "context", "not-runs", "run-1")
		if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(runDir, outside); err != nil {
			t.Fatal(err)
		}
		if _, err := renderProject(outside, time.Now()); err == nil ||
			!strings.Contains(err.Error(), "strictly beneath") {
			t.Fatalf("renderProject() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(outside, "scan.json")); err != nil {
			t.Fatalf("rejected artifacts were touched: %v", err)
		}
	})

	t.Run("run directory symlink", func(t *testing.T) {
		root, runDir := validRun(t)
		link := filepath.Join(root, ".fledge", "context", "runs", "linked")
		if err := os.Symlink(runDir, link); err != nil {
			t.Fatal(err)
		}
		if _, err := renderProject(link, time.Now()); err == nil ||
			!strings.Contains(err.Error(), "symlink") {
			t.Fatalf("renderProject() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(runDir, "scan.json")); err != nil {
			t.Fatalf("symlink target was touched: %v", err)
		}
	})

	for _, target := range []string{
		"requests",
		"replies",
		"scan.json",
		"requests/core.json",
		"replies/core.json",
		"synthesis.json",
		"provenance.json",
	} {
		t.Run(strings.ReplaceAll(target, "/", "-"), func(t *testing.T) {
			_, runDir := validRun(t)
			name := filepath.Join(runDir, filepath.FromSlash(target))
			realName := name + ".real"
			if err := os.Rename(name, realName); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(realName, name); err != nil {
				t.Fatal(err)
			}
			if _, err := renderProject(runDir, time.Now()); err == nil ||
				!strings.Contains(err.Error(), "symlink") {
				t.Fatalf("renderProject() error = %v", err)
			}
			if _, err := os.Lstat(filepath.Join(runDir, "provenance.json")); err != nil {
				t.Fatalf("preflight rejection touched artifacts: %v", err)
			}
		})
	}
}

func TestRenderProjectSynthesisSemanticAndOwnershipValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "duplicate routing",
			mutate: func(t *testing.T, runDir string) {
				var synthesis Synthesis
				readJSON(t, filepath.Join(runDir, "synthesis.json"), &synthesis)
				synthesis.Routing = append(synthesis.Routing, synthesis.Routing[0])
				writeJSON(t, filepath.Join(runDir, "synthesis.json"), synthesis)
			},
			want: "duplicate path_prefix",
		},
		{
			name: "empty routing guidance",
			mutate: func(t *testing.T, runDir string) {
				var synthesis Synthesis
				readJSON(t, filepath.Join(runDir, "synthesis.json"), &synthesis)
				synthesis.Routing[0].Guidance = " "
				writeJSON(t, filepath.Join(runDir, "synthesis.json"), synthesis)
			},
			want: "guidance must be nonempty",
		},
		{
			name: "empty flow description",
			mutate: func(t *testing.T, runDir string) {
				var synthesis Synthesis
				readJSON(t, filepath.Join(runDir, "synthesis.json"), &synthesis)
				synthesis.CrossGroupFlows[0].Description = ""
				writeJSON(t, filepath.Join(runDir, "synthesis.json"), synthesis)
			},
			want: "description must be nonempty",
		},
		{
			name: "empty global invariant",
			mutate: func(t *testing.T, runDir string) {
				var synthesis Synthesis
				readJSON(t, filepath.Join(runDir, "synthesis.json"), &synthesis)
				synthesis.GlobalInvariants[0] = " "
				writeJSON(t, filepath.Join(runDir, "synthesis.json"), synthesis)
			},
			want: "global_invariants[0]",
		},
		{
			name:   "routing violates file ownership",
			mutate: splitRunOwnership,
			want:   "owned by group",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, runDir := validRun(t)
			test.mutate(t, runDir)
			if _, err := renderProject(runDir, time.Now()); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("renderProject() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRenderProjectPostPublicationFailuresAreWarnings(t *testing.T) {
	t.Run("directory sync", func(t *testing.T) {
		root, runDir := validRun(t)
		old := syncPublishedDirectory
		syncPublishedDirectory = func(*os.Root) error { return errors.New("sync denied") }
		t.Cleanup(func() { syncPublishedDirectory = old })

		result, err := renderProject(runDir, time.Now())
		if err != nil {
			t.Fatalf("renderProject() error = %v", err)
		}
		// One warning per published artifact: project.md and provenance.json.
		if len(result.Warnings) != 2 || !strings.Contains(result.Warnings[0], "durability sync failed") ||
			!strings.Contains(result.Warnings[1], "durability sync failed") {
			t.Fatalf("warnings = %q", result.Warnings)
		}
		if _, err := os.Stat(filepath.Join(root, ".fledge", "context", "project.md")); err != nil {
			t.Fatalf("published document missing: %v", err)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		root, runDir := validRun(t)
		old := removeRunPath
		removeRunPath = func(root *os.Root, name string) error {
			if filepath.Base(name) == "scan.json" {
				return errors.New("remove denied")
			}
			return root.Remove(name)
		}
		t.Cleanup(func() { removeRunPath = old })

		result, err := renderProject(runDir, time.Now())
		if err != nil {
			t.Fatalf("renderProject() error = %v", err)
		}
		if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "cleanup failed") {
			t.Fatalf("warnings = %q", result.Warnings)
		}
		if _, err := os.Stat(filepath.Join(root, ".fledge", "context", "project.md")); err != nil {
			t.Fatalf("published document missing: %v", err)
		}
		if _, err := os.Stat(filepath.Join(runDir, "scan.json")); err != nil {
			t.Fatalf("failed cleanup evidence missing: %v", err)
		}
	})
}

func TestRenderProjectPinnedRunCannotBeRedirectedAfterOpen(t *testing.T) {
	root, runDir := validRun(t)
	movedRun := runDir + ".opened"
	replacementScan := []byte("replacement must not be read or removed\n")

	old := renderRootsOpened
	renderRootsOpened = func() error {
		if err := os.Rename(runDir, movedRun); err != nil {
			return err
		}
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(runDir, "scan.json"), replacementScan, 0o644)
	}
	t.Cleanup(func() { renderRootsOpened = old })

	result, err := renderProject(runDir, time.Now())
	if err != nil {
		t.Fatalf("renderProject() error = %v", err)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "changed after it was opened") {
		t.Fatalf("warnings = %q, want replacement cleanup warning", result.Warnings)
	}
	document, err := os.ReadFile(filepath.Join(root, ".fledge", "context", "project.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), "A small project.") {
		t.Fatalf("published replacement input instead of opened run:\n%s", document)
	}
	got, err := os.ReadFile(filepath.Join(runDir, "scan.json"))
	if err != nil {
		t.Fatalf("replacement input was removed: %v", err)
	}
	if string(got) != string(replacementScan) {
		t.Fatalf("replacement input changed to %q", got)
	}
}

func TestRenderProjectPinnedFileCannotBeRedirectedAfterOpen(t *testing.T) {
	root, runDir := validRun(t)
	outside := filepath.Join(t.TempDir(), "outside.json")
	outsideData := []byte("outside must not be read or removed\n")
	if err := os.WriteFile(outside, outsideData, 0o644); err != nil {
		t.Fatal(err)
	}
	scanName := filepath.Join(runDir, "scan.json")
	openedScan := scanName + ".opened"

	old := renderRootsOpened
	renderRootsOpened = func() error {
		if err := os.Rename(scanName, openedScan); err != nil {
			return err
		}
		return os.Symlink(outside, scanName)
	}
	t.Cleanup(func() { renderRootsOpened = old })

	result, err := renderProject(runDir, time.Now())
	if err != nil {
		t.Fatalf("renderProject() error = %v", err)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "refusing cleanup") {
		t.Fatalf("warnings = %q, want replacement cleanup warning", result.Warnings)
	}
	document, err := os.ReadFile(filepath.Join(root, ".fledge", "context", "project.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), "A small project.") {
		t.Fatalf("published symlink target instead of opened input:\n%s", document)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("outside symlink target was removed: %v", err)
	}
	if string(got) != string(outsideData) {
		t.Fatalf("outside symlink target changed to %q", got)
	}
	if info, err := os.Lstat(scanName); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replacement symlink was followed or removed: info=%v err=%v", info, err)
	}
}

func TestRenderProjectPinnedArtifactDirectoriesCannotBeRedirectedAfterOpen(t *testing.T) {
	for _, dirName := range []string{"requests", "replies"} {
		t.Run(dirName, func(t *testing.T) {
			root, runDir := validRun(t)
			dir := filepath.Join(runDir, dirName)
			openedDir := dir + ".opened"
			replacement := []byte("replacement must not be read or removed\n")

			old := renderRootsOpened
			renderRootsOpened = func() error {
				if err := os.Rename(dir, openedDir); err != nil {
					return err
				}
				if err := os.Mkdir(dir, 0o755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "core.json"), replacement, 0o644)
			}
			t.Cleanup(func() { renderRootsOpened = old })

			result, err := renderProject(runDir, time.Now())
			if err != nil {
				t.Fatalf("renderProject() error = %v", err)
			}
			if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "refusing cleanup") {
				t.Fatalf("warnings = %q, want replacement cleanup warning", result.Warnings)
			}
			document, err := os.ReadFile(filepath.Join(root, ".fledge", "context", "project.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(document), "The core subsystem.") {
				t.Fatalf("published replacement input instead of opened %s:\n%s", dirName, document)
			}
			got, err := os.ReadFile(filepath.Join(dir, "core.json"))
			if err != nil {
				t.Fatalf("replacement %s input was removed: %v", dirName, err)
			}
			if string(got) != string(replacement) {
				t.Fatalf("replacement %s input changed to %q", dirName, got)
			}
		})
	}
}

func TestRenderProjectPublicationUsesPinnedContextRoot(t *testing.T) {
	root, runDir := validRun(t)
	contextDir := filepath.Join(root, ".fledge", "context")
	openedContext := filepath.Join(root, ".fledge", "context.opened")
	replacementDocument := []byte("replacement context must not be published here\n")

	old := renderRootsOpened
	renderRootsOpened = func() error {
		if err := os.Rename(contextDir, openedContext); err != nil {
			return err
		}
		if err := os.Mkdir(contextDir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(contextDir, "project.md"), replacementDocument, 0o644)
	}
	t.Cleanup(func() { renderRootsOpened = old })

	if _, err := renderProject(runDir, time.Now()); err != nil {
		t.Fatalf("renderProject() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(contextDir, "project.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(replacementDocument) {
		t.Fatalf("publication was redirected into replacement context: %q", got)
	}
	published, err := os.ReadFile(filepath.Join(openedContext, "project.md"))
	if err != nil {
		t.Fatalf("project was not published beneath opened context root: %v", err)
	}
	if !strings.Contains(string(published), "A small project.") {
		t.Fatalf("opened context publication = %q", published)
	}
}

func TestRenderProjectFailuresPreserveDocumentAndArtifacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
		want   string
	}{
		{
			name: "scan totals",
			mutate: func(t *testing.T, runDir string) {
				var scan Scan
				readJSON(t, filepath.Join(runDir, "scan.json"), &scan)
				scan.TotalSize++
				writeJSON(t, filepath.Join(runDir, "scan.json"), scan)
			},
			want: "total_size",
		},
		{
			name: "duplicate ownership",
			mutate: func(t *testing.T, runDir string) {
				request := AnalyzerRequest{
					SchemaVersion: 1, GroupID: "other", Purpose: "Other", TotalSize: 1,
					Files: []File{{Path: "a.go", Size: 1}},
				}
				writeJSON(t, filepath.Join(runDir, "requests", "other.json"), request)
			},
			want: "assigned to both",
		},
		{
			name: "missing reply",
			mutate: func(t *testing.T, runDir string) {
				if err := os.Remove(filepath.Join(runDir, "replies", "core.json")); err != nil {
					t.Fatal(err)
				}
			},
			want: "missing reply",
		},
		{
			name: "analyzer error",
			mutate: func(t *testing.T, runDir string) {
				writeJSON(t, filepath.Join(runDir, "replies", "core.json"), AnalyzerErrorReply{
					SchemaVersion: 1, Status: "error", GroupID: "core",
					Errors: []AnalyzerError{{Code: "analysis-failed", Message: "failed"}},
				})
			},
			want: "analyzer returned",
		},
		{
			name: "unknown synthesis reference",
			mutate: func(t *testing.T, runDir string) {
				var synthesis Synthesis
				readJSON(t, filepath.Join(runDir, "synthesis.json"), &synthesis)
				synthesis.Routing[0].GroupID = "unknown"
				writeJSON(t, filepath.Join(runDir, "synthesis.json"), synthesis)
			},
			want: "unknown group_id",
		},
		{
			name: "bad provenance",
			mutate: func(t *testing.T, runDir string) {
				var provenance Provenance
				readJSON(t, filepath.Join(runDir, "provenance.json"), &provenance)
				provenance.Analyzers = append(provenance.Analyzers, provenance.Analyzers[0])
				writeJSON(t, filepath.Join(runDir, "provenance.json"), provenance)
			},
			want: "duplicate group_id",
		},
		{
			name: "reused provenance agent",
			mutate: func(t *testing.T, runDir string) {
				var provenance Provenance
				readJSON(t, filepath.Join(runDir, "provenance.json"), &provenance)
				provenance.Analyzers[0].Name = provenance.Forager.Name
				writeJSON(t, filepath.Join(runDir, "provenance.json"), provenance)
			},
			want: "reuses agent name",
		},
		{
			name: "padded provenance metadata",
			mutate: func(t *testing.T, runDir string) {
				var provenance Provenance
				readJSON(t, filepath.Join(runDir, "provenance.json"), &provenance)
				provenance.Analyzers[0].Profile = " sol "
				writeJSON(t, filepath.Join(runDir, "provenance.json"), provenance)
			},
			want: "single trimmed value",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, runDir := validRun(t)
			documentName := filepath.Join(root, ".fledge", "context", "project.md")
			if err := os.WriteFile(documentName, []byte("previous document\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, runDir)

			_, err := renderProject(runDir, time.Now())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("renderProject() error = %v, want containing %q", err, test.want)
			}
			data, readErr := os.ReadFile(documentName)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(data) != "previous document\n" {
				t.Fatalf("previous document changed to %q", data)
			}
			if _, statErr := os.Stat(filepath.Join(runDir, "scan.json")); statErr != nil {
				t.Fatalf("scan artifact not retained: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(runDir, "provenance.json")); statErr != nil {
				t.Fatalf("provenance artifact not retained: %v", statErr)
			}
		})
	}
}

func validRun(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	contextDir := filepath.Join(root, ".fledge", "context")
	runDir := filepath.Join(contextDir, "runs", "run-1")
	for _, dir := range []string{
		filepath.Join(runDir, "requests"),
		filepath.Join(runDir, "replies"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(t, filepath.Join(runDir, "scan.json"), Scan{
		SchemaVersion: 1,
		Root:          root,
		FileCount:     2,
		TotalSize:     3,
		Files:         []File{{Path: "a.go", Size: 1}, {Path: "image.png", Size: 2}},
	})
	writeJSON(t, filepath.Join(runDir, "requests", "core.json"), AnalyzerRequest{
		SchemaVersion: 1,
		GroupID:       "core",
		Purpose:       "Own the core subsystem.",
		TotalSize:     3,
		Files:         []File{{Path: "a.go", Size: 1}, {Path: "image.png", Size: 2}},
	})
	writeJSON(t, filepath.Join(runDir, "replies", "core.json"), AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "core",
		SubsystemSummary: "The core subsystem.",
		EntryPoints:      []EntryPoint{{Path: "a.go", Description: "Primary entry."}},
		KeySymbols:       []KeySymbol{{Path: "a.go", Name: "Run", Kind: "function", Description: "Runs it."}},
		Dependencies: Dependencies{
			Internal: []InternalDependency{{Path: "a.go", Description: "Core source."}},
			External: []ExternalDependency{{Name: "stdlib", Description: "Go standard library."}},
		},
		DataFlows:  []DataFlow{{From: "input", To: "output", Description: "Processes data."}},
		Invariants: []string{"The source is authoritative."},
		Tests:      []TestReference{{Path: "a.go", Description: "Inline example."}},
		Files: []FileSummary{
			{Path: "a.go", ContentKind: "text", Summary: "Go source."},
			{Path: "image.png", ContentKind: "non-text", Summary: "PNG image."},
		},
	})
	writeJSON(t, filepath.Join(runDir, "synthesis.json"), Synthesis{
		SchemaVersion:   1,
		ProjectOverview: "A small project.",
		Routing:         []Routing{{PathPrefix: ".", GroupID: "core", Guidance: "Route all work here."}},
		CrossGroupFlows: []CrossGroupFlow{{FromGroup: "core", ToGroup: "core", Description: "Internal loop."}},
		GlobalInvariants: []string{
			"Keep the context deterministic.",
		},
	})
	createdAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	writeJSON(t, filepath.Join(runDir, "provenance.json"), Provenance{
		SchemaVersion: 1,
		Forager:       Identity{Name: "forager-emperor", Profile: "terra", Model: "gpt-terra"},
		Analyzers: []AnalyzerIdentity{
			{GroupID: "core", Name: "analyzer-adelie", Profile: "sol", Model: "gpt-sol"},
		},
		CreatedAt: &createdAt,
	})
	return root, runDir
}

func splitRunOwnership(t *testing.T, runDir string) {
	t.Helper()
	var coreRequest AnalyzerRequest
	readJSON(t, filepath.Join(runDir, "requests", "core.json"), &coreRequest)
	coreRequest.Files = coreRequest.Files[:1]
	coreRequest.TotalSize = 1
	writeJSON(t, filepath.Join(runDir, "requests", "core.json"), coreRequest)

	var coreReply AnalyzerSuccessReply
	readJSON(t, filepath.Join(runDir, "replies", "core.json"), &coreReply)
	coreReply.Files = coreReply.Files[:1]
	writeJSON(t, filepath.Join(runDir, "replies", "core.json"), coreReply)

	writeJSON(t, filepath.Join(runDir, "requests", "assets.json"), AnalyzerRequest{
		SchemaVersion: 1, GroupID: "assets", Purpose: "Own assets.", TotalSize: 2,
		Files: []File{{Path: "image.png", Size: 2}},
	})
	writeJSON(t, filepath.Join(runDir, "replies", "assets.json"), AnalyzerSuccessReply{
		SchemaVersion:    1,
		Status:           "ok",
		GroupID:          "assets",
		SubsystemSummary: "Assets.",
		EntryPoints:      []EntryPoint{},
		KeySymbols:       []KeySymbol{},
		Dependencies:     Dependencies{Internal: []InternalDependency{}, External: []ExternalDependency{}},
		DataFlows:        []DataFlow{},
		Invariants:       []string{},
		Tests:            []TestReference{},
		Files:            []FileSummary{{Path: "image.png", ContentKind: "non-text", Summary: "PNG image."}},
	})
	var provenance Provenance
	readJSON(t, filepath.Join(runDir, "provenance.json"), &provenance)
	provenance.Analyzers = append(provenance.Analyzers, AnalyzerIdentity{
		GroupID: "assets", Name: "analyzer-assets", Profile: "sol", Model: "gpt-sol",
	})
	writeJSON(t, filepath.Join(runDir, "provenance.json"), provenance)
}

func cloneJSON[T any](t *testing.T, value T) T {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone T
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func writeJSON(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
