package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/daemon"
	"github.com/Harrison-Blair/fledge/internal/protocol"
	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func TestRootHelpOnlyShowsTopLevelCommandsAndFlags(t *testing.T) {
	out, err := captureRun(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  init ", "  deinit ", "  start ", "  stop ", "  watch ", "  context ", "  flock ", "  agent ", "--version, -V"} {
		if !strings.Contains(out, want) {
			t.Errorf("root help missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"--json", "--species", "register <type>", "context subcommands:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("root help unexpectedly contains %q:\n%s", unwanted, out)
		}
	}
}

func TestBareGroupsShowLocalHelpSuccessfully(t *testing.T) {
	for _, group := range []string{"context", "flock", "agent", "agent msg"} {
		t.Run(group, func(t *testing.T) {
			out, err := captureRun(t, strings.Fields(group)...)
			if err != nil {
				t.Fatal(err)
			}
			if out != helpPages[group] {
				t.Errorf("got:\n%s\nwant:\n%s", out, helpPages[group])
			}
		})
	}
}

func TestNestedHelpFormsResolveTheSameLeaf(t *testing.T) {
	for _, args := range [][]string{
		{"help", "agent", "spawn"},
		{"agent", "help", "spawn"},
		{"agent", "spawn", "--help"},
		{"agent", "spawn", "-H"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, err := captureRun(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if out != helpPages["agent spawn"] {
				t.Errorf("got:\n%s\nwant:\n%s", out, helpPages["agent spawn"])
			}
		})
	}
}

func TestExplicitRootAndGroupHelpForms(t *testing.T) {
	tests := []struct {
		args []string
		path string
	}{
		{[]string{"--help"}, ""},
		{[]string{"-H"}, ""},
		{[]string{"help"}, ""},
		{[]string{"start", "--help"}, "start"},
		{[]string{"help", "start"}, "start"},
		{[]string{"agent", "--help"}, "agent"},
		{[]string{"agent", "-H"}, "agent"},
		{[]string{"agent", "help"}, "agent"},
		{[]string{"help", "agent"}, "agent"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			out, err := captureRun(t, test.args...)
			if err != nil {
				t.Fatal(err)
			}
			if out != helpPages[test.path] {
				t.Errorf("got:\n%s\nwant:\n%s", out, helpPages[test.path])
			}
		})
	}
}

func TestEveryLeafAcceptsHelpWithoutExecuting(t *testing.T) {
	for path := range helpPages {
		if path == "" || !strings.Contains(path, " ") {
			continue
		}
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			args := append(strings.Fields(path), "--help")
			out, err := captureRun(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			if out != helpPages[path] {
				t.Errorf("got:\n%s\nwant:\n%s", out, helpPages[path])
			}
		})
	}

	for _, leaf := range []string{"init", "deinit", "start", "stop", "watch"} {
		out, err := captureRun(t, leaf, "--help")
		if err != nil {
			t.Fatal(err)
		}
		if out != helpPages[leaf] {
			t.Errorf("got:\n%s\nwant:\n%s", out, helpPages[leaf])
		}
	}
}

func TestFlockStartIsNotAccepted(t *testing.T) {
	if strings.Contains(helpPages["flock"], "  start") {
		t.Errorf("flock help still advertises start:\n%s", helpPages["flock"])
	}
	err := run([]string{"flock", "start"})
	if err == nil {
		t.Fatal("removed flock start command succeeded")
	}
	if !strings.Contains(err.Error(), `unknown flock subcommand "start"`) {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), helpPages["flock"]) {
		t.Errorf("error missing flock help:\n%s", err)
	}
}

