package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestClaudeOrchestratorStartupPluginIsDeterministicAndStartupOnly(t *testing.T) {
	d := boundDaemon(t, nil)
	args, err := d.orchestratorStartupArgs("claude")
	if err != nil {
		t.Fatal(err)
	}

	plugin := filepath.Join(flock.Dir(d.root, d.flockName), filepath.FromSlash(orchestratorRuntimeDir), "claude")
	if want := []string{"--plugin-dir", plugin}; !slices.Equal(args, want) {
		t.Fatalf("startup args = %#v, want %#v", args, want)
	}
	files := []struct {
		name string
		want string
	}{
		{filepath.Join(plugin, ".claude-plugin", "plugin.json"), claudePluginManifest},
		{filepath.Join(plugin, "hooks", "hooks.json"), claudeHooks},
		{filepath.Join(plugin, "ready.sh"), claudeReadyScript},
	}
	for _, file := range files {
		data, err := os.ReadFile(file.name)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != file.want {
			t.Fatalf("%s is not deterministic:\n%s", file.name, data)
		}
	}
	if !json.Valid([]byte(claudePluginManifest)) || !json.Valid([]byte(claudeHooks)) {
		t.Fatal("generated Claude plugin contains invalid JSON")
	}
	for _, want := range []string{
		`"SessionStart"`,
		`"matcher": "startup"`,
		`fledge agent ready --no-wait`,
		`Fledge readiness succeeded`,
		`Fledge readiness failed`,
	} {
		if !strings.Contains(claudeHooks+claudeReadyScript, want) {
			t.Fatalf("Claude startup assets do not contain %q", want)
		}
	}
	info, err := os.Stat(filepath.Join(plugin, "ready.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("ready.sh mode = %o, want 700", info.Mode().Perm())
	}
}

func TestPiOrchestratorStartupExtensionIsDeterministicAndDoesNotTriggerTurn(t *testing.T) {
	d := boundDaemon(t, nil)
	args, err := d.orchestratorStartupArgs("pi")
	if err != nil {
		t.Fatal(err)
	}

	extension := filepath.Join(flock.Dir(d.root, d.flockName), filepath.FromSlash(orchestratorRuntimeDir), "pi", "readiness.ts")
	if want := []string{"--extension", extension}; !slices.Equal(args, want) {
		t.Fatalf("startup args = %#v, want %#v", args, want)
	}
	data, err := os.ReadFile(extension)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != piReadyExtension {
		t.Fatalf("Pi readiness extension is not deterministic:\n%s", data)
	}
	for _, want := range []string{
		`pi.on("session_start"`,
		`event.reason !== "startup"`,
		`pi.exec("fledge", ["agent", "ready", "--no-wait"])`,
		`Fledge readiness succeeded`,
		`Fledge readiness failed`,
		`deliverAs: "nextTurn"`,
		`triggerTurn: false`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Pi startup extension does not contain %q", want)
		}
	}
	if strings.Contains(string(data), "sendUserMessage") {
		t.Fatal("Pi readiness extension sends a user message, which would trigger a model turn")
	}
}

func TestPiOrchestratorLoadsExtensionWithoutPositionalBootstrap(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "pi", Provider: "openai-codex", Model: "gpt-x"},
	})

	if _, err := d.spawn(&protocol.Request{Config: "orchestrator-profile", Orchestrator: true}); err != nil {
		t.Fatal(err)
	}
	argv := strs(t, f.params("agent.start")["argv"])
	extension := filepath.Join(flock.Dir(d.root, d.flockName), filepath.FromSlash(orchestratorRuntimeDir), "pi", "readiness.ts")
	if len(argv) < 4 || argv[len(argv)-4] != "--append-system-prompt" ||
		argv[len(argv)-2] != "--extension" || argv[len(argv)-1] != extension {
		t.Fatalf("Pi orchestrator argv = %#v", argv)
	}
	if slicesContains(argv, orchestratorBootstrapPrompt) || slicesContains(argv, bootstrapPrompt) {
		t.Fatalf("Pi orchestrator argv carries a positional readiness prompt: %#v", argv)
	}
}

func TestCodexOrchestratorKeepsPositionalBootstrap(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "codex", Model: "gpt-x"},
	})

	if _, err := d.spawn(&protocol.Request{Config: "orchestrator-profile", Orchestrator: true}); err != nil {
		t.Fatal(err)
	}
	argv := strs(t, f.params("agent.start")["argv"])
	if argv[len(argv)-1] != orchestratorBootstrapPrompt {
		t.Fatalf("Codex orchestrator argv = %#v, want existing readiness prompt", argv)
	}
	for _, unexpected := range []string{"--plugin-dir", "--extension"} {
		if slicesContains(argv, unexpected) {
			t.Fatalf("Codex orchestrator argv unexpectedly contains %q: %#v", unexpected, argv)
		}
	}
}

func TestOrchestratorStartupAssetWriteFailurePreventsLaunchAndReleasesName(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "claude", Model: "claude-opus-4"},
	})
	blocker := filepath.Join(flock.Dir(d.root, d.flockName), "runtime")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := d.spawn(&protocol.Request{Config: "orchestrator-profile", Orchestrator: true})
	if err == nil || !strings.Contains(err.Error(), "write Claude orchestrator readiness plugin") {
		t.Fatalf("spawn error = %v", err)
	}
	if got := f.count("agent.start"); got != 0 {
		t.Fatalf("Herdr launches = %d, want 0", got)
	}
	if len(d.agents) != 0 {
		t.Fatalf("failed asset write retained roster entries: %+v", d.agents)
	}
	if hasEvent(t, d, evRegistered, agentcfg.ReservedOrchestrator) {
		t.Fatal("failed asset write journaled an orchestrator registration")
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := d.spawn(&protocol.Request{Config: "orchestrator-profile", Orchestrator: true}); err != nil {
		t.Fatalf("orchestrator name was not released after asset failure: %v", err)
	}
}

func TestOrchestratorReadinessFailureUsesExistingTimeoutCleanup(t *testing.T) {
	f := serveHerdr(t, map[string]string{"agent.start": paneStartedReply})
	d := boundDaemon(t, f)
	d.skipReadiness = false
	writeAgents(t, d.root, map[string]agentcfg.Config{
		"orchestrator-profile": {Integration: "pi", Provider: "openai-codex", Model: "gpt-x"},
	})

	_, err := d.spawn(&protocol.Request{
		Config: "orchestrator-profile", Orchestrator: true, TimeoutMS: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "did not become ready") {
		t.Fatalf("spawn error = %v, want readiness timeout", err)
	}
	if got := f.count("pane.close"); got != 1 {
		t.Fatalf("pane closes = %d, want 1", got)
	}
	if got := agentState(d, agentcfg.ReservedOrchestrator); got != stateStopped {
		t.Fatalf("orchestrator state = %q, want stopped", got)
	}
}
