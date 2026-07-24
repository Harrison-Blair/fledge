package contextdoc

import (
	"strings"
	"testing"
)

const bareRequestJSON = `{"schema_version":1,"group_id":"core","purpose":"Core","total_size":1,"files":[{"path":"a.go","size":1}]}`

const composedRequestJSON = `{"schema_version":1,"group_id":"core","purpose":"Core","instructions_before":"Analyze now.","total_size":1,"files":[{"path":"a.go","size":1}],"instructions_after":"Reply once."}`

func TestValidateAnalyzerRequestAcceptsInstructionFields(t *testing.T) {
	if err := ValidateAnalyzerRequest([]byte(composedRequestJSON)); err != nil {
		t.Fatalf("composed request rejected: %v", err)
	}
	if err := ValidateAnalyzerRequest([]byte(bareRequestJSON)); err != nil {
		t.Fatalf("bare request rejected: %v", err)
	}
	unknown := strings.Replace(composedRequestJSON, `"instructions_before"`, `"instructions_beyond"`, 1)
	if err := ValidateAnalyzerRequest([]byte(unknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestValidateComposedAnalyzerRequest(t *testing.T) {
	if err := ValidateComposedAnalyzerRequest([]byte(composedRequestJSON)); err != nil {
		t.Fatalf("composed request rejected: %v", err)
	}
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			name: "bare request",
			json: bareRequestJSON,
			want: "instructions_before must be nonempty",
		},
		{
			name: "blank before",
			json: strings.Replace(composedRequestJSON, `"Analyze now."`, `" "`, 1),
			want: "instructions_before must be nonempty",
		},
		{
			name: "missing after",
			json: strings.Replace(composedRequestJSON, `,"instructions_after":"Reply once."`, ``, 1),
			want: "instructions_after must be nonempty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateComposedAnalyzerRequest([]byte(test.json))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParseRequestTemplate(t *testing.T) {
	valid := "ignored commentary\n<instructions_before>\nBefore {group_id}.\n</instructions_before>\nmore commentary\n<instructions_after>After.</instructions_after>\ntrailing\n"
	template, err := ParseRequestTemplate([]byte(valid))
	if err != nil {
		t.Fatalf("ParseRequestTemplate: %v", err)
	}
	if template.Before != "Before {group_id}." || template.After != "After." {
		t.Fatalf("template = %+v", template)
	}

	tests := []struct {
		name     string
		document string
		want     string
	}{
		{
			name:     "missing before tag",
			document: "<instructions_after>After.</instructions_after>",
			want:     "missing the <instructions_before> tag",
		},
		{
			name:     "missing close tag",
			document: "<instructions_before>Before.\n<instructions_after>After.</instructions_after>",
			want:     "missing the </instructions_before> tag",
		},
		{
			name:     "duplicate tag",
			document: valid + "<instructions_after>Again.</instructions_after>",
			want:     "more than one <instructions_after> tag",
		},
		{
			name:     "empty section",
			document: "<instructions_before>  \n</instructions_before><instructions_after>After.</instructions_after>",
			want:     "<instructions_before> must be nonempty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseRequestTemplate([]byte(test.document))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestComposeAnalyzerRequestSubstitutesPlaceholders(t *testing.T) {
	request := AnalyzerRequest{
		SchemaVersion:      SchemaVersion,
		GroupID:            "core",
		Purpose:            "Core files",
		InstructionsBefore: "stale",
		TotalSize:          1,
		Files:              []File{{Path: "a.go", Size: 1}},
		InstructionsAfter:  "stale",
	}
	template := RequestTemplate{
		Before: "Group {group_id}: {purpose}. Keep {unknown} verbatim.",
		After:  "Reply for {group_id} via {worksheet_path}.",
	}
	composed, err := ComposeAnalyzerRequest(request, template, "runs/r1/worksheets/core.md")
	if err != nil {
		t.Fatalf("ComposeAnalyzerRequest: %v", err)
	}
	if composed.InstructionsBefore != "Group core: Core files. Keep {unknown} verbatim." {
		t.Errorf("InstructionsBefore = %q", composed.InstructionsBefore)
	}
	if composed.InstructionsAfter != "Reply for core via runs/r1/worksheets/core.md." {
		t.Errorf("InstructionsAfter = %q", composed.InstructionsAfter)
	}
	if composed.GroupID != request.GroupID || len(composed.Files) != 1 {
		t.Errorf("non-instruction fields changed: %+v", composed)
	}

	if _, err := ComposeAnalyzerRequest(request, template, ""); err == nil ||
		!strings.Contains(err.Error(), "{worksheet_path}") {
		t.Fatalf("missing worksheet path error = %v", err)
	}
}

func TestComposeWorksheetStampsTemplate(t *testing.T) {
	request := AnalyzerRequest{
		SchemaVersion: SchemaVersion,
		GroupID:       "core",
		Purpose:       "Core files",
		TotalSize:     3,
		Files:         []File{{Path: "a.go", Size: 1}, {Path: "b/c.go", Size: 2}},
	}
	worksheet, err := ComposeWorksheet(request, []byte("# {group_id}\n{purpose}\n\n{files}\n"))
	if err != nil {
		t.Fatalf("ComposeWorksheet: %v", err)
	}
	want := "# core\nCore files\n\n- [ ] `a.go` (1 bytes)\n- [ ] `b/c.go` (2 bytes)\n"
	if worksheet != want {
		t.Errorf("worksheet = %q, want %q", worksheet, want)
	}

	if _, err := ComposeWorksheet(request, []byte("  \n")); err == nil ||
		!strings.Contains(err.Error(), "nonempty") {
		t.Fatalf("empty template error = %v", err)
	}
}