func TestTopLevelMsgIsNotAccepted(t *testing.T) {
	if strings.Contains(rootHelp, "  msg ") {
		t.Errorf("root help still advertises msg:\n%s", rootHelp)
	}
	err := run([]string{"msg", "send"})
	if err == nil {
		t.Fatal("removed top-level msg command succeeded")
	}
	if !strings.Contains(err.Error(), `unknown command "msg"`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnknownNamesCarryNearestHelp(t *testing.T) {
	err := run([]string{"unknown"})
	if err == nil {
		t.Fatal("unknown root command succeeded")
	}
	if !strings.Contains(err.Error(), rootHelp) {
		t.Errorf("root error missing root help:\n%s", err)
	}

	err = run([]string{"agent", "unknown"})
	if err == nil {
		t.Fatal("unknown agent subcommand succeeded")
	}
	if !strings.Contains(err.Error(), helpPages["agent"]) {
		t.Errorf("subcommand error missing agent help:\n%s", err)
	}
	if strings.Contains(err.Error(), helpPages["agent spawn"]) {
		t.Errorf("subcommand error unexpectedly contains leaf help:\n%s", err)
	}

	err = run([]string{"help", "agent", "spawn", "unknown"})
	if err == nil {
		t.Fatal("unknown nested help topic succeeded")
	}
	if !strings.Contains(err.Error(), helpPages["agent spawn"]) {
		t.Errorf("nested help error missing nearest leaf help:\n%s", err)
	}
}

func TestSyntaxErrorsCarryLeafHelpButRuntimeErrorsDoNot(t *testing.T) {
	err := run([]string{"agent", "msg", "send"})
	if err == nil {
		t.Fatal("incomplete msg send succeeded")
	}
	if !strings.Contains(err.Error(), helpPages["agent msg send"]) {
		t.Errorf("syntax error missing leaf help:\n%s", err)
	}

	t.Setenv("FLEDGE_FLOCK", "")
	err = run([]string{"agent", "list"})
	if err == nil {
		t.Fatal("agent list without flock context succeeded")
	}
	if strings.Contains(err.Error(), "usage:") {
		t.Errorf("runtime error unexpectedly contains help:\n%s", err)
	}
}

// withStdin feeds the prompt from a pipe, the stdin mirror of captureRun's
// stdout swap. A pipe is never a char device, so tests that want the
// interactive path must also stub stdinIsTerminal.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = original
		r.Close()
	})
}

func stubStdinTerminal(t *testing.T) {
	t.Helper()
	original := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = original })
}

// stubStdinNotTerminal pins the scripted path: the test binary may itself be
// run from a terminal, so non-TTY behaviour cannot be left to ambient stdin.
func stubStdinNotTerminal(t *testing.T) {
	t.Helper()
	original := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = original })
}

func TestDeinitMissingDirIsNoop(t *testing.T) {
	dir := t.TempDir()
	out, err := captureRun(t, "deinit", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nothing to remove") {
		t.Errorf("output missing noop message:\n%s", out)
	}
}

func TestDeinitNonTTYAborts(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	withStdin(t, "y\n")
	_, err := captureRun(t, "deinit", dir)
	if err == nil {
		t.Fatal("deinit succeeded without a terminal on stdin")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("error does not mention terminal: %s", err)
	}
	if _, err := os.Stat(filepath.Join(dir, scaffold.DirName)); err != nil {
		t.Errorf(".fledge missing after refused deinit: %v", err)
	}
}

func TestDeinitDeclineLeavesTree(t *testing.T) {
	for name, input := range map[string]string{
		"n":     "n\n",
		"eof":   "",
		"enter": "\n",
		"nope":  "nope\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := scaffold.Ensure(dir); err != nil {
				t.Fatal(err)
			}
			stubStdinTerminal(t)
			withStdin(t, input)
			out, err := captureRun(t, "deinit", dir)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "aborted") {
				t.Errorf("output missing abort message:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(dir, scaffold.DirName)); err != nil {
				t.Errorf(".fledge missing after declined deinit: %v", err)
			}
		})
	}
}

func TestDeinitConfirmListsAndRemoves(t *testing.T) {
	for name, input := range map[string]string{"y": "y\n", "yes": "yes\n"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := scaffold.Ensure(dir); err != nil {
				t.Fatal(err)
			}
			sibling := filepath.Join(dir, "keep.txt")
			if err := os.WriteFile(sibling, []byte("keep"), 0o644); err != nil {
				t.Fatal(err)
			}
			stubStdinTerminal(t)
			withStdin(t, input)
			out, err := captureRun(t, "deinit", dir)
			if err != nil {
				t.Fatal(err)
			}
			prompt := strings.Index(out, "[y/N]")
			if prompt < 0 {
				t.Fatalf("output missing prompt:\n%s", out)
			}
			for _, entry := range []string{
				filepath.Join(scaffold.DirName, scaffold.AgentsName),
				filepath.Join(scaffold.DirName, "flocks") + "/",
			} {
				at := strings.Index(out, entry)
				if at < 0 || at > prompt {
					t.Errorf("listing missing %q before the prompt:\n%s", entry, out)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, scaffold.DirName)); !os.IsNotExist(err) {
				t.Errorf(".fledge still present after confirmed deinit: %v", err)
			}
			if _, err := os.Stat(sibling); err != nil {
				t.Errorf("sibling file removed: %v", err)
			}
		})
	}
}

