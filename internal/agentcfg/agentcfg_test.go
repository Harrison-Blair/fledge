package agentcfg

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func TestRoute(t *testing.T) {
	tests := []struct {
		model       string
		integration string
		provider    string
		wantErr     bool
	}{
		{model: "claude-opus-4", integration: "claude"},
		{model: "claude", integration: "claude"},
		{model: "gpt-5", integration: "pi", provider: "openai-codex"},
		{model: "codex-mini", integration: "pi", provider: "openai-codex"},
		{model: "o3", integration: "pi", provider: "openai-codex"},
		{model: "o4-mini", integration: "pi", provider: "openai-codex"},
		{model: "opencode-zen", integration: "pi", provider: "opencode"},
		{model: "opencode-go/glm-5.2", integration: "pi", provider: "opencode-go"},
		{model: "llama-3", wantErr: true},
		{model: "opus", wantErr: true},
		{model: "o", wantErr: true},
		{model: "omni", wantErr: true},
		{model: "", wantErr: true},
	}

	for _, tt := range tests {
		integration, provider, err := Route(tt.model)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Route(%q) = %q/%q, want error", tt.model, integration, provider)
			}
			continue
		}
		if err != nil {
			t.Errorf("Route(%q): %v", tt.model, err)
			continue
		}
		if integration != tt.integration || provider != tt.provider {
			t.Errorf("Route(%q) = %q/%q, want %q/%q", tt.model, integration, provider, tt.integration, tt.provider)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		desc    string
		name    string
		cfg     Config
		wantErr bool
	}{
		{desc: "claude ok", name: "worker", cfg: Config{Integration: "claude"}},
		{desc: "pi ok", name: "worker2", cfg: Config{Integration: "pi"}},
		{desc: "codex ok", name: "worker3", cfg: Config{Integration: "codex"}},
		{desc: "claude with permission mode", name: "worker", cfg: Config{Integration: "claude", PermissionMode: "acceptEdits"}},
		{desc: "pi with provider", name: "worker", cfg: Config{Integration: "pi", Provider: "openai"}},
		{desc: "codex with sandbox", name: "worker", cfg: Config{Integration: "codex", Sandbox: "workspace-write"}},
		{desc: "empty name", name: "", cfg: Config{Integration: "claude"}, wantErr: true},
		{desc: "uppercase name", name: "Worker", cfg: Config{Integration: "claude"}, wantErr: true},
		{desc: "kebab-case name", name: "code-worker", cfg: Config{Integration: "claude"}},
		{desc: "empty integration", name: "worker", cfg: Config{}, wantErr: true},
		{desc: "unknown integration", name: "worker", cfg: Config{Integration: "goose"}, wantErr: true},
		{desc: "provider on claude", name: "worker", cfg: Config{Integration: "claude", Provider: "openai"}, wantErr: true},
		{desc: "provider on codex", name: "worker", cfg: Config{Integration: "codex", Provider: "openai"}, wantErr: true},
		{desc: "permission mode on pi", name: "worker", cfg: Config{Integration: "pi", PermissionMode: "acceptEdits"}, wantErr: true},
		{desc: "permission mode on codex", name: "worker", cfg: Config{Integration: "codex", PermissionMode: "acceptEdits"}, wantErr: true},
		{desc: "sandbox on claude", name: "worker", cfg: Config{Integration: "claude", Sandbox: "read-only"}, wantErr: true},
		{desc: "sandbox on pi", name: "worker", cfg: Config{Integration: "pi", Sandbox: "read-only"}, wantErr: true},
		{desc: "reserved orchestrator name", name: ReservedOrchestrator, cfg: Config{Integration: "claude"}},
	}

	for _, tt := range tests {
		err := tt.cfg.Validate(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: Validate(%q) error = %v, wantErr %v", tt.desc, tt.name, err, tt.wantErr)
		}
	}
}

func TestCommandArgv(t *testing.T) {
	tests := []struct {
		desc string
		cfg  Config
		want []string
	}{
		{
			desc: "claude bare",
			cfg:  Config{Integration: "claude"},
			want: []string{"claude", "--session-id", "sid"},
		},
		{
			desc: "claude full",
			cfg: Config{
				Integration:    "claude",
				Model:          "claude-opus-4",
				PermissionMode: "acceptEdits",
				Argv:           []string{"--verbose"},
			},
			want: []string{"claude", "--session-id", "sid", "--permission-mode", "acceptEdits", "--model", "claude-opus-4", "--verbose"},
		},
		{
			desc: "pi bare",
			cfg:  Config{Integration: "pi"},
			want: []string{"pi", "--mode", "rpc"},
		},
		{
			desc: "pi full ignores session id",
			cfg: Config{
				Integration: "pi",
				Provider:    "openai",
				Model:       "o3",
				Argv:        []string{"--trace"},
			},
			want: []string{"pi", "--mode", "rpc", "--provider", "openai", "--model", "o3", "--trace"},
		},
		{
			desc: "codex bare ignores session id",
			cfg:  Config{Integration: "codex"},
			want: []string{"codex"},
		},
		{
			desc: "codex full",
			cfg: Config{
				Integration: "codex",
				Model:       "gpt-5.6-sol",
				Sandbox:     "workspace-write",
				Argv:        []string{"--profile", "work"},
			},
			want: []string{"codex", "--sandbox", "workspace-write", "--model", "gpt-5.6-sol", "--profile", "work"},
		},
		{
			desc: "unknown integration is not launchable",
			cfg:  Config{Integration: "goose", Model: "gpt-5"},
			want: nil,
		},
	}

	for _, tt := range tests {
		if got := tt.cfg.CommandArgv("sid"); !slices.Equal(got, tt.want) {
			t.Errorf("%s: CommandArgv() = %v, want %v", tt.desc, got, tt.want)
		}
	}
}

