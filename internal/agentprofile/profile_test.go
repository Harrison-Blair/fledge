package agentprofile

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := testProfile("review")
	tests := []struct {
		name    string
		mutate  func(*Profile)
		field   string
		wantErr bool
	}{
		{name: "valid"},
		{name: "optional values absent", mutate: func(p *Profile) {
			p.Description, p.Model, p.Effort, p.Instructions = "", "", "", ""
		}},
		{name: "generic harness and model absent", mutate: func(p *Profile) {
			p.Harness, p.Model = "", ""
		}},
		{name: "name absent", mutate: func(p *Profile) { p.Name = "" }, field: "name", wantErr: true},
		{name: "name traversal", mutate: func(p *Profile) { p.Name = "../review" }, field: "name", wantErr: true},
		{name: "name slash", mutate: func(p *Profile) { p.Name = "team/review" }, field: "name", wantErr: true},
		{name: "name backslash", mutate: func(p *Profile) { p.Name = `team\review` }, field: "name", wantErr: true},
		{name: "hidden name", mutate: func(p *Profile) { p.Name = ".review" }, field: "name", wantErr: true},
		{name: "unicode name", mutate: func(p *Profile) { p.Name = "réview" }, field: "name", wantErr: true},
		{name: "long name", mutate: func(p *Profile) { p.Name = strings.Repeat("a", maxNameBytes+1) }, field: "name", wantErr: true},
		{name: "schema absent", mutate: func(p *Profile) { p.SchemaVersion = 0 }, field: "schema_version", wantErr: true},
		{name: "future schema", mutate: func(p *Profile) { p.SchemaVersion = 2 }, field: "schema_version", wantErr: true},
		{name: "model without harness", mutate: func(p *Profile) { p.Harness = "" }, field: "model", wantErr: true},
		{name: "harness unsupported", mutate: func(p *Profile) { p.Harness = "cursor" }, field: "harness", wantErr: true},
		{name: "harness whitespace", mutate: func(p *Profile) { p.Harness = " codex" }, field: "harness", wantErr: true},
		{name: "effort unsupported", mutate: func(p *Profile) { p.Effort = "maximum" }, field: "effort", wantErr: true},
		{name: "blank model", mutate: func(p *Profile) { p.Model = " \t" }, field: "model", wantErr: true},
		{name: "nul instructions", mutate: func(p *Profile) { p.Instructions = "bad\x00text" }, field: "instructions", wantErr: true},
		{name: "empty native arg", mutate: func(p *Profile) { p.NativeArgs = []string{"--one", ""} }, field: "native_args[1]", wantErr: true},
		{name: "nul native arg", mutate: func(p *Profile) { p.NativeArgs = []string{"bad\x00arg"} }, field: "native_args[0]", wantErr: true},
		{name: "invalid utf8 native arg", mutate: func(p *Profile) { p.NativeArgs = []string{string([]byte{0xff})} }, field: "native_args[0]", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := valid
			profile.NativeArgs = append([]string(nil), valid.NativeArgs...)
			if tt.mutate != nil {
				tt.mutate(&profile)
			}
			err := Validate(profile)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() error = %v, want ErrInvalid", err)
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("Validate() error type = %T, want *ValidationError", err)
			}
			if validationErr.Field != tt.field {
				t.Fatalf("ValidationError.Field = %q, want %q", validationErr.Field, tt.field)
			}
		})
	}
}

func TestValidateAcceptsEverySupportedHarnessAndEffort(t *testing.T) {
	for _, harness := range []string{HarnessClaude, HarnessCodex, HarnessOpenCode, HarnessPi} {
		for _, effort := range []string{"", EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax} {
			profile := testProfile("valid")
			profile.Harness, profile.Effort = harness, effort
			if err := Validate(profile); err != nil {
				t.Errorf("Validate(harness=%q, effort=%q) error = %v", harness, effort, err)
			}
		}
	}
}

func TestProfileSerializationKeepsNameOutOfTOMLAndInJSON(t *testing.T) {
	profile := testProfile("review")
	data, err := encode(profile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "name") {
		t.Fatalf("TOML persisted derived name:\n%s", data)
	}
	if !strings.Contains(string(data), "schema_version = 1") ||
		!strings.Contains(string(data), `native_args = ["--readonly"]`) {
		t.Fatalf("TOML missing schema fields:\n%s", data)
	}

	jsonData, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonData), `"name":"review"`) {
		t.Fatalf("JSON missing logical name: %s", jsonData)
	}
}

func TestInstructionOnlyProfileOmitsLaunchSelectionsAndEmptyFields(t *testing.T) {
	profile := Profile{
		Name: "orchestrator", SchemaVersion: SchemaVersion,
		Instructions: "Use only the inherited Fledge session.",
	}
	data, err := encode(profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"harness", "model", "effort", "description", "native_args"} {
		if strings.Contains(string(data), omitted) {
			t.Fatalf("instruction-only TOML contains %q:\n%s", omitted, data)
		}
	}
	jsonData, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{`"harness"`, `"model"`, `"effort"`, `"description"`, `"native_args"`} {
		if strings.Contains(string(jsonData), omitted) {
			t.Fatalf("instruction-only JSON contains %q: %s", omitted, jsonData)
		}
	}
}

func testProfile(name string) Profile {
	return Profile{
		Name:          name,
		SchemaVersion: SchemaVersion,
		Description:   "Find correctness, security, and test risks.",
		Harness:       "codex",
		Model:         "gpt-5.6-sol",
		Effort:        "high",
		NativeArgs:    []string{"--readonly"},
		Instructions:  "Review code like an owner.",
	}
}