func TestDeinitRefusesWhileFlockRunning(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, scaffold.DirName, "flocks", "flock1"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sock := daemon.SocketPath(dir, "flock1")
	if err := os.MkdirAll(filepath.Dir(sock), 0o755); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	_, err = captureRun(t, "deinit", dir)
	if err == nil {
		t.Fatal("deinit succeeded with a running flock")
	}
	if !strings.Contains(err.Error(), "flock1") || !strings.Contains(err.Error(), "flock stop") {
		t.Errorf("error does not name the running flock: %s", err)
	}
	if _, err := os.Stat(filepath.Join(dir, scaffold.DirName)); err != nil {
		t.Errorf(".fledge missing after refused deinit: %v", err)
	}
}

func captureRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	runErr := run(args)
	os.Stdout = original
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return string(out), runErr
}

// A roster with no spawned agent must render byte-identically to the format
// that shipped before the launch columns existed.
func TestAgentRowsPlainRosterUnchanged(t *testing.T) {
	rows := agentRows([]protocol.Agent{
		{Name: "engineer-emperor", Species: "emperor", PID: 101, Alive: true},
		{Name: "pm-gentoo", Species: "gentoo", PID: 2, Alive: false},
	})

	want := []string{
		"engineer-emperor  emperor    101      alive",
		"pm-gentoo         gentoo     2        dead",
	}
	assertRows(t, rows, want)
}

func TestAgentRowsSpawnedAddsLaunchColumns(t *testing.T) {
	rows := agentRows([]protocol.Agent{
		{
			Name: "worker-emperor", Species: "emperor", PID: 101, Alive: true,
			Integration: "claude", Model: "claude-sonnet-5", PaneID: "%7", State: "running",
		},
		{
			Name: "pi-gentoo", Species: "gentoo", PID: 202, Alive: false,
			Integration: "pi", Model: "gpt-x", State: "stopped",
		},
	})

	want := []string{
		"worker-emperor  emperor    101      alive  claude claude-sonnet-5 %7 running",
		"pi-gentoo       gentoo     202      dead   pi     gpt-x              stopped",
	}
	assertRows(t, rows, want)
}

func TestAgentRowsDedicatedWorkspaceAddsPlacement(t *testing.T) {
	rows := agentRows([]protocol.Agent{{
		Name: "fledge-forager-emperor", Species: "emperor", PID: 101, Alive: true,
		Integration: "codex", Model: "gpt-5.6-sol", PaneID: "w9:p2", State: "running",
		Agent: "fledge-forager", Profile: "gpt56cx", Source: "fledge/fledge-forager/fledge-forager.agent.md",
		WorkspaceID: "w9", WorkspaceLabel: "fledge-context",
	}})
	if len(rows) != 1 || !strings.Contains(rows[0], "workspace=fledge-context workspace_id=w9") {
		t.Fatalf("rows = %v", rows)
	}
}

// A self-registered agent sharing a roster with a spawned one keeps its blank
// launch columns rather than borrowing its neighbour's.
func TestAgentRowsMixedRosterLeavesRegisteredBlank(t *testing.T) {
	rows := agentRows([]protocol.Agent{
		{Name: "lead", Species: "emperor", PID: 1, Alive: true},
		{
			Name: "worker", Species: "gentoo", PID: 2, Alive: true,
			Integration: "claude", Model: "claude-sonnet-5", PaneID: "%7", State: "running",
		},
	})

	want := []string{
		"lead    emperor    1        alive",
		"worker  gentoo     2        alive  claude claude-sonnet-5 %7 running",
	}
	assertRows(t, rows, want)
}