func TestPaneHosted(t *testing.T) {
	tests := []struct {
		integration string
		want        bool
	}{
		{integration: "claude", want: true},
		{integration: "codex", want: true},
		{integration: "pi", want: false},
		{integration: "", want: false},
		{integration: "goose", want: false},
	}
	for _, tt := range tests {
		if got := PaneHosted(tt.integration); got != tt.want {
			t.Errorf("PaneHosted(%q) = %v, want %v", tt.integration, got, tt.want)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	configs, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("Load = %v, want empty map", configs)
	}
	if configs == nil {
		t.Error("Load returned a nil map, want an empty one")
	}
}

func TestLoadValid(t *testing.T) {
	root := writeConfig(t, `{
		"builder": {"integration": "claude", "model": "claude-opus-4", "permission_mode": "acceptEdits"},
		"scout": {"integration": "pi", "provider": "openai", "model": "o3", "env": {"PI_LOG": "debug"}}
	}`)

	configs, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("Load returned %d configs, want 2", len(configs))
	}

	builder := configs["builder"]
	if builder.Integration != "claude" || builder.PermissionMode != "acceptEdits" || builder.Model != "claude-opus-4" {
		t.Errorf("builder = %+v", builder)
	}
	scout := configs["scout"]
	if scout.Provider != "openai" || scout.Env["PI_LOG"] != "debug" {
		t.Errorf("scout = %+v", scout)
	}
}

func TestLoadMalformed(t *testing.T) {
	root := writeConfig(t, `{"builder": `)
	if _, err := Load(root); err == nil {
		t.Fatal("Load of malformed JSON succeeded, want error")
	}
}

func TestLoadCatalogOnly(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, CatalogName, `{"gpt56solcx": {"integration": "codex", "model": "gpt-5.6-sol"}}`)

	configs, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := configs["gpt56solcx"]; got.Integration != "codex" || got.Model != "gpt-5.6-sol" {
		t.Errorf("catalog entry = %+v", got)
	}
}

func TestLoadUserShadowsCatalog(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, CatalogName, `{
		"gpt55pi": {"integration": "pi", "provider": "openai-codex", "model": "gpt-5.5"},
		"gpt56solcx": {"integration": "codex", "model": "gpt-5.6-sol"}
	}`)
	writeConfigFile(t, root, FileName, `{
		"gpt56solcx": {"integration": "codex", "model": "gpt-5.6-sol", "sandbox": "workspace-write"}
	}`)

	configs, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("Load returned %d configs, want 2 (merged)", len(configs))
	}
	if got := configs["gpt56solcx"]; got.Sandbox != "workspace-write" {
		t.Errorf("user entry did not shadow the catalog: %+v", got)
	}
	if got := configs["gpt55pi"]; got.Provider != "openai-codex" {
		t.Errorf("unshadowed catalog entry lost: %+v", got)
	}
}

func TestLoadMalformedCatalogNamesItsFile(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, CatalogName, `{"broken": `)
	_, err := Load(root)
	if err == nil {
		t.Fatal("Load of malformed catalog succeeded, want error")
	}
	if !strings.Contains(err.Error(), CatalogName) {
		t.Errorf("error = %v, want it to name %s", err, CatalogName)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	writeConfigFile(t, root, FileName, body)
	return root
}

func writeConfigFile(t *testing.T, root, file, body string) {
	t.Helper()
	dir := filepath.Join(root, scaffold.DirName, filepath.Dir(file))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(file)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNewSessionID(t *testing.T) {
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	first := NewSessionID()
	if !uuidV4.MatchString(first) {
		t.Errorf("NewSessionID() = %q, not a v4 UUID", first)
	}
	if second := NewSessionID(); second == first {
		t.Errorf("NewSessionID() returned %q twice", first)
	}
}

func TestPortableNamesAllowKebabCase(t *testing.T) {
	if err := (Config{Integration: "claude"}).Validate(ReservedOrchestrator); err != nil {
		t.Errorf("reserved name %q rejected: %v", ReservedOrchestrator, err)
	}
	for _, name := range []string{"some-agent", "fledge-orchestrator2", "fledge-orchestra"} {
		if err := (Config{Integration: "claude"}).Validate(name); err != nil {
			t.Errorf("kebab-case name %q was rejected: %v", name, err)
		}
	}
	for _, name := range []string{"Fledge-Orchestrator", "-agent", "agent-", "some--agent"} {
		if err := (Config{Integration: "claude"}).Validate(name); err == nil {
			t.Errorf("invalid kebab-case name %q was accepted", name)
		}
	}
}
