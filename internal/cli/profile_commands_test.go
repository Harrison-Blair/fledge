package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentprofile"
	"github.com/Harrison-Blair/fledge/internal/ui"
)

func TestAgentProfileCRUDHumanAndJSON(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)

	code, stdout, stderr := executeCLI(t, []string{
		"agent", "profile", "create", "reviewer",
		"--harness", "codex", "--model", "gpt-5.6", "--effort", "high",
		"--description", "Reviews changes", "--instructions", "Review carefully.",
		"--native-arg=--image=diagram.png",
	}, "")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Created agent profile reviewer") {
		t.Fatalf("create: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	code, stdout, stderr = executeCLI(t, []string{"--json", "agent", "profile", "show", "reviewer"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("show: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	data := successData(t, stdout)
	profile := data["profile"].(map[string]any)
	if profile["name"] != "reviewer" || profile["harness"] != "codex" ||
		profile["model"] != "gpt-5.6" || profile["effort"] != "high" {
		t.Fatalf("show profile = %#v", profile)
	}

	code, stdout, stderr = executeCLI(t, []string{
		"--json", "agent", "profile", "update", "reviewer",
		"--model", "gpt-5.7", "--effort=", "--native-arg=--image=updated.png",
	}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("update: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	profile = successData(t, stdout)["profile"].(map[string]any)
	if profile["model"] != "gpt-5.7" {
		t.Fatalf("updated profile = %#v", profile)
	}
	if _, present := profile["effort"]; present {
		t.Fatalf("cleared effort still present: %#v", profile)
	}

	code, stdout, stderr = executeCLI(t, []string{"agent", "profile", "list"}, "")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "NAME") ||
		!strings.Contains(stdout, "reviewer") || !strings.Contains(stdout, "gpt-5.7") {
		t.Fatalf("list: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = executeCLI(t, []string{"--json", "agent", "profile", "list"}, "")
	if code != 0 || stderr != "" {
		t.Fatalf("JSON list: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	profiles := successData(t, stdout)["profiles"].([]any)
	if len(profiles) != 1 || profiles[0].(map[string]any)["name"] != "reviewer" {
		t.Fatalf("JSON profiles = %#v", profiles)
	}
	code, stdout, stderr = executeCLI(t, []string{"--json", "agent", "profile", "validate", "reviewer"}, "")
	if code != 0 || stderr != "" || successData(t, stdout)["valid"] != true {
		t.Fatalf("validate: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = executeCLI(t, []string{"--json", "agent", "profile", "delete", "reviewer"}, "")
	if code != 0 || stderr != "" || successData(t, stdout)["deleted"] != true {
		t.Fatalf("delete: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	code, stdout, stderr = executeCLI(t, []string{"agent", "profile", "list"}, "")
	if code != 0 || stderr != "" || stdout != "No agent profiles\n" {
		t.Fatalf("empty list: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestAgentProfileFileAndStdinWorkflowIsStrictAndAtomic(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	valid := "schema_version = 1\ndescription = \"From file\"\nharness = \"claude\"\nmodel = \"sonnet\"\neffort = \"medium\"\nnative_args = [\"--debug\"]\ninstructions = \"Be deterministic.\"\n"

	code, stdout, stderr := executeCLI(t, []string{
		"agent", "profile", "create", "builder", "--file", "-", "--model", "opus",
	}, valid)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Created") {
		t.Fatalf("stdin create: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	before, err := os.ReadFile(filepath.Join(root, ".fledge", "profiles", "builder.toml"))
	if err != nil {
		t.Fatal(err)
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(invalidPath, []byte(valid+"unknown = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = executeCLI(t, []string{
		"--json", "agent", "profile", "update", "builder", "--file", invalidPath,
	}, "")
	if code != 1 || errorCode(t, stderr) != "profile_invalid" {
		t.Fatalf("invalid update: exit=%d stderr=%q", code, stderr)
	}
	after, err := os.ReadFile(filepath.Join(root, ".fledge", "profiles", "builder.toml"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("invalid update mutated profile: err=%v\nbefore=%q\nafter=%q", err, before, after)
	}

	code, _, stderr = executeCLI(t, []string{
		"--json", "agent", "profile", "validate", "candidate", "--file", "-",
	}, "schema_version = 1\nname = \"smuggled\"\nharness = \"codex\"\n")
	if code != 1 || errorCode(t, stderr) != "profile_invalid" {
		t.Fatalf("strict validate: exit=%d stderr=%q", code, stderr)
	}
	code, _, stderr = executeCLI(t, []string{
		"--json", "agent", "profile", "create", "old-schema", "--file", "-",
	}, "schema_version = 0\nharness = \"codex\"\n")
	if code != 1 || errorCode(t, stderr) != "profile_invalid" {
		t.Fatalf("explicit invalid schema: exit=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".fledge", "profiles", "old-schema.toml")); !os.IsNotExist(err) {
		t.Fatalf("invalid schema published a profile: %v", err)
	}
}

func TestAgentProfileTypedErrorsHaveStableJSONCodesAndDetails(t *testing.T) {
	root := initializedProject(t)
	t.Chdir(root)
	executeCLI(t, []string{"agent", "profile", "create", "reviewer", "-k", "codex"}, "")

	code, _, stderr := executeCLI(t, []string{
		"--json", "agent", "profile", "create", "reviewer", "-k", "codex",
	}, "")
	if code != 1 || errorCode(t, stderr) != "profile_already_exists" {
		t.Fatalf("duplicate: exit=%d stderr=%q", code, stderr)
	}
	var envelope errorEnvelope
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatal(err)
	}
	details := envelope.Error.Details.(map[string]any)
	if details["name"] != "reviewer" || details["operation"] != "create" {
		t.Fatalf("duplicate details = %#v", details)
	}

	code, _, stderr = executeCLI(t, []string{"--json", "agent", "profile", "show", "missing"}, "")
	if code != 1 || errorCode(t, stderr) != "profile_not_found" {
		t.Fatalf("not found: exit=%d stderr=%q", code, stderr)
	}
	code, _, stderr = executeCLI(t, []string{
		"--json", "agent", "profile", "create", "bad/name", "-k", "codex",
	}, "")
	if code != 1 || errorCode(t, stderr) != "profile_invalid" {
		t.Fatalf("invalid name: exit=%d stderr=%q", code, stderr)
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatal(err)
	}
	details = envelope.Error.Details.(map[string]any)
	if details["field"] != "name" {
		t.Fatalf("invalid details = %#v", details)
	}
}

func TestProfileHumanRenderingEscapesTerminalControlsWithoutChangingJSON(t *testing.T) {
	profile := agentprofile.Profile{
		Name:          "name\tforged",
		SchemaVersion: agentprofile.SchemaVersion,
		Harness:       "codex\nforged",
		Model:         "普通-model\tFORGED\nROW\r\x1b[31mred\x07\u009b",
		Effort:        "high\rforged",
		Description:   "Café\tFORGED\nROW\r\x1b]0;owned\x07\u2028next\u2029row",
		NativeArgs:    []string{"--safe"},
		Instructions:  "line one\n\x1b[2Jline two",
	}

	var human bytes.Buffer
	themeEnv := &environment{out: &human}
	printProfiles(&human, []agentprofile.Profile{profile}, themeEnv.stdoutTheme())
	output := human.String()
	if strings.Count(output, "\n") != 2 {
		t.Fatalf("profile data forged a table row:\n%q", output)
	}
	for _, control := range []string{"\t", "\r", "\x1b", "\x07", "\u009b", "\u2028", "\u2029"} {
		if strings.Contains(output, control) {
			t.Fatalf("profile table contains raw control %q: %q", control, output)
		}
	}
	for _, escaped := range []string{
		`name\tforged`, `codex\nforged`, `普通-model\tFORGED\nROW\r\x1b[31mred\x07\x9b`,
		`Café\tFORGED\nROW\r\x1b]0;owned\x07\u2028next\u2029row`,
	} {
		if !strings.Contains(output, escaped) {
			t.Fatalf("profile table is missing escaped readable value %q: %q", escaped, output)
		}
	}

	human.Reset()
	printProfile(&human, profile, themeEnv.stdoutTheme())
	if strings.Contains(human.String(), "\x1b") || !strings.Contains(human.String(), `line one\n\x1b[2Jline two`) {
		t.Fatalf("profile detail output is not terminal-safe: %q", human.String())
	}

	var jsonOutput bytes.Buffer
	jsonEnv := &environment{out: &jsonOutput, json: true}
	if err := jsonEnv.print(profileListResult{Profiles: []agentprofile.Profile{profile}}, func(io.Writer, *ui.Theme) {}); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data profileListResult `json:"data"`
	}
	if err := json.Unmarshal(jsonOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Profiles) != 1 || envelope.Data.Profiles[0].Model != profile.Model ||
		envelope.Data.Profiles[0].Description != profile.Description ||
		envelope.Data.Profiles[0].Instructions != profile.Instructions {
		t.Fatalf("JSON profile values changed: %#v", envelope.Data.Profiles)
	}
}

func executeCLI(t *testing.T, args []string, input string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func successData(t *testing.T, output string) map[string]any {
	t.Helper()
	var envelope struct {
		SchemaVersion int            `json:"schema_version"`
		OK            bool           `json:"ok"`
		Data          map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode success envelope: %v (%s)", err, output)
	}
	if envelope.SchemaVersion != schemaVersion || !envelope.OK {
		t.Fatalf("unexpected success envelope: %#v", envelope)
	}
	return envelope.Data
}

func errorCode(t *testing.T, output string) string {
	t.Helper()
	var envelope errorEnvelope
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, output)
	}
	return envelope.Error.Code
}