// pickerFixture is the shared picker catalog: two claude configs under the
// derived anthropic provider and one pi config, so grouping and numbering both
// have something to order.
func pickerFixture() map[string]agentcfg.Config {
	return map[string]agentcfg.Config{
		"sonnet5": {Integration: "claude", Model: "claude-sonnet-5"},
		"opus48":  {Integration: "claude", Model: "claude-opus-4-8"},
		"gpt55":   {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
	}
}

// writePickerWorkspace makes a scaffolded workspace whose catalog is exactly
// configs and moves the test into it, so workspaceRoot finds it.
func writePickerWorkspace(t *testing.T, configs map[string]agentcfg.Config) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := scaffold.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(agentcfg.Index{Version: agentcfg.IndexVersion, Agents: map[string]agentcfg.AgentRecord{}, Profiles: configs})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, scaffold.DirName, agentcfg.CatalogName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func TestPickerRowsGroupsByProvider(t *testing.T) {
	rows := pickerRows(pickerFixture())

	want := []string{
		"Configured agents:",
		"",
		"  anthropic (claude)",
		"    1. opus48    claude-opus-4-8",
		"    2. sonnet5   claude-sonnet-5",
		"  openai-codex (pi)",
		"    3. gpt55     gpt-5.5",
	}
	assertRows(t, rows, want)
}

func TestPickerRowsPlacesDiscoveredClaudeChoicesBeforeOpenAI(t *testing.T) {
	rows := pickerRows(map[string]agentcfg.Config{
		"default": {Integration: "claude"},
		"opus":    {Integration: "claude", Model: "opus"},
		"fable":   {Integration: "claude", Model: "fable"},
		"sonnet":  {Integration: "claude", Model: "sonnet"},
		"haiku":   {Integration: "claude", Model: "haiku"},
		"sol56":   {Integration: "codex", Model: "gpt-5.6-sol"},
		"gpt55":   {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
	})

	want := []string{
		"Configured agents:",
		"",
		"  anthropic (claude)",
		"    1. default",
		"    2. fable     fable",
		"    3. haiku     haiku",
		"    4. opus      opus",
		"    5. sonnet    sonnet",
		"  openai (codex)",
		"    6. sol56     gpt-5.6-sol",
		"  openai-codex (pi)",
		"    7. gpt55     gpt-5.5",
	}
	assertRows(t, rows, want)
}

// Numbering runs over the same grouped order the resolver indexes, so the row
// an operator reads and the config they get can never disagree.
func TestPickAgentConfigByNumber(t *testing.T) {
	var out strings.Builder
	got, err := pickAgentConfig(pickerFixture(), strings.NewReader("2\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got != "sonnet5" {
		t.Errorf("got %q, want sonnet5", got)
	}
	if !strings.Contains(out.String(), "Spawn which agent?") {
		t.Errorf("prompt missing:\n%s", out.String())
	}
}

func TestPickAgentConfigByName(t *testing.T) {
	var out strings.Builder
	got, err := pickAgentConfig(pickerFixture(), strings.NewReader("gpt55\n"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got != "gpt55" {
		t.Errorf("got %q, want gpt55", got)
	}
}

func TestPickAgentConfigRejectsInvalidSelections(t *testing.T) {
	for _, input := range []string{"0\n", "99\n", "nope\n"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			var out strings.Builder
			got, err := pickAgentConfig(pickerFixture(), strings.NewReader(input), &out)
			if err == nil {
				t.Fatalf("invalid selection %q returned %q", input, got)
			}
			if !strings.Contains(err.Error(), "invalid selection") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPickAgentConfigCancels(t *testing.T) {
	for name, input := range map[string]string{"eof": "", "enter": "\n"} {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			got, err := pickAgentConfig(pickerFixture(), strings.NewReader(input), &out)
			if err == nil {
				t.Fatalf("cancel returned %q", got)
			}
			if !strings.Contains(err.Error(), "cancelled") {
				t.Errorf("unexpected error: %v", err)
			}
			if !strings.Contains(out.String(), "Spawn which agent?") {
				t.Errorf("prompt missing before cancel:\n%s", out.String())
			}
		})
	}
}

func TestAgentSpawnBareWithoutTerminalIsUsageError(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	stubStdinNotTerminal(t)
	out, err := captureRun(t, "agent", "spawn")
	if err == nil {
		t.Fatal("bare agent spawn succeeded without a terminal")
	}
	if !strings.Contains(err.Error(), "choose an agent, --profile, or --model") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Configured agents:") {
		t.Errorf("menu shown without a terminal:\n%s", out)
	}
}

func TestAgentSpawnPickerProfileAgnosticAgentNeedsProfiles(t *testing.T) {
	writePickerWorkspace(t, map[string]agentcfg.Config{})
	stubStdinTerminal(t)
	withStdin(t, "1\n")
	out, err := captureRun(t, "agent", "spawn")
	if err == nil {
		t.Fatal("bare agent spawn succeeded with an empty catalog")
	}
	if !strings.Contains(err.Error(), "no profiles are configured") {
		t.Errorf("error missing hint: %v", err)
	}
	if !strings.Contains(out, agentcfg.ReservedOrchestrator) {
		t.Errorf("managed agent missing from menu:\n%s", out)
	}
}

// Cancelling is a runtime outcome, so it carries no help page.
func TestAgentSpawnPickerCancelIsRuntimeError(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	stubStdinTerminal(t)
	withStdin(t, "\n")
	_, err := captureRun(t, "agent", "spawn")
	if err == nil {
		t.Fatal("cancelled agent spawn succeeded")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "usage:") {
		t.Errorf("runtime error unexpectedly contains help:\n%s", err)
	}
}

// A picked name rejoins the normal spawn path, so the next thing that fails is
// the flock lookup every spawn does — not the exactly-one-of check.
func TestAgentSpawnPickerSelectionRejoinsSpawnPath(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	stubStdinTerminal(t)
	withStdin(t, "1\n1\n")
	t.Setenv("FLEDGE_FLOCK", "")
	out, err := captureRun(t, "agent", "spawn")
	if err == nil {
		t.Fatal("agent spawn without flock context succeeded")
	}
	if !strings.Contains(err.Error(), "FLEDGE_FLOCK") {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(out, agentcfg.ReservedOrchestrator) || !strings.Contains(out, "1. opus48") {
		t.Errorf("agent/profile menus missing:\n%s", out)
	}
}

// --provider without --model stays the same usage error it was, terminal or
// not: the picker only fires when no launch flag was given at all.
func TestAgentSpawnProviderWithoutModelSkipsPicker(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	stubStdinTerminal(t)
	withStdin(t, "1\n")
	out, err := captureRun(t, "agent", "spawn", "--provider", "openai-codex")
	if err == nil {
		t.Fatal("agent spawn --provider without --model succeeded")
	}
	if !strings.Contains(err.Error(), "--provider only applies to --model") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Configured agents:") {
		t.Errorf("menu shown for --provider without --model:\n%s", out)
	}
}

// --integration without --model has nothing to override; like --provider it
// skips the picker and keeps the usage error.
func TestAgentSpawnIntegrationWithoutModelSkipsPicker(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	stubStdinTerminal(t)
	withStdin(t, "1\n")
	out, err := captureRun(t, "agent", "spawn", "--integration", "codex")
	if err == nil {
		t.Fatal("agent spawn --integration without --model succeeded")
	}
	if !strings.Contains(err.Error(), "--integration only applies to --model") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Configured agents:") {
		t.Errorf("menu shown for --integration without --model:\n%s", out)
	}
}

func TestAgentSpawnIntegrationWithConfigRejected(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	_, err := captureRun(t, "agent", "spawn", "opus48", "--integration", "codex")
	if err == nil {
		t.Fatal("agent spawn <config> --integration succeeded")
	}
	if !strings.Contains(err.Error(), "--integration only applies to --model") {
		t.Errorf("unexpected error: %v", err)
	}
}

func assertRows(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

// The catalog groups by provider, alphabetically, and names anthropic as the
// claude integration's provider even though no config carries it: the claude
// CLI reaches exactly one vendor, so a blank column would read as unknown.
func TestModelRowsGroupsByProvider(t *testing.T) {
	rows := modelRows(map[string]agentcfg.Config{
		"glm52":   {Integration: "pi", Provider: "opencode-go", Model: "glm-5.2"},
		"opus48":  {Integration: "claude", Model: "claude-opus-4-8"},
		"bare":    {Integration: "claude"},
		"gpt55":   {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
		"sol56cx": {Integration: "codex", Model: "gpt-5.6-sol"},
		"pickle":  {Integration: "pi", Provider: "opencode", Model: "big-pickle"},
	})

	want := []string{
		"NAME     INTEGRATION  PROVIDER      MODEL",
		"bare     claude       anthropic",
		"opus48   claude       anthropic     claude-opus-4-8",
		"",
		"sol56cx  codex        openai        gpt-5.6-sol",
		"",
		"gpt55    pi           openai-codex  gpt-5.5",
		"",
		"glm52    pi           opencode-go   glm-5.2",
		"",
		"pickle   pi           opencode-zen  big-pickle",
	}
	assertRows(t, rows, want)
}

// JSON output carries the same derived provider and grouped order the table
// shows, so the two renderings never disagree about what a model is.
func TestModelEntriesMatchTableOrderAndProvider(t *testing.T) {
	entries := modelEntries(map[string]agentcfg.Config{
		"glm52":   {Integration: "pi", Provider: "opencode-go", Model: "glm-5.2"},
		"opus48":  {Integration: "claude", Model: "claude-opus-4-8"},
		"gpt55":   {Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
		"sol56cx": {Integration: "codex", Model: "gpt-5.6-sol"},
		"pickle":  {Integration: "pi", Provider: "opencode", Model: "big-pickle"},
	})

	want := []modelEntry{
		{Name: "opus48", Integration: "claude", Provider: "anthropic", Model: "claude-opus-4-8"},
		{Name: "sol56cx", Integration: "codex", Provider: "openai", Model: "gpt-5.6-sol"},
		{Name: "gpt55", Integration: "pi", Provider: "openai-codex", Model: "gpt-5.5"},
		{Name: "glm52", Integration: "pi", Provider: "opencode-go", Model: "glm-5.2"},
		{Name: "pickle", Integration: "pi", Provider: "opencode-zen", Model: "big-pickle"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d:\n got %+v\nwant %+v", i, entries[i], want[i])
		}
	}
}

// --headless is gone: an interactive start must always end in an orchestrator,
// so there is no longer a flag that skips the attach.
func TestStartRejectsRemovedHeadlessFlag(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	_, err := captureRun(t, "start", "--headless")
	if err == nil {
		t.Fatal("start --headless succeeded")
	}
	if !strings.Contains(err.Error(), `unknown flag "--headless"`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartRejectsRemovedHeadlessShortFlag(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())
	_, err := captureRun(t, "start", "-B")
	if err == nil {
		t.Fatal("start -B succeeded")
	}
	if !strings.Contains(err.Error(), `unknown flag "-B"`) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReservedOrchestratorListsAsManagedType(t *testing.T) {
	writePickerWorkspace(t, pickerFixture())

	out, err := captureRun(t, "agent", "types")
	if err != nil {
		t.Fatalf("agent types: %v", err)
	}
	if !strings.Contains(out, agentcfg.ReservedOrchestrator) {
		t.Errorf("agent types omits the managed definition:\n%s", out)
	}
}

// stubDiscovery pins init's discovery to fake Claude, Pi and Codex binaries
// with small constant catalogs, so init tests neither exec the real ones nor
// depend on what the machine has installed.
func stubDiscovery(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	scripts := map[string]string{
		"claude": "#!/bin/sh\ntest \"$1\" = --version || exit 2\nprintf '%s\\n' '2.1.0'\n",
		"pi":     "#!/bin/sh\ncat <<'EOF'\nprovider model\nopenai-codex gpt-5.5\nEOF\n",
		"codex":  "#!/bin/sh\ncat <<'EOF'\n{\"models\":[{\"slug\":\"gpt-5.6-sol\",\"visibility\":\"list\"}]}\nEOF\n",
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestInitAppendsToGitignore(t *testing.T) {
	stubDiscovery(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "init", dir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, "added "+strings.Join(scaffold.GitignoreEntries, ", ")+" to .gitignore") {
		t.Errorf("output missing gitignore log line: %q", out)
	}

	out, err = captureRun(t, "init", dir)
	if err != nil {
		t.Fatalf("re-init: %v", err)
	}
	if strings.Contains(out, ".gitignore") {
		t.Errorf("re-init logged a gitignore append with nothing to add: %q", out)
	}
}

func TestInitWritesCatalog(t *testing.T) {
	stubDiscovery(t)
	dir := t.TempDir()

	out, err := captureRun(t, "init", dir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, "wrote .fledge/agents/catalog.json (5 from claude, 1 from codex, 1 from pi)") {
		t.Errorf("output missing catalog log line: %q", out)
	}

	configs, err := agentcfg.Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := configs["gpt56solcx"]; got.Integration != "codex" || got.Model != "gpt-5.6-sol" {
		t.Errorf("gpt56solcx = %+v", got)
	}
	if got := configs["gpt55pi"]; got.Integration != "pi" || got.Provider != "openai-codex" {
		t.Errorf("gpt55pi = %+v", got)
	}
	if got := configs["defaultcl"]; got.Integration != "claude" || got.Model != "" || got.Provider != "" ||
		got.PermissionMode != "" || got.Sandbox != "" || len(got.Argv) != 0 || len(got.Env) != 0 {
		t.Errorf("defaultcl = %+v, want model-less native Claude launcher", got)
	}
	for name, model := range map[string]string{"opuscl": "opus", "fablecl": "fable", "sonnetcl": "sonnet", "haikucl": "haiku"} {
		got := configs[name]
		if got.Integration != "claude" || got.Model != model || got.Provider != "" ||
			got.PermissionMode != "" || got.Sandbox != "" || len(got.Argv) != 0 || len(got.Env) != 0 {
			t.Errorf("%s = %+v, want native Claude family launcher", name, got)
		}
	}
	// Discovery writes the catalog, never the operator's empty agents.json.
	userData, err := os.ReadFile(filepath.Join(dir, scaffold.DirName, agentcfg.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(userData), `"version": 1`) || !strings.Contains(string(userData), `"profiles": {}`) {
		t.Errorf("agents.json changed during discovery: %q", userData)
	}
}

func TestInitKeepsCatalogWhenNothingAnswers(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	sentinel := `{"version":1,"agents":{},"profiles":{"kept":{"integration":"codex","model":"gpt-5.6-sol"}}}`
	catalogPath := filepath.Join(dir, scaffold.DirName, agentcfg.CatalogName)
	if err := os.WriteFile(catalogPath, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	// A PATH with no claude, pi or codex: discovery skips all three without
	// failing init.
	t.Setenv("PATH", t.TempDir())

	out, err := captureRun(t, "init", dir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, "left as it was") {
		t.Errorf("output missing the kept-catalog line: %q", out)
	}
	if !strings.Contains(out, "note: claude is not on PATH; skipped") {
		t.Errorf("output missing the Claude discovery note: %q", out)
	}
	got, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("empty discovery clobbered the catalog: %q", got)
	}
}

func TestInitJSON(t *testing.T) {
	stubDiscovery(t)
	dir := t.TempDir()

	out, err := captureRun(t, "init", dir, "--json")
	if err != nil {
		t.Fatalf("init --json: %v", err)
	}
	var summary struct {
		Root           string         `json:"root"`
		Existed        bool           `json:"existed"`
		CatalogWritten bool           `json:"catalog_written"`
		Models         map[string]int `json:"models"`
		Notes          []string       `json:"notes"`
	}
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if !summary.CatalogWritten || summary.Models["claude"] != 5 || summary.Models["codex"] != 1 || summary.Models["pi"] != 1 {
		t.Errorf("summary = %+v", summary)
	}
	if summary.Existed {
		t.Errorf("fresh init reported existed = true")
	}
}

// A start that created the session must tear it down when the daemon never
// comes up, or `fledge start` exits 1 with a stranded herdr server.
func TestGuardedBringUpTearsDownCreatedSessionOnSpawnFailure(t *testing.T) {
	var torn []string
	teardown := func(session string) error { torn = append(torn, session); return nil }
	boom := errors.New("daemon did not come up")
	err := guardedBringUp("fledge-ws-flock", true, teardown, func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to carry the spawn failure", err)
	}
	if len(torn) != 1 || torn[0] != "fledge-ws-flock" {
		t.Fatalf("teardown calls = %v, want one for the created session", torn)
	}
}

// A workspace-create failure is also pre-daemon, so it tears the session down
// too and never reaches the spawn step.
func TestGuardedBringUpTearsDownBeforeSpawnOnWorkspaceFailure(t *testing.T) {
	var torn []string
	teardown := func(session string) error { torn = append(torn, session); return nil }
	wsErr := errors.New("create workspace in session")
	spawnRan := false
	err := guardedBringUp("fledge-ws-flock", true, teardown,
		func() error { return wsErr },
		func() error { spawnRan = true; return nil },
	)
	if !errors.Is(err, wsErr) {
		t.Fatalf("err = %v, want the workspace failure", err)
	}
	if spawnRan {
		t.Fatal("spawn step ran after the workspace step failed")
	}
	if len(torn) != 1 || torn[0] != "fledge-ws-flock" {
		t.Fatalf("teardown calls = %v, want one for the created session", torn)
	}
}

// A reused operator session (created == false) belongs to the operator, so a
// failed bring-up leaves it running rather than killing it out from under them.
func TestGuardedBringUpLeavesReusedSessionRunning(t *testing.T) {
	var torn []string
	teardown := func(session string) error { torn = append(torn, session); return nil }
	boom := errors.New("daemon did not come up")
	err := guardedBringUp("operator-session", false, teardown, func() error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the spawn failure", err)
	}
	if len(torn) != 0 {
		t.Fatalf("tore down a reused operator session: %v", torn)
	}
}

// A spawn failure that lands just after the Herdr attach returns must still fail
// the start: dropping it exits 0 over a flock the goroutine already rolled back.
func TestAwaitSpawnPropagatesLateSpawnFailure(t *testing.T) {
	spawned := make(chan error, 1)
	spawnErr := errors.New("spawn failed")
	go func() {
		time.Sleep(20 * time.Millisecond)
		spawned <- spawnErr
	}()
	abort := func(root, name string, cause error) error {
		t.Fatalf("abort called though the attach succeeded: %v", cause)
		return nil
	}
	err := awaitSpawn("/root", "flock", nil, spawned, abort)
	if !errors.Is(err, spawnErr) {
		t.Fatalf("err = %v, want it to carry the late spawn failure", err)
	}
}

// A failed attach never handed off to the goroutine, so awaitSpawn rolls back
// through abort without blocking on the channel that will never be written.
func TestAwaitSpawnRollsBackOnAttachFailure(t *testing.T) {
	attachErr := errors.New("attach failed")
	spawned := make(chan error, 1) // deliberately never written
	aborted := false
	abort := func(root, name string, cause error) error {
		aborted = true
		return cause
	}
	err := awaitSpawn("/root", "flock", attachErr, spawned, abort)
	if !aborted {
		t.Fatal("attach failure did not roll the start back")
	}
	if !errors.Is(err, attachErr) {
		t.Fatalf("err = %v, want the attach failure", err)
	}
}

// The clean path: attach succeeds and the spawn reports no error.
func TestAwaitSpawnSucceedsWhenSpawnSucceeds(t *testing.T) {
	spawned := make(chan error, 1)
	spawned <- nil
	err := awaitSpawn("/root", "flock", nil, spawned, func(_, _ string, cause error) error {
		t.Fatalf("abort called on success: %v", cause)
		return nil
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

// When both a buffered spawn failure and an attach error are present, the spawn
// failure wins: the goroutine that reported it has already rolled the flock
// back, so it is the real cause and abort must not tear down a second time.
func TestAwaitSpawnBufferedSpawnErrorWinsOverAttachError(t *testing.T) {
	spawnErr := errors.New("spawn failed")
	attachErr := errors.New("attach wait error")
	spawned := make(chan error, 1)
	spawned <- spawnErr
	abortCalled := false
	abort := func(root, name string, cause error) error {
		abortCalled = true
		return cause
	}
	err := awaitSpawn("/root", "flock", attachErr, spawned, abort)
	if !errors.Is(err, spawnErr) {
		t.Fatalf("err = %v, want the buffered spawn failure to win", err)
	}
	if abortCalled {
		t.Fatal("abort ran though the goroutine already reported and rolled back")
	}
}
