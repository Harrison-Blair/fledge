package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// Shapes lifted from the live binaries: codex-cli 0.144.6 `codex debug
// models`, pi 0.80.3 `pi --list-models`.
const codexSample = `{
  "models": [
    {"slug": "gpt-5.6-sol", "display_name": "GPT-5.6-Sol", "visibility": "list"},
    {"slug": "gpt-5.5", "display_name": "GPT-5.5", "visibility": "list"},
    {"slug": "gpt-5.4", "display_name": "GPT-5.4", "visibility": "hide"},
    {"slug": "codex-auto-review", "display_name": "Codex Auto Review", "visibility": "hide"}
  ]
}`

const piSample = `provider      model                   context  max-out  thinking  images
openai-codex  gpt-5.5                 272K     128K     yes       yes
openai-codex  gpt-5.4                 272K     128K     yes       yes
opencode      gpt-5.4                 272K     128K     yes       yes
opencode-go   glm-5.1                 200K     64K      yes       yes
`

func TestParseCodexModels(t *testing.T) {
	entries, err := parseCodexModels([]byte(codexSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Hidden models are not spawnable choices.
	if len(entries) != 2 {
		t.Fatalf("parsed %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].model != "gpt-5.6-sol" || entries[0].integration != "codex" {
		t.Errorf("first entry = %+v", entries[0])
	}
}

func TestParseCodexModelsMalformed(t *testing.T) {
	if _, err := parseCodexModels([]byte("not json")); err == nil {
		t.Fatal("parse of non-JSON succeeded, want error")
	}
}

func TestParsePiModels(t *testing.T) {
	entries, err := parsePiModels([]byte(piSample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("parsed %d entries, want 4: %+v", len(entries), entries)
	}
	if entries[0].provider != "openai-codex" || entries[0].model != "gpt-5.5" {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[3].provider != "opencode-go" || entries[3].model != "glm-5.1" {
		t.Errorf("last entry = %+v", entries[3])
	}
}

func TestParsePiModelsMalformedRow(t *testing.T) {
	if _, err := parsePiModels([]byte("provider model\nlonely\n")); err == nil {
		t.Fatal("parse of a model-less row succeeded, want error")
	}
}

// installBin puts a fake binary on PATH. The empty-PATH case in other tests
// relies on t.Setenv's restore, so each test's PATH starts from the real one.
func installBin(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func fakeAll(t *testing.T) {
	t.Helper()
	installBin(t, "claude", "test \"$1\" = --version || exit 2\nprintf '%s\\n' '2.1.0'")
	installBin(t, "codex", "cat <<'EOF'\n"+codexSample+"\nEOF")
	installBin(t, "pi", "cat <<'EOF'\n"+piSample+"EOF")
}

func TestDiscoverNamesEverySourceAlways(t *testing.T) {
	fakeAll(t)
	configs, notes := Discover()
	if len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}

	want := map[string]agentcfg.Config{
		"defaultcl":  {Integration: "claude"},
		"opuscl":     {Integration: "claude", Model: "opus"},
		"fablecl":    {Integration: "claude", Model: "fable"},
		"sonnetcl":   {Integration: "claude", Model: "sonnet"},
		"haikucl":    {Integration: "claude", Model: "haiku"},
		"gpt56solcx": {Integration: "codex", Model: "gpt-5.6-sol"},
		"gpt55cx":    {Integration: "codex", Model: "gpt-5.5"},
		"gpt55pi":    {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
		"gpt54pi":    {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.4"},
		"gpt54oc":    {Integration: "pi", Provider: "opencode", Model: "gpt-5.4"},
		"glm51og":    {Integration: "pi", Provider: "opencode-go", Model: "glm-5.1"},
	}
	if len(configs) != len(want) {
		t.Fatalf("discovered %d configs, want %d: %+v", len(configs), len(want), configs)
	}
	for name, w := range want {
		got, ok := configs[name]
		if !ok {
			t.Errorf("missing %s", name)
			continue
		}
		if got.Integration != w.Integration || got.Provider != w.Provider || got.Model != w.Model {
			t.Errorf("%s = %+v, want %+v", name, got, w)
		}
	}
	for name, cfg := range configs {
		if err := cfg.Validate(name); err != nil {
			t.Errorf("generated entry does not validate: %v", err)
		}
	}
}

func TestDiscoverClaudeVersionGeneratesDefaultAndFamilyLaunchers(t *testing.T) {
	fakeAll(t)

	configs, notes := Discover()
	if len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
	if got := configs["defaultcl"]; got.Integration != "claude" || got.Model != "" || got.Provider != "" ||
		got.PermissionMode != "" || got.Sandbox != "" || len(got.Argv) != 0 || len(got.Env) != 0 {
		t.Errorf("defaultcl = %+v, want a model-less native Claude launcher", got)
	}
	for name, model := range map[string]string{"opuscl": "opus", "fablecl": "fable", "sonnetcl": "sonnet", "haikucl": "haiku"} {
		got, ok := configs[name]
		if !ok {
			t.Errorf("discovery has no %s launcher: %+v", name, configs)
			continue
		}
		if got.Integration != "claude" || got.Model != model || got.Provider != "" ||
			got.PermissionMode != "" || got.Sandbox != "" || len(got.Argv) != 0 || len(got.Env) != 0 {
			t.Errorf("%s = %+v, want its native Claude family launcher", name, got)
		}
	}
	count := 0
	for _, cfg := range configs {
		if cfg.Integration == "claude" {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("discovered %d claude launchers, want 5: %+v", count, configs)
	}
}

func TestDiscoverSkipsMissingBinary(t *testing.T) {
	installBin(t, "claude", "exit 0")
	installBin(t, "codex", "cat <<'EOF'\n"+codexSample+"\nEOF")
	t.Setenv("PATH", pathWithout(t, "pi"))

	configs, notes := Discover()
	if len(configs) != 7 {
		t.Fatalf("discovered %d configs, want Claude's 5 plus Codex's 2: %+v", len(configs), configs)
	}
	if len(notes) != 1 || notes[0].Integration != "pi" {
		t.Fatalf("notes = %+v, want one pi skip", notes)
	}
}

func TestDiscoverSkipsFailingBinary(t *testing.T) {
	installBin(t, "claude", "exit 0")
	installBin(t, "codex", "exit 1")
	installBin(t, "pi", "cat <<'EOF'\n"+piSample+"EOF")

	configs, notes := Discover()
	if len(notes) != 1 || notes[0].Integration != "codex" {
		t.Fatalf("notes = %+v, want one codex failure", notes)
	}
	if _, ok := configs["gpt55pi"]; !ok {
		t.Fatalf("pi models lost to codex's failure: %+v", configs)
	}
}

func TestRunDeadlineReportsTimeout(t *testing.T) {
	installBin(t, "stuck-discovery", "while :; do :; done")
	src := source{integration: "pi", argv: []string{"stuck-discovery", "--list-models"}}

	_, note := runWithTimeout(src, 20*time.Millisecond)
	if note == nil {
		t.Fatal("run returned no note, want timeout")
	}
	if note.Integration != "pi" {
		t.Errorf("note integration = %q, want pi", note.Integration)
	}
	if want := "stuck-discovery --list-models timed out after 20ms; skipped pi discovery"; note.Detail != want {
		t.Errorf("note detail = %q, want %q", note.Detail, want)
	}
	if cmdTimeout != 30*time.Second {
		t.Errorf("production timeout = %s, want 30s", cmdTimeout)
	}
}

func TestRunExternalSIGKILLReportsGenericFailure(t *testing.T) {
	// A child kills the discovery process while its context is still live.
	// Signal text alone must not be treated as proof that the deadline fired.
	installBin(t, "killed-discovery", "sh -c 'kill -9 $PPID' & wait")
	src := source{integration: "pi", argv: []string{"killed-discovery", "--list-models"}}

	_, note := runWithTimeout(src, time.Second)
	if note == nil {
		t.Fatal("run returned no note, want failure")
	}
	if note.Integration != "pi" {
		t.Errorf("note integration = %q, want pi", note.Integration)
	}
	if !strings.HasPrefix(note.Detail, "killed-discovery --list-models failed: ") {
		t.Errorf("note detail = %q, want generic failure", note.Detail)
	}
	if strings.Contains(note.Detail, "timed out") {
		t.Errorf("external SIGKILL mislabeled as timeout: %q", note.Detail)
	}
}

func TestDiscoverSkipsMissingClaudeWithNote(t *testing.T) {
	installBin(t, "codex", "cat <<'EOF'\n"+codexSample+"\nEOF")
	installBin(t, "pi", "cat <<'EOF'\n"+piSample+"EOF")
	t.Setenv("PATH", pathWithout(t, "claude"))

	configs, notes := Discover()
	for _, cfg := range configs {
		if cfg.Integration == "claude" {
			t.Fatalf("missing claude produced a launcher: %+v", configs)
		}
	}
	if len(configs) != 6 {
		t.Fatalf("claude's absence affected other discovery: %+v", configs)
	}
	if len(notes) != 1 || notes[0].Integration != "claude" || !strings.Contains(notes[0].Detail, "not on PATH") {
		t.Fatalf("notes = %+v, want one missing-claude note", notes)
	}
}

func TestDiscoverSkipsFailingClaudeWithNote(t *testing.T) {
	installBin(t, "claude", "exit 7")
	installBin(t, "codex", "cat <<'EOF'\n"+codexSample+"\nEOF")
	installBin(t, "pi", "cat <<'EOF'\n"+piSample+"EOF")

	configs, notes := Discover()
	for _, cfg := range configs {
		if cfg.Integration == "claude" {
			t.Fatalf("failing claude produced a launcher: %+v", configs)
		}
	}
	if len(configs) != 6 {
		t.Fatalf("claude's failure affected other discovery: %+v", configs)
	}
	if len(notes) != 1 || notes[0].Integration != "claude" || !strings.Contains(notes[0].Detail, "claude --version failed") {
		t.Fatalf("notes = %+v, want one failing-claude note", notes)
	}
}

func TestDiscoverDropsPostSuffixCollision(t *testing.T) {
	// Two distinct pi models that reduce to the same name and suffix.
	installBin(t, "claude", "exit 0")
	installBin(t, "pi", "cat <<'EOF'\nprovider model\nopencode gpt-5.5\nopencode gpt5.5\nEOF")
	t.Setenv("PATH", pathWithout(t, "codex"))

	configs, notes := Discover()
	if got := configs["gpt55oc"]; got.Model != "gpt-5.5" {
		t.Fatalf("survivor = %+v, want the first row's gpt-5.5", got)
	}
	var dropped bool
	for _, n := range notes {
		if n.Integration == "pi" && n.Detail != "" && n.Detail != "pi is not on PATH; skipped" {
			dropped = true
		}
	}
	if !dropped {
		t.Fatalf("no note for the dropped collider: %+v", notes)
	}
}

// pathWithout is the current PATH minus every directory that holds name, so
// LookPath cannot find it — even when the real binary is installed on the
// machine running the tests.
func pathWithout(t *testing.T, name string) string {
	t.Helper()
	var keep []string
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			keep = append(keep, dir)
		}
	}
	return strings.Join(keep, string(os.PathListSeparator))
}

func TestWriteIsStableAndLoadable(t *testing.T) {
	fakeAll(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, scaffold.DirName), 0o755); err != nil {
		t.Fatal(err)
	}

	configs, _ := Discover()
	if err := Write(root, configs); err != nil {
		t.Fatalf("write: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, scaffold.DirName, agentcfg.CatalogName))
	if err != nil {
		t.Fatal(err)
	}
	if err := Write(root, configs); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(root, scaffold.DirName, agentcfg.CatalogName))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("two writes of the same discovery differ")
	}

	// User entries shadow generated names, including Claude family launchers;
	// everything else flows through.
	if err := os.WriteFile(filepath.Join(root, scaffold.DirName, agentcfg.FileName),
		[]byte(`{
			"opus": {"integration": "claude", "model": "opus", "permission_mode": "plan"},
			"gpt56solcx": {"integration": "codex", "model": "gpt-5.6-sol", "sandbox": "read-only"}
		}`), 0o644); err != nil {
		t.Fatal(err)
	}
	merged, err := agentcfg.Load(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if merged["gpt56solcx"].Sandbox != "read-only" {
		t.Fatalf("user entry did not shadow the catalog: %+v", merged["gpt56solcx"])
	}
	if merged["opus"].PermissionMode != "plan" {
		t.Fatalf("user opus entry did not shadow discovery: %+v", merged["opus"])
	}
	if merged["glm51og"].Provider != "opencode-go" {
		t.Fatalf("catalog entry lost in merge: %+v", merged)
	}
}

// TestDiscoveredClaudeLaunchersDoNotCollideWithUserProfiles reproduces the
// machine-dependent break: a committed user definition declaring profile
// "opus" syncs fine on a machine without claude, but the claude-installed
// machine's generated (gitignored) catalog once contributed a bare "opus"
// launcher, so Synchronize hard-failed with a cross-source conflict. Suffixed
// launcher names must not collide with the profile names users commonly pick.
func TestDiscoveredClaudeLaunchersDoNotCollideWithUserProfiles(t *testing.T) {
	fakeAll(t)
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	configs, notes := Discover()
	if len(notes) != 0 {
		t.Fatalf("notes = %+v, want none", notes)
	}
	if err := Write(root, configs); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	def := "---\nname: reviewer\ndescription: Review changes.\nmodel: claude-opus-4\nfledge:\n  profile: opus\n  launch:\n    permission_mode: plan\n---\nReview.\n"
	path := filepath.Join(root, scaffold.DirName, agentcfg.AgentsDir, "user", "reviewer", "reviewer.agent.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := agentcfg.Synchronize(root); err != nil {
		t.Fatalf("Synchronize: %v", err)
	}
}
