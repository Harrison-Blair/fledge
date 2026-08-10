package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/tui"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

const testSessionName = "fledge-00000000000000000000000000000000"

func TestStartCreatesAndReusesSession(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "My Project")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	wantSessionName := "fledge-my-project-00000000"

	if err := manager.Start(context.Background(), root, StartOptions{
		Harness: "codex", HarnessSet: true, Model: "custom/model", ModelSet: true,
		NativeArgs: []string{"--approval-policy", "never"}, Timeout: 45 * time.Second,
	}); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	wantFreshCalls := []string{"check", "list", "start-server", "wait-ready", "rename-tab", "rename-pane", "split-pane", "start-agent", "focus-agent", "attach"}
	if strings.Join(client.calls, ",") != strings.Join(wantFreshCalls, ",") {
		t.Fatalf("fresh call order = %v, want %v", client.calls, wantFreshCalls)
	}
	wantNativePrefix := []string{"--model", "custom/model", "--approval-policy", "never"}
	if len(client.startAgent.args) < len(wantNativePrefix) || strings.Join(client.startAgent.args[:len(wantNativePrefix)], "\x00") != strings.Join(wantNativePrefix, "\x00") {
		t.Errorf("native args = %#v, want prefix %#v", client.startAgent.args, wantNativePrefix)
	}
	if client.startAgent.name != "orchestrator" || client.startAgent.kind != "codex" || client.startAgent.pane != "w1:p1" || client.startAgent.timeout != 45*time.Second {
		t.Errorf("StartAgent() = %#v", client.startAgent)
	}
	wantInstructions := project.DefaultOrchestratorInstructions + "\n\n" + codexCoordinatorGuidance + "\n\n" + mandatoryCoordinatorCommunicationPolicy
	if len(client.startAgent.args) != 6 || client.startAgent.args[4] != "-c" || !strings.Contains(client.startAgent.args[5], wantInstructions[:40]) {
		t.Errorf("orchestrator native args = %#v, want final developer_instructions override", client.startAgent.args)
	}
	if len(client.promptCalls) != 0 {
		t.Errorf("orchestrator PromptAgent calls = %#v, want none", client.promptCalls)
	}
	client.sessions = []herdr.Session{{Name: wantSessionName, Running: true}}
	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}

	if len(client.attachCalls) != 2 {
		t.Fatalf("Attach() calls = %d, want 2", len(client.attachCalls))
	}
	for _, call := range client.attachCalls {
		if call.name != wantSessionName {
			t.Errorf("Attach() name = %q, want %q", call.name, wantSessionName)
		}
		if call.dir != root {
			t.Errorf("Attach() dir = %q, want %q", call.dir, root)
		}
	}

	value, found, err := readRecord(root)
	if err != nil {
		t.Fatalf("readRecord() error = %v", err)
	}
	if !found || value.SessionName != wantSessionName {
		t.Fatalf("readRecord() = %#v, %v", value, found)
	}

	recordInfo, err := os.Stat(recordPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if permissions := recordInfo.Mode().Perm(); permissions != 0o600 {
		t.Errorf("record permissions = %o, want 600", permissions)
	}
	for _, name := range []string{openCodeInstructionsFile, openCodeEnvironmentFile} {
		if _, err := os.Stat(filepath.Join(root, stateDirectory, "logs", wantSessionName, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("non-OpenCode runtime artifact %s error = %v, want absent", name, err)
		}
	}
	logEntries, err := os.ReadDir(fsutil.Session(root, wantSessionName))
	if err != nil {
		t.Fatal(err)
	}
	var logNames []string
	for _, entry := range logEntries {
		logNames = append(logNames, entry.Name())
	}
	if strings.Join(logNames, ",") != "events.jsonl,fledge.log" {
		t.Fatalf("session log entries = %v, want only actual logs", logNames)
	}
	tmpEntries, err := os.ReadDir(fsutil.TempSession(root, wantSessionName))
	if err != nil || len(tmpEntries) != 1 || tmpEntries[0].Name() != "events.lock" {
		t.Fatalf("session temp entries = %v, %v; want events.lock", tmpEntries, err)
	}

	ignore, err := os.ReadFile(filepath.Join(root, stateDirectory, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != ignoreContents {
		t.Errorf(".gitignore = %q, want %q", ignore, ignoreContents)
	}
}

func TestOrchestratorInstructionsAppendMandatoryPolicyAcrossHarnesses(t *testing.T) {
	t.Parallel()

	const custom = "Keep these custom coordinator instructions."
	for _, harnessID := range []string{"claude", "codex", "pi", "opencode"} {
		got := orchestratorInstructions(custom, harnessID)
		wantPrefix := custom + "\n\n"
		if harnessID == "codex" {
			wantPrefix += codexCoordinatorGuidance + "\n\n"
		}
		if got != wantPrefix+mandatoryCoordinatorCommunicationPolicy {
			t.Errorf("%s prompt = %q", harnessID, got)
		}
		if strings.Index(got, mandatoryCoordinatorCommunicationPolicy) <= strings.Index(got, custom) {
			t.Errorf("%s mandatory policy does not follow custom profile: %q", harnessID, got)
		}
	}
}

func TestOrchestratorInstructionsIncludeModelCatalogGuidanceAfterCustomProfile(t *testing.T) {
	t.Parallel()

	const custom = "Existing custom coordinator profile."
	for _, harnessID := range []string{"claude", "codex", "pi", "opencode"} {
		instructions := orchestratorInstructions(custom, harnessID)
		for _, required := range []string{
			"fledge agent models [harness]",
			"fledge agent spawn --model <exact-model-value>",
			"catalog entries are advisory",
		} {
			if !strings.Contains(instructions, required) {
				t.Errorf("%s instructions = %q, want %q", harnessID, instructions, required)
			}
		}
		if strings.Index(instructions, "fledge agent models [harness]") <= strings.Index(instructions, custom) {
			t.Errorf("%s model guidance does not follow the existing custom profile: %q", harnessID, instructions)
		}
	}
}

func TestStartLeavesClaudeProfileUnchangedAndAppendsDurableInstructions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	profilePath := filepath.Join(root, ".fledge", "profiles", "orchestrator.toml")
	const profileContents = "schema_version = 1\ninstructions = 'custom Claude instructions'\n"
	if err := os.WriteFile(profilePath, []byte(profileContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codex", "rules", "fledge.rules"), []byte("# custom Codex rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = func(name string) (string, error) {
		if name == "claude" {
			return "/test/claude", nil
		}
		return "", os.ErrNotExist
	}

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "claude", HarnessSet: true}); err != nil {
		t.Fatal(err)
	}
	wantInstructions := "custom Claude instructions\n\n" + mandatoryCoordinatorCommunicationPolicy
	promptReference := generatedPrompt(t, root, wantInstructions)
	wantArgs := []string{"--permission-mode", "bypassPermissions", "--append-system-prompt-file", promptReference}
	if strings.Join(client.startAgent.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("Claude args = %#v, want %#v", client.startAgent.args, wantArgs)
	}
	if len(client.promptCalls) != 0 {
		t.Errorf("orchestrator PromptAgent calls = %#v, want none", client.promptCalls)
	}
	generated, err := os.ReadFile(generatedPromptFile(root))
	if err != nil || string(generated) != wantInstructions {
		t.Fatalf("generated prompt = %q, %v; want exact rendered instructions", generated, err)
	}
	for _, arg := range client.startAgent.args {
		if strings.ContainsAny(arg, "\n\r\t") {
			t.Fatalf("Claude argument contains a control character: %q", arg)
		}
	}
	contentsAfter, err := os.ReadFile(profilePath)
	if err != nil || string(contentsAfter) != profileContents {
		t.Fatalf("profile after Start() = %q, %v; want unchanged", contentsAfter, err)
	}
}

func TestStartPiUsesControlCharacterFreeGeneratedPromptArgument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = func(name string) (string, error) {
		if name == "pi" {
			return "/test/pi", nil
		}
		return "", os.ErrNotExist
	}

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "pi", HarnessSet: true}); err != nil {
		t.Fatal(err)
	}
	want := project.DefaultOrchestratorInstructions + "\n\n" + mandatoryCoordinatorCommunicationPolicy
	wantArgs := []string{"--append-system-prompt", generatedPrompt(t, root, want)}
	if strings.Join(client.startAgent.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Pi args = %#v, want %#v", client.startAgent.args, wantArgs)
	}
	for _, arg := range client.startAgent.args {
		if strings.ContainsAny(arg, "\n\r\t") {
			t.Fatalf("Pi argument contains a control character: %q", arg)
		}
	}
	contents, err := os.ReadFile(generatedPromptFile(root))
	if err != nil || string(contents) != want {
		t.Fatalf("generated prompt = %q, %v; want exact multiline policy", contents, err)
	}
}

func TestStartBackfillsCodexRulesBeforeLaunch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	rulesPath := filepath.Join(root, ".codex", "rules", "fledge.rules")
	if err := os.Remove(rulesPath); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{snapshot: testSnapshot()}
	client.startHook = func() {
		if _, err := os.Stat(rulesPath); err != nil {
			t.Errorf("rule at StartAgent() = %v, want installed", err)
		}
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err != nil {
		t.Fatal(err)
	}
}

func TestStartRejectsConflictingCodexRulesBeforeSessionCreation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	rulesPath := filepath.Join(root, ".codex", "rules", "fledge.rules")
	const contents = "# custom rules\n"
	if err := os.WriteFile(rulesPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness

	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("Start() error = %v, want Codex rule conflict", err)
	}
	if slicesContain(client.calls, "start-server") || slicesContain(client.calls, "start-agent") {
		t.Fatalf("calls = %v, want no session or agent launch", client.calls)
	}
	contentsAfter, readErr := os.ReadFile(rulesPath)
	if readErr != nil || string(contentsAfter) != contents {
		t.Fatalf("rules after conflict = %q, %v; want preserved", contentsAfter, readErr)
	}
}

func TestStartRejectsLegacyCodexRulesWithInitGuidance(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	rulesPath := filepath.Join(root, ".codex", "rules", "fledge.rules")
	legacyContents := writeLegacyCodexRules(t, rulesPath)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness

	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
	if err == nil || !strings.Contains(err.Error(), "run fledge init") {
		t.Fatalf("Start() error = %v, want run-fledge-init guidance", err)
	}
	if slicesContain(client.calls, "start-server") || slicesContain(client.calls, "start-agent") {
		t.Fatalf("calls = %v, want no session or agent launch", client.calls)
	}
	contentsAfter, readErr := os.ReadFile(rulesPath)
	if readErr != nil || string(contentsAfter) != legacyContents {
		t.Fatalf("rules after legacy rejection = %q, %v; want preserved", contentsAfter, readErr)
	}
}

func TestStartCreatesWorkspaceForEmptyHeadlessServer(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{
		createdWorkspace: herdr.Workspace{WorkspaceID: "w1"},
		createdTab:       herdr.Tab{TabID: "w1:t1", WorkspaceID: "w1"},
		createdPane:      herdr.Pane{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1"},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.authorityRandom = bytes.NewReader(bytes.Repeat([]byte{0xcd}, paneAuthorityBytes))

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{"check", "list", "start-server", "wait-ready", "create-workspace", "rename-tab", "rename-pane", "split-pane", "start-agent", "focus-agent", "attach"}
	if strings.Join(client.calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("call order = %v, want %v", client.calls, wantCalls)
	}
	if client.createWorkspace.dir != root || client.createWorkspace.label != "orchestrator" {
		t.Fatalf("CreateWorkspace() call = %#v", client.createWorkspace)
	}
	wantAuthority := strings.Repeat("cd", paneAuthorityBytes)
	if client.serverEnvironment[paneAuthorityEnvironment] != wantAuthority {
		t.Fatalf("server pane authority was not injected")
	}
	if client.splitEnvironment[paneAuthorityEnvironment] != "" {
		t.Fatalf("control pane inherited orchestrator authority")
	}
	value, found, err := readRecord(root)
	if err != nil || !found {
		t.Fatalf("readRecord() = %#v, %v, %v", value, found, err)
	}
	agent, err := messaging.New(root, value.SessionName).Agent(orchestratorIdentity)
	if err != nil || agent.AuthorityHash != paneAuthorityHash(wantAuthority) {
		t.Fatalf("orchestrator authority = %q, %v", agent.AuthorityHash, err)
	}
}

func TestStartRejectsSelectionFlagsWhenReattaching(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	if err := initializeStateDirectory(root); err != nil {
		t.Fatal(err)
	}
	// An unbound legacy record: the pre-lock reattach must reject the selection
	// flags before binding, so MessagingSessionID stays empty. If binding ran
	// first, a durable session ID would be written even though Start errors.
	if created, err := createRecord(root, record{Version: recordVersion, SessionName: testSessionName}); err != nil || !created {
		t.Fatalf("createRecord() = %v, %v", created, err)
	}
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{})

	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Model: "gpt-custom", ModelSet: true})
	if err == nil || !strings.Contains(err.Error(), "cannot be used when reattaching") {
		t.Fatalf("Start() error = %v", err)
	}
	if len(client.attachCalls) != 0 {
		t.Fatalf("Attach() calls = %v, want none", client.attachCalls)
	}
	value, found, err := readRecord(root)
	if err != nil || !found {
		t.Fatalf("readRecord() = %v, %v", found, err)
	}
	if value.MessagingSessionID != "" {
		t.Fatalf("MessagingSessionID = %q, want empty (pre-lock reject must not bind)", value.MessagingSessionID)
	}
	if _, found, err := readPreferences(root); err != nil || found {
		t.Fatalf("preferences after rejected reattach = %v, %v; want none", found, err)
	}
}

// TestStartPostLockReattachBindsBeforeRejectingSelection covers the second
// reattach path: the session is stopped at the first List and running at the
// second (it started up between the two checks under the startup lock). That
// path intentionally binds durable state and releases the lock before it
// validates the selection flags, so Start still errors on the flags but leaves
// the record bound.
func TestStartPostLockReattachBindsBeforeRejectingSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	if err := initializeStateDirectory(root); err != nil {
		t.Fatal(err)
	}
	sessionID, err := messaging.New(root, testSessionName).Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if created, err := createRecord(root, record{Version: recordVersion, SessionName: testSessionName}); err != nil || !created {
		t.Fatalf("createRecord() = %v, %v", created, err)
	}
	client := &fakeHerdr{listSequence: [][]herdr.Session{
		{{Name: testSessionName, Running: false}},
		{{Name: testSessionName, Running: true}},
	}}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	launches := 0
	manager.watchLauncher = func(string) error { launches++; return nil }

	err = manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
	if err == nil || !strings.Contains(err.Error(), "cannot be used when reattaching") {
		t.Fatalf("Start() error = %v", err)
	}
	if len(client.attachCalls) != 0 {
		t.Fatalf("Attach() calls = %v, want none", client.attachCalls)
	}
	if launches != 0 {
		t.Fatalf("watcher launches = %d, want 0 (selection rejected before the common tail)", launches)
	}
	value, found, err := readRecord(root)
	if err != nil || !found {
		t.Fatalf("readRecord() = %v, %v", found, err)
	}
	if value.MessagingSessionID != sessionID {
		t.Fatalf("MessagingSessionID = %q, want %q (post-lock path must bind before the selection check)", value.MessagingSessionID, sessionID)
	}
}

// TestStartPostLockReattachReleasesStartupLock covers the same post-lock
// reattach path as the test above, but asserts the branch releases the startup
// lock it owns before returning. The startup lock lives on the dedicated
// .fledge/session.lock file, whose inode is stable across every session.json
// rewrite, so re-acquiring the same lock after Start returns reliably observes
// whether unlock() actually ran regardless of any record binding. The record is
// pre-bound to keep the reattach binder a no-op, holding the test on the
// lock-release contract rather than record-binding behavior. A regression that
// binds but omits unlock() keeps the flock held, so the bounded re-acquire
// below blocks to its deadline and fails instead of succeeding at once.
func TestStartPostLockReattachReleasesStartupLock(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	if err := initializeStateDirectory(root); err != nil {
		t.Fatal(err)
	}
	sessionID, err := messaging.New(root, testSessionName).Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if created, err := createRecord(root, record{Version: recordVersion, SessionName: testSessionName, MessagingSessionID: sessionID}); err != nil || !created {
		t.Fatalf("createRecord() = %v, %v", created, err)
	}
	client := &fakeHerdr{listSequence: [][]herdr.Session{
		{{Name: testSessionName, Running: false}},
		{{Name: testSessionName, Running: true}},
	}}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.watchLauncher = func(string) error { return nil }

	err = manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
	if err == nil || !strings.Contains(err.Error(), "cannot be used when reattaching") {
		t.Fatalf("Start() error = %v", err)
	}

	// The post-lock reattach path must release the startup lock it owns before
	// returning. The dedicated session.lock inode is stable, so re-acquiring the
	// same lock proves release: a leaked unlock() would keep the flock held and
	// this bounded acquire would block to its deadline and fail.
	lockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	unlock, err := lockSessionRecord(lockCtx, root)
	if err != nil {
		t.Fatalf("re-acquire startup lock after reattach = %v; post-lock reattach path leaked the lock", err)
	}
	if err := unlock(); err != nil {
		t.Fatalf("unlock() = %v", err)
	}
}

func TestStartReusesLegacySessionName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{})

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if len(client.attachCalls) != 1 {
		t.Fatalf("Attach() calls = %d, want 1", len(client.attachCalls))
	}
	if call := client.attachCalls[0]; call.name != testSessionName || call.dir != root {
		t.Errorf("Attach() call = %#v, want name %q and dir %q", call, testSessionName, root)
	}

	value, found, err := readRecord(root)
	if err != nil || !found || value.SessionName != testSessionName {
		t.Fatalf("readRecord() = %#v, %v, %v; want unchanged legacy record", value, found, err)
	}
}

func TestStartLaunchesWatcherBeforeEveryAttachAndWarnsOnly(t *testing.T) {
	t.Parallel()

	t.Run("reattach", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeTestRecord(t, root)
		client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
		manager, output := newTestManager(client, &fakeConfirmer{})
		launched := 0
		manager.watchLauncher = func(gotRoot string) error {
			launched++
			if gotRoot != root || len(client.attachCalls) != 0 {
				t.Errorf("watch launcher root/attach state = %q/%v, want project root before Attach", gotRoot, client.attachCalls)
			}
			return errors.New("launcher failed")
		}
		if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
			t.Fatal(err)
		}
		if launched != 1 || len(client.attachCalls) != 1 {
			t.Fatalf("launcher/Attach calls = %d/%d, want 1/1", launched, len(client.attachCalls))
		}
		if !strings.Contains(output.String(), "Warning: watcher could not be started") {
			t.Errorf("output = %q, want warn-only launcher failure", output.String())
		}
	})

	t.Run("fresh start", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{snapshot: testSnapshot()}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness
		launched := 0
		manager.watchLauncher = func(gotRoot string) error {
			launched++
			if gotRoot != root || len(client.attachCalls) != 0 {
				t.Errorf("watch launcher root/attach state = %q/%v, want project root before Attach", gotRoot, client.attachCalls)
			}
			return nil
		}
		if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err != nil {
			t.Fatal(err)
		}
		if launched != 1 || len(client.attachCalls) != 1 {
			t.Fatalf("launcher/Attach calls = %d/%d, want 1/1", launched, len(client.attachCalls))
		}
	})
}

func TestStartReattachPreservesGeneratedPromptSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	generatedPrompt(t, root, "active session prompt")
	profilePath := filepath.Join(root, stateDirectory, "profiles", "orchestrator.toml")
	if err := os.WriteFile(profilePath, []byte("schema_version = 1\ninstructions = 'edited profile'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{})

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(generatedPromptFile(root))
	if err != nil || string(contents) != "active session prompt" {
		t.Fatalf("generated prompt after reattach = %q, %v; want active snapshot", contents, err)
	}
}

func TestStartRestartsStoppedSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: false}},
		snapshot: testSnapshot(),
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	wantCalls := []string{"check", "list", "list", "start-server", "wait-ready", "rename-tab", "rename-pane", "split-pane", "start-agent", "focus-agent", "attach"}
	if strings.Join(client.calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("call order = %v, want %v", client.calls, wantCalls)
	}
	if call := client.attachCalls[0]; call.name != testSessionName || call.dir != root {
		t.Errorf("Attach() call = %#v, want name %q and dir %q", call, testSessionName, root)
	}
	value, found, err := readRecord(root)
	if err != nil || !found || value.SessionName != testSessionName {
		t.Fatalf("readRecord() = %#v, %v, %v; want record reused", value, found, err)
	}
}

func TestRevalidateLockedRecordCleansUpCreatedRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		corrupt       bool
		sessionName   string
		recordCreated bool
		wantErr       bool
		wantRemoved   bool
	}{
		{name: "read failure removes created record", corrupt: true, sessionName: testSessionName, recordCreated: true, wantErr: true, wantRemoved: true},
		{name: "read failure preserves preexisting record", corrupt: true, sessionName: testSessionName, recordCreated: false, wantErr: true, wantRemoved: false},
		{name: "changed record is preserved", corrupt: false, sessionName: "fledge-11111111111111111111111111111111", recordCreated: true, wantErr: true, wantRemoved: false},
		{name: "matching record passes", corrupt: false, sessionName: testSessionName, recordCreated: false, wantErr: false, wantRemoved: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.corrupt {
				initTestProject(t, root)
				if err := initializeStateDirectory(root); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(recordPath(root), []byte("{not json"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				writeTestRecord(t, root)
			}

			err := revalidateLockedRecord(root, test.sessionName, test.recordCreated)
			if (err != nil) != test.wantErr {
				t.Fatalf("revalidateLockedRecord() error = %v, wantErr %v", err, test.wantErr)
			}
			_, statErr := os.Stat(recordPath(root))
			if removed := errors.Is(statErr, os.ErrNotExist); removed != test.wantRemoved {
				t.Fatalf("record removed = %v (stat error %v), want removed %v", removed, statErr, test.wantRemoved)
			}
		})
	}
}

func TestStartResetsMessagingOnlyForFreshServer(t *testing.T) {
	t.Parallel()

	t.Run("fresh server resets an old audit and drops legacy files", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		session := "fledge-" + sessionSlug(root) + "-00000000"
		store := messaging.New(root, session)
		if _, err := store.Initialize(); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "old", Body: "old", RecipientPane: "old-pane"}); err != nil {
			t.Fatal(err)
		}
		legacyLog := filepath.Join(root, stateDirectory, "messages.jsonl")
		if err := os.WriteFile(legacyLog, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		client := &fakeHerdr{snapshot: testSnapshot()}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err != nil {
			t.Fatal(err)
		}
		messages, err := messaging.New(root, session).List()
		if err != nil || len(messages) != 0 {
			t.Fatalf("fresh messages = %#v, %v", messages, err)
		}
		if _, err := os.Stat(legacyLog); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy messages.jsonl error = %v, want removed", err)
		}
	})

	t.Run("reattach preserves the active audit", func(t *testing.T) {
		root := t.TempDir()
		writeTestRecord(t, root)
		ignorePath := filepath.Join(root, stateDirectory, ".gitignore")
		if err := os.WriteFile(ignorePath, []byte("session.json\nkeep-local/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		store := messaging.New(root, testSessionName)
		sessionID, err := store.Initialize()
		if err != nil {
			t.Fatal(err)
		}
		if err := writeRecordSessionBinding(root, testSessionName, sessionID, true); err != nil {
			t.Fatal(err)
		}
		created, err := store.Create(messaging.CreateParams{Sender: "user", Recipient: "worker", Body: "keep", RecipientPane: "pane"})
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
		manager, _ := newTestManager(client, &fakeConfirmer{})

		if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
			t.Fatal(err)
		}
		preserved, err := messaging.New(root, testSessionName).Get(created.ID)
		if err != nil || preserved.Body != "keep" {
			t.Fatalf("preserved message = %#v, %v", preserved, err)
		}
		ignore, err := os.ReadFile(ignorePath)
		if err != nil || string(ignore) != "session.json\nkeep-local/\nsession.lock\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n" {
			t.Fatalf("reattach .gitignore = %q, %v", ignore, err)
		}
	})
}

func TestStartChecksHerdrBeforeWritingState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{checkErr: errors.New("not installed")}
	manager, _ := newTestManager(client, &fakeConfirmer{})

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err == nil {
		t.Fatal("Start() error = nil, want error")
	}
	if _, found, err := readRecord(root); err != nil || found {
		t.Fatalf("session record found = %v, error = %v; want no record", found, err)
	}
}

func TestStartRandomFailureDoesNotCreateRecord(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	randomErr := errors.New("random failed")
	client := &fakeHerdr{}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.random = errorReader{err: randomErr}

	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
	if !errors.Is(err, randomErr) {
		t.Fatalf("Start() error = %v, want %v", err, randomErr)
	}
	if len(client.attachCalls) != 0 {
		t.Errorf("Attach() calls = %d, want 0", len(client.attachCalls))
	}
	if client.listCalls != 1 || len(client.stopCalls) != 0 || len(client.deleteCalls) != 0 {
		t.Errorf("unexpected Herdr calls: List = %d, Stop = %v, Delete = %v", client.listCalls, client.stopCalls, client.deleteCalls)
	}
	if _, found, readErr := readRecord(root); readErr != nil || found {
		t.Errorf("readRecord() found = %v, error = %v; want no record", found, readErr)
	}
}

func TestStartFailureOwnership(t *testing.T) {
	t.Run("reattach failure preserves record", func(t *testing.T) {
		root := t.TempDir()
		writeTestRecord(t, root)
		client := &fakeHerdr{
			attachErr: errors.New("attach failed"),
			sessions:  []herdr.Session{{Name: testSessionName, Running: true}},
		}
		manager, _ := newTestManager(client, &fakeConfirmer{})

		if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err == nil {
			t.Fatal("Start() error = nil, want error")
		}
		if _, found, err := readRecord(root); err != nil || !found {
			t.Fatalf("record found = %v, error = %v; want preserved", found, err)
		}
	})

	t.Run("fresh initialization failure removes record", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{snapshot: herdr.Snapshot{Tabs: []herdr.Tab{{TabID: "t1"}}}}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
		if err == nil {
			t.Fatal("Start() error = nil, want layout error")
		}
		if _, found, readErr := readRecord(root); readErr != nil || found {
			t.Fatalf("record found = %v, error = %v; want removed", found, readErr)
		}
		if len(client.stopCalls) != 1 || len(client.deleteCalls) != 1 {
			t.Fatalf("cleanup calls = stop %v delete %v", client.stopCalls, client.deleteCalls)
		}
	})

	t.Run("delete failure during rollback preserves retry state", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{
			snapshot:  herdr.Snapshot{Tabs: []herdr.Tab{{TabID: "t1"}}},
			deleteErr: errors.New("delete failed"),
		}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
		if err == nil || !strings.Contains(err.Error(), "delete failed") {
			t.Fatalf("Start() error = %v, want delete failure surfaced", err)
		}
		if value, found, readErr := readRecord(root); readErr != nil || !found || value.SessionName == "" {
			t.Fatalf("record = %#v, found %v, error = %v; want preserved", value, found, readErr)
		}
		sessionLogs := filepath.Join(root, stateDirectory, "logs", "fledge-"+sessionSlug(root)+"-00000000")
		if _, statErr := os.Stat(sessionLogs); statErr != nil {
			t.Fatalf("session log directory stat error = %v, want preserved", statErr)
		}
		sessionTmp := fsutil.TempSession(root, "fledge-"+sessionSlug(root)+"-00000000")
		if _, statErr := os.Stat(sessionTmp); statErr != nil {
			t.Fatalf("session temp directory stat error = %v, want preserved", statErr)
		}
		if len(client.stopCalls) != 1 || len(client.deleteCalls) != 1 {
			t.Fatalf("cleanup calls = stop %v delete %v", client.stopCalls, client.deleteCalls)
		}
	})

	t.Run("server start failure removes record without deleting an unowned session", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{serverErr: errors.New("server failed")}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness

		err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true})
		if err == nil || !strings.Contains(err.Error(), "server failed") {
			t.Fatalf("Start() error = %v", err)
		}
		if len(client.stopCalls) != 0 || len(client.deleteCalls) != 0 {
			t.Fatalf("cleanup touched unowned server: stop %v delete %v", client.stopCalls, client.deleteCalls)
		}
		if _, found, readErr := readRecord(root); readErr != nil || found {
			t.Fatalf("record found = %v, error = %v; want removed", found, readErr)
		}
	})

	t.Run("canceled initialization uses a live cleanup context", func(t *testing.T) {
		root := t.TempDir()
		initTestProject(t, root)
		client := &fakeHerdr{waitErr: context.Canceled}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := manager.Start(ctx, root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "codex", HarnessSet: true}); err == nil {
			t.Fatal("Start() error = nil")
		}
		if client.stopCtxErr != nil {
			t.Fatalf("cleanup Stop() context error = %v", client.stopCtxErr)
		}
	})
}

func TestSpawnCreatesDedicatedTabAndPrompts(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	child := filepath.Join(root, "nested")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    testSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1", Label: "worker"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.getenv = func(string) string { return "" }
	manager.authorityRandom = bytes.NewReader(bytes.Repeat([]byte{0xab}, paneAuthorityBytes))
	launched := 0
	manager.watchLauncher = func(gotRoot string) error {
		launched++
		if gotRoot != root || len(client.promptCalls) != 1 {
			t.Errorf("watch launcher root/prompt state = %q/%v, want project root after successful prompt", gotRoot, client.promptCalls)
		}
		return nil
	}

	err := manager.Spawn(context.Background(), child, SpawnOptions{
		Name: "worker", Harness: "codex", Model: "gpt-custom", ModelSet: true,
		Timeout: 60 * time.Second, Task: "Review the diff", NativeArgs: []string{"--sandbox", "read-only"},
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	wantCalls := []string{"check", "list", "snapshot", "create-tab", "rename-pane", "start-agent", "focus-agent", "prompt-agent"}
	if strings.Join(client.calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("call order = %v, want %v", client.calls, wantCalls)
	}
	if client.createCall.cwd != root || client.createCall.label != "worker" || client.createCall.workspace != "w1" {
		t.Errorf("CreateTab() = %#v", client.createCall)
	}
	wantAuthority := strings.Repeat("ab", paneAuthorityBytes)
	if got := client.createCall.environment[paneAuthorityEnvironment]; got != wantAuthority {
		t.Errorf("pane authority = %q, want deterministic token", got)
	}
	registered, err := messaging.New(root, testSessionName).Agent("worker")
	if err != nil || registered.AuthorityHash != paneAuthorityHash(wantAuthority) {
		t.Errorf("registered authority = %q, %v", registered.AuthorityHash, err)
	}
	if client.startAgent.pane != "w1:p2" || client.startAgent.kind != "codex" || client.startAgent.timeout != 60*time.Second {
		t.Errorf("StartAgent() = %#v", client.startAgent)
	}
	wantArgs := []string{"--model", "gpt-custom", "--sandbox", "read-only"}
	if strings.Join(client.startAgent.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("native args = %#v, want %#v", client.startAgent.args, wantArgs)
	}
	wantPrompt := expectedWorkerPrompt(root, "worker", "Review the diff")
	if len(client.promptCalls) != 1 || client.prompt != wantPrompt {
		t.Errorf("PromptAgent calls = %#v, want one call with %q", client.promptCalls, wantPrompt)
	}
	if launched != 1 {
		t.Errorf("watch launcher calls = %d, want 1", launched)
	}
}

func TestRuntimeCommunicationPoliciesRequireFledgeCompletionAndForbidPolling(t *testing.T) {
	t.Parallel()

	for name, policy := range map[string]string{
		"orchestrator": mandatoryCoordinatorCommunicationPolicy,
		"worker":       agentMessagingContext,
	} {
		for _, required := range []string{
			"fledge agent message send",
			"fledge agent message reply",
			"task",
			"Never poll with fledge agent message inbox",
			"Herdr API snapshots",
		} {
			if !strings.Contains(policy, required) {
				t.Errorf("%s policy = %q, want containing %q", name, policy, required)
			}
		}
	}
	for _, required := range []string{
		"--can-delegate", "--parent-task", "Ordinary messages always wake",
		"herdr agent wait, read, get, list, prompt, send-keys, attach, and explain",
	} {
		if !strings.Contains(mandatoryCoordinatorCommunicationPolicy, required) {
			t.Errorf("orchestrator policy = %q, want %q", mandatoryCoordinatorCommunicationPolicy, required)
		}
	}
	for _, required := range []string{
		"A terminal transition (complete/fail) delivers its detail to the assigner",
		"do NOT also message the same summary",
		"herdr agent wait/read/get/list/prompt/send-keys/",
		"attach/explain",
	} {
		if !strings.Contains(agentMessagingContext, required) {
			t.Errorf("worker policy = %q, want containing %q", agentMessagingContext, required)
		}
	}
	if strings.Contains(agentMessagingContext, "completion summary") {
		t.Errorf("worker policy = %q, must not instruct sending a completion summary message", agentMessagingContext)
	}
}

func TestSpawnPromptCompositionAcrossHarnesses(t *testing.T) {
	t.Parallel()

	promptCases := []struct {
		name   string
		prompt string
	}{
		{name: "absent"},
		{name: "present", prompt: "Review the diff"},
	}
	for _, harnessID := range []string{"claude", "codex", "pi", "opencode"} {
		for _, promptCase := range promptCases {
			t.Run(harnessID+"/"+promptCase.name, func(t *testing.T) {
				root := t.TempDir()
				writeTestRecord(t, root)
				client := &fakeHerdr{
					sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
					snapshot:    testSnapshot(),
					createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1", Label: "worker"},
					createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
				}
				manager, _ := newTestManager(client, &fakeConfirmer{})
				manager.lookPath = func(name string) (string, error) {
					if name == harnessID {
						return "/test/" + name, nil
					}
					return "", os.ErrNotExist
				}
				manager.getenv = func(string) string { return "" }

				if err := manager.Spawn(context.Background(), root, SpawnOptions{
					Timeout: DefaultAgentTimeout, Name: "worker", Harness: harnessID, Task: promptCase.prompt,
				}); err != nil {
					t.Fatalf("Spawn() error = %v", err)
				}
				want := promptCall{session: testSessionName, recipient: "worker", prompt: expectedWorkerPrompt(root, "worker", promptCase.prompt)}
				if len(client.promptCalls) != 1 || client.promptCalls[0] != want {
					t.Fatalf("PromptAgent calls = %#v, want exactly %#v", client.promptCalls, want)
				}
			})
		}
	}
}

func TestStartOpenCodeUsesDurableSnapshotAndIsolatesControlPane(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = func(name string) (string, error) {
		if name == "opencode" {
			return "/test/opencode", nil
		}
		return "", os.ErrNotExist
	}
	const original = `{"instructions":["AGENTS.md"],"theme":"dark"}`
	manager.getenv = func(name string) string {
		if name == openCodeConfigEnvironment {
			return original
		}
		return ""
	}

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "opencode", HarnessSet: true}); err != nil {
		t.Fatal(err)
	}
	session := "fledge-" + sessionSlug(root) + "-00000000"
	instructions, err := os.ReadFile(generatedPromptFile(root))
	if err != nil {
		t.Fatal(err)
	}
	wantInstructions := project.DefaultOrchestratorInstructions + "\n\n" + mandatoryCoordinatorCommunicationPolicy
	instructionsReference := generatedPrompt(t, root, wantInstructions)
	if string(instructions) != wantInstructions {
		t.Fatalf("instruction snapshot = %q, want %q", instructions, wantInstructions)
	}
	if client.splitEnvironment[openCodeConfigEnvironment] != original {
		t.Fatalf("control pane environment = %#v, want original config", client.splitEnvironment)
	}
	merged := client.serverEnvironment[openCodeConfigEnvironment]
	if !strings.Contains(merged, instructionsReference) || !strings.Contains(merged, `"theme":"dark"`) {
		t.Fatalf("server config = %q, want instruction path and original fields", merged)
	}
	assertProtectedFile(t, filepath.Join(fsutil.TempSession(root, session), openCodeEnvironmentFile), original)
	if len(client.startAgent.args) != 0 || len(client.promptCalls) != 0 {
		t.Fatalf("OpenCode launch args = %#v, prompt calls = %#v; want no prompt submission", client.startAgent.args, client.promptCalls)
	}
}

func TestStartOpenCodeRejectsMalformedInlineConfigBeforeLaunch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = func(name string) (string, error) {
		if name == "opencode" {
			return "/test/opencode", nil
		}
		return "", os.ErrNotExist
	}
	manager.getenv = func(name string) string {
		if name == openCodeConfigEnvironment {
			return "{invalid"
		}
		return ""
	}
	session := "fledge-" + sessionSlug(root) + "-00000000"
	if err := os.MkdirAll(fsutil.TempSession(root, session), 0o700); err != nil {
		t.Fatal(err)
	}

	err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "opencode", HarnessSet: true})
	if err == nil || !strings.Contains(err.Error(), "decode "+openCodeConfigEnvironment) {
		t.Fatalf("Start() error = %v, want malformed inline config error", err)
	}
	if slicesContain(client.calls, "start-server") {
		t.Fatalf("calls = %v, want no server launch", client.calls)
	}
	if _, found, readErr := readRecord(root); readErr != nil || found {
		t.Fatalf("record after failed Start() = found %v, error %v; want removed", found, readErr)
	}
	if _, statErr := os.Stat(fsutil.TempSession(root, session)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary session directory after failed preparation = %v, want removed", statErr)
	}
}

func TestStartOpenCodeRollbackRemovesRuntimeArtifacts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{snapshot: testSnapshot(), waitErr: errors.New("not ready")}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = func(name string) (string, error) {
		if name == "opencode" {
			return "/test/opencode", nil
		}
		return "", os.ErrNotExist
	}
	manager.getenv = func(string) string { return "{}" }

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout, Harness: "opencode", HarnessSet: true}); err == nil {
		t.Fatal("Start() error = nil")
	}
	session := "fledge-" + sessionSlug(root) + "-00000000"
	if _, err := os.Stat(fsutil.TempSession(root, session)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rollback temp directory error = %v, want removed", err)
	}
	if _, err := os.Stat(generatedPromptFile(root)); err != nil {
		t.Errorf("generated prompt after rollback = %v, want preserved", err)
	}
}

func TestStartReattachDoesNotRebuildOpenCodeSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	if _, err := prepareOpenCodeRuntime(root, testSessionName, generatedPromptFile(root), `{"old":true}`); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.getenv = func(string) string { return `{"new":true}` }

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(fsutil.TempSession(root, testSessionName), openCodeEnvironmentFile))
	if err != nil || string(contents) != `{"old":true}` {
		t.Fatalf("environment snapshot after reattach = %q, %v", contents, err)
	}
	if slicesContain(client.calls, "start-server") {
		t.Fatalf("calls = %v, want attach without rebuilding", client.calls)
	}
}

func TestSpawnRestoresOriginalOpenCodeConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	const original = ` {"theme":"dark"} `
	if _, err := prepareOpenCodeRuntime(root, testSessionName, generatedPromptFile(root), original); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    testSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.getenv = func(string) string { return "" }

	if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err != nil {
		t.Fatal(err)
	}
	if got := client.createCall.environment[openCodeConfigEnvironment]; got != original {
		t.Fatalf("worker environment = %#v, want exact original config %q", client.createCall.environment, original)
	}
}

func TestOpenCodeRuntimeCleanupFollowsSessionDeletion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		sessions []herdr.Session
	}{
		{name: "successful deletion", sessions: []herdr.Session{{Name: testSessionName, Running: false}}},
		{name: "stale cleanup"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestRecord(t, root)
			generatedPrompt(t, root, "durable prompt")
			if _, err := prepareOpenCodeRuntime(root, testSessionName, generatedPromptFile(root), "{}"); err != nil {
				t.Fatal(err)
			}
			auditPath := filepath.Join(root, stateDirectory, "logs", testSessionName, "events.jsonl")
			if err := os.MkdirAll(filepath.Dir(auditPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(auditPath, []byte("audit\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			manager, _ := newTestManager(&fakeHerdr{sessions: test.sessions}, &fakeConfirmer{answer: true})
			if err := manager.Stop(context.Background(), root); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(fsutil.TempSession(root, testSessionName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("temporary session directory error = %v, want removed", err)
			}
			if contents, err := os.ReadFile(auditPath); err != nil || string(contents) != "audit\n" {
				t.Fatalf("audit after cleanup = %q, %v; want preserved", contents, err)
			}
			if contents, err := os.ReadFile(generatedPromptFile(root)); err != nil || string(contents) != "durable prompt" {
				t.Fatalf("generated prompt after cleanup = %q, %v; want preserved", contents, err)
			}
		})
	}
}

func TestOpenCodeRuntimeRetainedWhenSessionDeletionFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestRecord(t, root)
	if _, err := prepareOpenCodeRuntime(root, testSessionName, generatedPromptFile(root), "{}"); err != nil {
		t.Fatal(err)
	}
	manager, _ := newTestManager(&fakeHerdr{
		sessions:  []herdr.Session{{Name: testSessionName, Running: false}},
		deleteErr: errors.New("delete failed"),
	}, &fakeConfirmer{answer: true})
	if err := manager.Stop(context.Background(), root); err == nil {
		t.Fatal("Stop() error = nil")
	}
	if _, err := os.Stat(filepath.Join(fsutil.TempSession(root, testSessionName), openCodeEnvironmentFile)); err != nil {
		t.Errorf("recoverable environment snapshot removed: %v", err)
	}
}

func TestSpawnCwdMustBelongToOwningProject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		prepare   func(*testing.T, string) (string, string)
		wantError string
	}{
		{
			name: "project root",
			prepare: func(_ *testing.T, root string) (string, string) {
				return root, root
			},
		},
		{
			name: "descendant",
			prepare: func(t *testing.T, root string) (string, string) {
				child := filepath.Join(root, "nested", "work")
				if err := os.MkdirAll(child, 0o755); err != nil {
					t.Fatal(err)
				}
				return filepath.Join("nested", "work"), child
			},
		},
		{
			name: "symlink resolving inside",
			prepare: func(t *testing.T, root string) (string, string) {
				child := filepath.Join(root, "work")
				if err := os.Mkdir(child, 0o755); err != nil {
					t.Fatal(err)
				}
				link := filepath.Join(root, "work-link")
				if err := os.Symlink(child, link); err != nil {
					t.Skipf("create symlink: %v", err)
				}
				return link, child
			},
		},
		{
			name: "external directory",
			prepare: func(t *testing.T, _ string) (string, string) {
				return t.TempDir(), ""
			},
			wantError: "outside the owning Fledge project",
		},
		{
			name: "symlink resolving outside",
			prepare: func(t *testing.T, root string) (string, string) {
				link := filepath.Join(root, "external-link")
				if err := os.Symlink(t.TempDir(), link); err != nil {
					t.Skipf("create symlink: %v", err)
				}
				return link, ""
			},
			wantError: "outside the owning Fledge project",
		},
		{
			name: "nested Fledge project",
			prepare: func(t *testing.T, root string) (string, string) {
				nested := filepath.Join(root, "nested-project")
				if err := os.Mkdir(nested, 0o755); err != nil {
					t.Fatal(err)
				}
				initTestProject(t, nested)
				return nested, ""
			},
			wantError: "belongs to Fledge project",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestRecord(t, root)
			requested, wantCwd := test.prepare(t, root)
			client := &fakeHerdr{
				sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
				snapshot:    testSnapshot(),
				createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1"},
				createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
			}
			manager, _ := newTestManager(client, &fakeConfirmer{})
			manager.lookPath = installedTestHarness

			err := manager.Spawn(context.Background(), root, SpawnOptions{
				Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex", Cwd: requested,
			})
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Spawn() error = %v, want %q", err, test.wantError)
				}
				if slicesContain(client.calls, "create-tab") {
					t.Fatalf("calls = %v, want cwd rejection before tab creation", client.calls)
				}
				return
			}
			if err != nil {
				t.Fatalf("Spawn() error = %v", err)
			}
			if client.createCall.cwd != wantCwd {
				t.Fatalf("CreateTab() cwd = %q, want canonical %q", client.createCall.cwd, wantCwd)
			}
		})
	}
}

func TestSpawnBackfillsCodexRulesAndPreservesConflictsBeforeCreatingTab(t *testing.T) {
	t.Parallel()

	newManager := func(t *testing.T) (*Manager, *fakeHerdr, string, string) {
		t.Helper()
		root := t.TempDir()
		writeTestRecord(t, root)
		path := filepath.Join(root, ".codex", "rules", "fledge.rules")
		client := &fakeHerdr{
			sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
			snapshot:    testSnapshot(),
			createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1"},
			createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
		}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness
		return manager, client, root, path
	}

	t.Run("backfill precedes launch", func(t *testing.T) {
		manager, client, root, path := newManager(t)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		client.startHook = func() {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("rule at StartAgent() = %v, want installed", err)
			}
		}
		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("conflict preserves file and prevents tab creation", func(t *testing.T) {
		manager, client, root, path := newManager(t)
		const contents = "# custom rules\n"
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "conflicts") {
			t.Fatalf("Spawn() error = %v, want Codex rule conflict", err)
		}
		if slicesContain(client.calls, "create-tab") || slicesContain(client.calls, "start-agent") {
			t.Fatalf("calls = %v, want no tab or agent launch", client.calls)
		}
		contentsAfter, readErr := os.ReadFile(path)
		if readErr != nil || string(contentsAfter) != contents {
			t.Fatalf("rules after conflict = %q, %v; want preserved", contentsAfter, readErr)
		}
	})

	t.Run("legacy policy directs user to init before creating tab", func(t *testing.T) {
		manager, client, root, path := newManager(t)
		legacyContents := writeLegacyCodexRules(t, path)
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "run fledge init") {
			t.Fatalf("Spawn() error = %v, want run-fledge-init guidance", err)
		}
		if slicesContain(client.calls, "create-tab") || slicesContain(client.calls, "start-agent") {
			t.Fatalf("calls = %v, want no tab or agent launch", client.calls)
		}
		contentsAfter, readErr := os.ReadFile(path)
		if readErr != nil || string(contentsAfter) != legacyContents {
			t.Fatalf("rules after legacy rejection = %q, %v; want preserved", contentsAfter, readErr)
		}
	})
}

func TestSpawnCallerAwareFocusAndFailures(t *testing.T) {
	t.Parallel()

	newSpawnManager := func(t *testing.T) (*Manager, *fakeHerdr, string) {
		t.Helper()
		root := t.TempDir()
		writeTestRecord(t, root)
		snapshot := testSnapshot()
		kind, name := "codex", "orchestrator"
		liveName := "helper"
		snapshot.Agents = []herdr.Agent{
			{PaneID: "w1:p1", Agent: &kind, Name: &name},
			{PaneID: "w1:p3", Agent: &kind, Name: &liveName},
		}
		client := &fakeHerdr{
			sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
			snapshot:    snapshot,
			createdTab:  herdr.Tab{TabID: "t2"},
			createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2"},
		}
		manager, _ := newTestManager(client, &fakeConfirmer{})
		manager.lookPath = installedTestHarness
		return manager, client, root
	}

	t.Run("agent caller does not steal focus and still receives messaging context", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		manager.getenv = func(name string) string {
			if name == "HERDR_PANE_ID" {
				return "w1:p1"
			}
			return ""
		}
		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err != nil {
			t.Fatal(err)
		}
		if slicesContain(client.calls, "focus-agent") {
			t.Fatalf("calls = %v, want no focus", client.calls)
		}
		if len(client.promptCalls) != 1 || client.prompt != expectedWorkerPrompt(root, "worker", "") {
			t.Fatalf("PromptAgent calls = %#v, want one messaging-context call", client.promptCalls)
		}
	})

	t.Run("startup failure closes tab", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		client.startErr = errors.New("launch failed")
		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err == nil {
			t.Fatal("Spawn() error = nil")
		}
		if len(client.closeCalls) != 1 || client.closeCalls[0] != "t2" {
			t.Fatalf("CloseTab() calls = %v", client.closeCalls)
		}
	})

	t.Run("startup and tab cleanup failures are both returned", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		client.startErr = errors.New("launch failed")
		client.closeErr = errors.New("close failed")
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "launch failed") || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("Spawn() error = %v", err)
		}
	})

	t.Run("canceled spawn uses a live tab cleanup context", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		client.startErr = context.Canceled
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := manager.Spawn(ctx, root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err == nil {
			t.Fatal("Spawn() error = nil")
		}
		if client.closeCtxErr != nil {
			t.Fatalf("cleanup CloseTab() context error = %v", client.closeCtxErr)
		}
	})

	t.Run("prompt failure closes tab", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		client.promptErr = errors.New("prompt failed")
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex", Task: "work"})
		if err == nil || !strings.Contains(err.Error(), "initial prompt failed") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if len(client.closeCalls) != 1 || client.closeCalls[0] != "t2" {
			t.Fatalf("CloseTab() calls = %v, want t2", client.closeCalls)
		}
	})

	t.Run("focus failure still delivers prompt", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		manager.getenv = func(string) string { return "" }
		client.focusErr = errors.New("focus failed")
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex", Task: "work"})
		if err == nil || !strings.Contains(err.Error(), "focusing it failed") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if client.prompt != expectedWorkerPrompt(root, "worker", "work") || len(client.promptCalls) != 1 {
			t.Fatalf("prompt = %q, calls = %v, want prompt delivered", client.prompt, client.calls)
		}
		if len(client.closeCalls) != 0 {
			t.Fatalf("CloseTab() calls = %v, want none", client.closeCalls)
		}
	})

	t.Run("focus prompt and rollback failures are all returned", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		manager.getenv = func(string) string { return "" }
		client.focusErr = errors.New("focus failed")
		client.promptErr = errors.New("prompt failed")
		client.closeErr = errors.New("close failed")
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex", Task: "work"})
		if err == nil || !strings.Contains(err.Error(), "focusing it failed") ||
			!strings.Contains(err.Error(), "initial prompt failed") || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if len(client.closeCalls) != 1 || client.closeCalls[0] != "t2" {
			t.Fatalf("CloseTab() calls = %v, want t2", client.closeCalls)
		}
	})

	t.Run("duplicate name does not create tab", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "helper", Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if slicesContain(client.calls, "create-tab") {
			t.Fatalf("calls = %v, want no tab creation", client.calls)
		}
	})

	// "orchestrator" bypasses delegation and transition authorization, so it must
	// stay unclaimable even once fledge agent stop has freed the live name.
	t.Run("reserved orchestrator name does not create tab", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		client.snapshot.Agents = nil
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: orchestratorIdentity, Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if slicesContain(client.calls, "create-tab") {
			t.Fatalf("calls = %v, want no tab creation", client.calls)
		}
	})

	t.Run("reserved user name does not create tab", func(t *testing.T) {
		manager, client, root := newSpawnManager(t)
		err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: userIdentity, Harness: "codex"})
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("Spawn() error = %v", err)
		}
		if slicesContain(client.calls, "create-tab") {
			t.Fatalf("calls = %v, want no tab creation", client.calls)
		}
	})

}

func expectedWorkerPrompt(root, agent, task string) string {
	return agentMessagingContext
}

func TestStopAgentClosesNamedAgentPane(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	name := "worker"
	snapshot := testSnapshot()
	snapshot.Agents = []herdr.Agent{{Name: &name, PaneID: "w1:p2"}}
	client := &fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot: snapshot,
	}
	manager, output := newTestManager(client, &fakeConfirmer{})
	registerTestAgent(t, root, name, "w1:p2")

	if err := manager.StopAgent(context.Background(), root, name); err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{"check", "list", "close-pane"}
	if strings.Join(client.calls, ",") != strings.Join(wantCalls, ",") {
		t.Fatalf("call order = %v, want %v", client.calls, wantCalls)
	}
	if len(client.closePaneCalls) != 1 || client.closePaneCalls[0] != "w1:p2" {
		t.Fatalf("ClosePane() calls = %v", client.closePaneCalls)
	}
	if !strings.Contains(output.String(), "Stopped agent worker and closed pane w1:p2") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestStopAgentRejectsInvalidOrProtectedNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"Not Valid", "orchestrator"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTestRecord(t, root)
			client := &fakeHerdr{}
			manager, _ := newTestManager(client, &fakeConfirmer{})

			if err := manager.StopAgent(context.Background(), root, name); err == nil {
				t.Fatal("StopAgent() error = nil")
			}
			if len(client.calls) != 0 {
				t.Fatalf("Herdr calls = %v", client.calls)
			}
		})
	}
}

func TestStopAgentRequiresRunningSessionAndLiveAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		withRecord bool
		sessions   []herdr.Session
		snapshot   herdr.Snapshot
		wantError  string
	}{
		{name: "no record", wantError: "no Fledge session"},
		{name: "missing session", withRecord: true, wantError: "not running"},
		{name: "stopped session", withRecord: true, sessions: []herdr.Session{{Name: testSessionName}}, wantError: "not running"},
		{name: "unknown agent", withRecord: true, sessions: []herdr.Session{{Name: testSessionName, Running: true}}, snapshot: testSnapshot(), wantError: "not found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.withRecord {
				writeTestRecord(t, root)
			} else {
				initTestProject(t, root)
			}
			client := &fakeHerdr{sessions: test.sessions, snapshot: test.snapshot}
			manager, _ := newTestManager(client, &fakeConfirmer{})

			err := manager.StopAgent(context.Background(), root, "worker")
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("StopAgent() error = %v, want %q", err, test.wantError)
			}
			if len(client.closePaneCalls) != 0 {
				t.Fatalf("ClosePane() calls = %v", client.closePaneCalls)
			}
		})
	}
}

func TestStopAgentReturnsCloseFailures(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		closeErr error
	}{
		{name: "close", closeErr: errors.New("close failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestRecord(t, root)
			name := "worker"
			snapshot := testSnapshot()
			snapshot.Agents = []herdr.Agent{{Name: &name, PaneID: "w1:p2"}}
			client := &fakeHerdr{
				sessions: []herdr.Session{{Name: testSessionName, Running: true}}, snapshot: snapshot,
				closePaneErr: test.closeErr,
			}
			manager, output := newTestManager(client, &fakeConfirmer{})
			registerTestAgent(t, root, name, "w1:p2")

			err := manager.StopAgent(context.Background(), root, name)
			wantErr := test.closeErr
			if !errors.Is(err, wantErr) {
				t.Fatalf("StopAgent() error = %v, want %v", err, wantErr)
			}
			if output.Len() != 0 {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func registerTestAgent(t *testing.T, root, name, pane string) {
	t.Helper()
	store := messaging.New(root, testSessionName)
	sessionID, err := store.Initialize()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRecordSessionBinding(root, testSessionName, sessionID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.RegisterAgent(messaging.RegisterParams{Name: name, PaneID: pane, Harness: "codex", Caller: "user"}); err != nil {
		t.Fatal(err)
	}
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCallerInputTreatsNamedDetectionUncertaintyAsAgentOccupancy(t *testing.T) {
	t.Parallel()
	name := "worker"
	snapshot := testSnapshot()
	snapshot.Agents = []herdr.Agent{{PaneID: "w1:p1", Name: &name}}
	input := callerInput("w1:p1", snapshot, true)
	if got := tui.ClassifyCaller(input); got != tui.CallerAgent {
		t.Fatalf("ClassifyCaller() = %v, want agent", got)
	}
}

func TestSessionSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "lowercase and collapse separators", base: "My  Project__Name", want: "my-project-name"},
		{name: "non-ASCII is a separator", base: "Crème東京App", want: "cr-me-app"},
		{name: "trim separators", base: "---Project---", want: "project"},
		{name: "empty fallback", base: "東京", want: "cwd"},
		{
			name: "truncate and trim trailing separator",
			base: strings.Repeat("a", 63) + " project",
			want: strings.Repeat("a", 63),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := sessionSlug(filepath.Join("root", test.base)); got != test.want {
				t.Errorf("sessionSlug() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadRecordSessionNameFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sessionName string
		wantValid   bool
	}{
		{name: "new", sessionName: "fledge-my-project-0123abcd", wantValid: true},
		{name: "new maximum slug", sessionName: "fledge-" + strings.Repeat("a", 64) + "-0123abcd", wantValid: true},
		{name: "legacy", sessionName: testSessionName, wantValid: true},
		{name: "missing slug", sessionName: "fledge-0123abcd", wantValid: false},
		{name: "uppercase slug", sessionName: "fledge-My-project-0123abcd", wantValid: false},
		{name: "consecutive separators", sessionName: "fledge-my--project-0123abcd", wantValid: false},
		{name: "slug too long", sessionName: "fledge-" + strings.Repeat("a", 65) + "-0123abcd", wantValid: false},
		// A grammatically valid hyphenated slug of 65 bytes must still be rejected
		// by the retained length cap, so delegating grammar to statedir (which has
		// no length bound) cannot silently broaden accepted records.
		{name: "hyphenated slug too long", sessionName: "fledge-a" + strings.Repeat("-a", 32) + "-0123abcd", wantValid: false},
		{name: "short identifier", sessionName: "fledge-my-project-0123abc", wantValid: false},
		{name: "uppercase identifier", sessionName: "fledge-my-project-0123abcD", wantValid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if err := initializeStateDirectory(root); err != nil {
				t.Fatal(err)
			}
			created, err := createRecord(root, record{Version: recordVersion, SessionName: test.sessionName})
			if err != nil {
				t.Fatal(err)
			}
			if !created {
				t.Fatal("record already existed")
			}

			value, found, err := readRecord(root)
			if test.wantValid {
				if err != nil || !found || value.SessionName != test.sessionName {
					t.Fatalf("readRecord() = %#v, %v, %v; want valid record", value, found, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("readRecord() = %#v, %v, nil; want invalid session name error", value, found)
			}
		})
	}
}

func TestStopSessionStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sessions    []herdr.Session
		wantStops   int
		wantDeletes int
		wantOutput  string
	}{
		{
			name:        "running",
			sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
			wantStops:   1,
			wantDeletes: 1,
			wantOutput:  "Stopped and deleted",
		},
		{
			name:        "stopped",
			sessions:    []herdr.Session{{Name: testSessionName, Running: false}},
			wantDeletes: 1,
			wantOutput:  "Stopped and deleted",
		},
		{
			name:       "missing",
			wantOutput: "Removed stale",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			child := filepath.Join(root, "one", "two")
			if err := os.MkdirAll(child, 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestRecord(t, root)
			store := messaging.New(root, testSessionName)
			if _, err := store.Initialize(); err != nil {
				t.Fatal(err)
			}
			sessionDirectory := filepath.Join(root, stateDirectory, "logs", testSessionName)
			logPath := filepath.Join(root, stateDirectory, "logs", "session.log")
			if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(logPath, []byte("keep me"), 0o600); err != nil {
				t.Fatal(err)
			}

			client := &fakeHerdr{sessions: test.sessions}
			confirmer := &fakeConfirmer{answer: true}
			manager, output := newTestManager(client, confirmer)

			if err := manager.Stop(context.Background(), child); err != nil {
				t.Fatalf("Stop() error = %v", err)
			}
			if len(client.stopCalls) != test.wantStops {
				t.Errorf("Stop() calls = %d, want %d", len(client.stopCalls), test.wantStops)
			}
			if len(client.deleteCalls) != test.wantDeletes {
				t.Errorf("Delete() calls = %d, want %d", len(client.deleteCalls), test.wantDeletes)
			}
			if _, err := os.Stat(recordPath(root)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("session record error = %v, want not exist", err)
			}
			if _, err := os.Stat(filepath.Join(sessionDirectory, "events.jsonl")); err != nil {
				t.Errorf("messaging log after Stop: %v; want preserved", err)
			}
			if _, err := os.Stat(fsutil.TempSession(root, testSessionName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("temporary session directory error = %v, want not exist", err)
			}
			if contents, err := os.ReadFile(logPath); err != nil || string(contents) != "keep me" {
				t.Errorf("log contents = %q, %v; want preserved", contents, err)
			}
			if !strings.Contains(output.String(), test.wantOutput) {
				t.Errorf("output = %q, want substring %q", output, test.wantOutput)
			}
			if len(confirmer.questions) != 1 || !strings.Contains(confirmer.questions[0], root) {
				t.Errorf("confirmation questions = %#v, want project root", confirmer.questions)
			}
		})
	}
}

func TestStopCancellationLeavesEverythingUntouched(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, output := newTestManager(client, &fakeConfirmer{answer: false})

	if err := manager.Stop(context.Background(), root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if len(client.stopCalls) != 0 || len(client.deleteCalls) != 0 {
		t.Fatalf("destructive calls = %v, %v; want none", client.stopCalls, client.deleteCalls)
	}
	if _, err := os.Stat(recordPath(root)); err != nil {
		t.Errorf("session record removed: %v", err)
	}
	if output.String() != "Canceled.\n" {
		t.Errorf("output = %q, want canceled", output.String())
	}
}

func TestStopRemovesRecordBeforeStoppingRunningSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: true}},
		stopHook: func() {
			if _, err := os.Stat(recordPath(root)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("session record error during Stop() = %v, want not exist", err)
			}
		},
	}
	manager, _ := newTestManager(client, &fakeConfirmer{answer: true})

	if err := manager.Stop(context.Background(), root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestStopAppendsRuntimeIgnoreForUpgradedProjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestRecord(t, root)
	// Simulate a project initialized by an older Fledge whose ignore file predates
	// session.lock. The first command after upgrade may be Stop, so it must add the
	// persistent lock entry rather than leave it visible as an untracked file.
	ignorePath := filepath.Join(root, stateDirectory, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("session.json\nkeep-local/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{sessions: []herdr.Session{{Name: testSessionName, Running: true}}}
	manager, _ := newTestManager(client, &fakeConfirmer{answer: true})

	if err := manager.Stop(context.Background(), root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	ignore, err := os.ReadFile(ignorePath)
	if err != nil || string(ignore) != "session.json\nkeep-local/\nsession.lock\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n" {
		t.Fatalf("Stop() .gitignore = %q, %v", ignore, err)
	}
}

func TestStopFailureRetainsRecord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sessionRunning bool
		stopErr        error
		deleteErr      error
	}{
		{name: "running session stop fails", sessionRunning: true, stopErr: errors.New("stop failed")},
		{name: "running session delete fails", sessionRunning: true, deleteErr: errors.New("delete failed")},
		{name: "stopped session delete fails", deleteErr: errors.New("delete failed")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestRecord(t, root)
			if _, err := messaging.New(root, testSessionName).Initialize(); err != nil {
				t.Fatal(err)
			}
			client := &fakeHerdr{
				sessions:  []herdr.Session{{Name: testSessionName, Running: test.sessionRunning}},
				stopErr:   test.stopErr,
				deleteErr: test.deleteErr,
			}
			manager, _ := newTestManager(client, &fakeConfirmer{answer: true})

			if err := manager.Stop(context.Background(), root); err == nil {
				t.Fatal("Stop() error = nil, want error")
			}
			if _, err := os.Stat(recordPath(root)); err != nil {
				t.Errorf("session record removed: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, stateDirectory, "logs", testSessionName, "events.jsonl")); err != nil {
				t.Errorf("messaging log removed after failed cleanup: %v", err)
			}
		})
	}
}

func TestStopHerdrLookupFailureLeavesRecordUntouched(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		checkErr      error
		listErr       error
		wantListCalls int
	}{
		{name: "check fails", checkErr: errors.New("check failed")},
		{name: "list fails", listErr: errors.New("list failed"), wantListCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTestRecord(t, root)
			client := &fakeHerdr{checkErr: test.checkErr, listErr: test.listErr}
			confirmer := &fakeConfirmer{answer: true}
			manager, _ := newTestManager(client, confirmer)

			wantErr := test.checkErr
			if wantErr == nil {
				wantErr = test.listErr
			}
			if err := manager.Stop(context.Background(), root); !errors.Is(err, wantErr) {
				t.Fatalf("Stop() error = %v, want %v", err, wantErr)
			}
			if client.checkCalls != 1 {
				t.Errorf("Check() calls = %d, want 1", client.checkCalls)
			}
			if client.listCalls != test.wantListCalls {
				t.Errorf("List() calls = %d, want %d", client.listCalls, test.wantListCalls)
			}
			if len(confirmer.questions) != 0 {
				t.Errorf("confirmation questions = %#v, want none", confirmer.questions)
			}
			if len(client.stopCalls) != 0 || len(client.deleteCalls) != 0 {
				t.Errorf("destructive calls = %v, %v; want none", client.stopCalls, client.deleteCalls)
			}
			value, found, err := readRecord(root)
			if err != nil || !found || value.SessionName != testSessionName {
				t.Errorf("readRecord() = %#v, %v, %v; want unchanged record", value, found, err)
			}
		})
	}
}

func TestStopWithoutRecordIsNoOp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	client := &fakeHerdr{checkErr: errors.New("should not be checked")}
	manager, output := newTestManager(client, &fakeConfirmer{})

	if err := manager.Stop(context.Background(), root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if client.checkCalls != 0 {
		t.Errorf("Check() calls = %d, want 0", client.checkCalls)
	}
	if !strings.Contains(output.String(), "No active Fledge session") {
		t.Errorf("output = %q", output.String())
	}
}

func TestMalformedRecordFailsWithoutCleanup(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	stateDir := filepath.Join(root, stateDirectory)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := recordPath(root)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, _ := newTestManager(&fakeHerdr{}, &fakeConfirmer{answer: true})

	if err := manager.Stop(context.Background(), root); err == nil {
		t.Fatal("Stop() error = nil, want error")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("malformed record removed: %v", err)
	}
}

func TestSpawnOffersLastUsedOnlyWhenInstalled(t *testing.T) {
	t.Parallel()

	t.Run("installed harness offered", func(t *testing.T) {
		t.Parallel()
		resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "worker", Harness: "codex"}}
		manager, _, _, root := newPreferencesSpawnManager(t, resolver)
		if err := writePreferences(root, preferences{Version: preferencesVersion, Harness: "codex", Model: "gpt-custom"}); err != nil {
			t.Fatal(err)
		}

		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout}); err != nil {
			t.Fatalf("Spawn() error = %v", err)
		}
		want := tui.LastUsed{Harness: "codex", Model: "gpt-custom"}
		if resolver.request.LastUsed == nil || *resolver.request.LastUsed != want {
			t.Errorf("request.LastUsed = %#v, want %#v", resolver.request.LastUsed, want)
		}
		if len(resolver.request.Harnesses) != 1 || resolver.request.Harnesses[0].Value != "codex" {
			t.Errorf("request.Harnesses = %#v", resolver.request.Harnesses)
		}
	})

	t.Run("uninstalled remembered harness suppresses shortcut", func(t *testing.T) {
		t.Parallel()
		resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "worker", Harness: "codex"}}
		manager, _, _, root := newPreferencesSpawnManager(t, resolver)
		if err := writePreferences(root, preferences{Version: preferencesVersion, Harness: "claude", Model: "sonnet"}); err != nil {
			t.Fatal(err)
		}

		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout}); err != nil {
			t.Fatalf("Spawn() error = %v", err)
		}
		if resolver.request.LastUsed != nil {
			t.Errorf("request.LastUsed = %#v, want nil", resolver.request.LastUsed)
		}
	})

	t.Run("harness flag suppresses shortcut", func(t *testing.T) {
		t.Parallel()
		resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "worker", Harness: "codex"}}
		manager, _, _, root := newPreferencesSpawnManager(t, resolver)
		if err := writePreferences(root, preferences{Version: preferencesVersion, Harness: "codex", Model: "gpt-custom"}); err != nil {
			t.Fatal(err)
		}

		if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout, Name: "worker", Harness: "codex"}); err != nil {
			t.Fatalf("Spawn() error = %v", err)
		}
		if resolver.request.LastUsed != nil {
			t.Errorf("request.LastUsed = %#v, want nil", resolver.request.LastUsed)
		}
	})
}

func TestSpawnSavesPromptedSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selection tui.Selection
		startErr  error
		wantErr   bool
		wantSaved bool
		want      preferences
	}{
		{
			name:      "prompted selection saved",
			selection: tui.Selection{Name: "worker", Harness: "codex", Model: "gpt-custom", Prompted: true},
			wantSaved: true,
			want:      preferences{Version: preferencesVersion, Harness: "codex", Model: "gpt-custom"},
		},
		{
			name:      "remembered harness default round-trips",
			selection: tui.Selection{Name: "worker", Harness: "codex", Prompted: true},
			wantSaved: true,
			want:      preferences{Version: preferencesVersion, Harness: "codex"},
		},
		{
			name:      "unprompted selection not saved",
			selection: tui.Selection{Name: "worker", Harness: "codex", Model: "gpt-custom"},
		},
		{
			name:      "saved even when agent start fails",
			selection: tui.Selection{Name: "worker", Harness: "codex", Model: "gpt-custom", Prompted: true},
			startErr:  errors.New("start failed"),
			wantErr:   true,
			wantSaved: true,
			want:      preferences{Version: preferencesVersion, Harness: "codex", Model: "gpt-custom"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			resolver := &fakeSelectionResolver{selection: test.selection}
			manager, client, _, root := newPreferencesSpawnManager(t, resolver)
			client.startErr = test.startErr

			err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout})
			if (err != nil) != test.wantErr {
				t.Fatalf("Spawn() error = %v, wantErr %v", err, test.wantErr)
			}
			value, found, readErr := readPreferences(root)
			if readErr != nil {
				t.Fatalf("readPreferences() error = %v", readErr)
			}
			if found != test.wantSaved {
				t.Fatalf("preferences found = %v, want %v", found, test.wantSaved)
			}
			if found && value != test.want {
				t.Errorf("preferences = %#v, want %#v", value, test.want)
			}
		})
	}
}

func TestSpawnIgnoresCorruptPreferences(t *testing.T) {
	t.Parallel()

	resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "worker", Harness: "codex"}}
	manager, _, output, root := newPreferencesSpawnManager(t, resolver)
	if err := os.WriteFile(preferencesPath(root), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}
	if !strings.Contains(output.String(), "Warning: ignoring saved picker preferences") {
		t.Errorf("output = %q, want corrupt-preferences warning", output.String())
	}
	if resolver.request.LastUsed != nil {
		t.Errorf("request.LastUsed = %#v, want nil", resolver.request.LastUsed)
	}
}

func TestSpawnPropagatesCorruptPreferencesWarningWriteFailure(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("output write failed")
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    testSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "worker", Harness: "codex"}}
	manager := NewManager(client, &fakeConfirmer{}, nil, errorWriter{err: writeErr})
	manager.random = bytes.NewReader(make([]byte, 16))
	manager.getenv = func(string) string { return "" }
	manager.lookPath = installedTestHarness
	manager.selector = resolver
	if err := os.WriteFile(preferencesPath(root), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := manager.Spawn(context.Background(), root, SpawnOptions{Timeout: DefaultAgentTimeout}); !errors.Is(err, writeErr) {
		t.Fatalf("Spawn() error = %v, want %v", err, writeErr)
	}
}

func TestStartOffersAndSavesLastUsedSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initTestProject(t, root)
	if err := initializeStateDirectory(root); err != nil {
		t.Fatal(err)
	}
	if err := writePreferences(root, preferences{Version: preferencesVersion, Harness: "codex", Model: "old-model"}); err != nil {
		t.Fatal(err)
	}
	client := &fakeHerdr{snapshot: testSnapshot()}
	manager, _ := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	resolver := &fakeSelectionResolver{selection: tui.Selection{Name: "orchestrator", Harness: "codex", Model: "new-model", Prompted: true}}
	manager.selector = resolver

	if err := manager.Start(context.Background(), root, StartOptions{Timeout: DefaultAgentTimeout}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	want := tui.LastUsed{Harness: "codex", Model: "old-model"}
	if resolver.request.LastUsed == nil || *resolver.request.LastUsed != want {
		t.Errorf("request.LastUsed = %#v, want %#v", resolver.request.LastUsed, want)
	}
	value, found, err := readPreferences(root)
	if err != nil || !found {
		t.Fatalf("readPreferences() = %v, %v", found, err)
	}
	if want := (preferences{Version: preferencesVersion, Harness: "codex", Model: "new-model"}); value != want {
		t.Errorf("preferences = %#v, want %#v", value, want)
	}
}

type fakeSelectionResolver struct {
	request   tui.SelectionRequest
	selection tui.Selection
	err       error
}

func (f *fakeSelectionResolver) Select(_ context.Context, request tui.SelectionRequest) (tui.Selection, error) {
	f.request = request
	return f.selection, f.err
}

func newPreferencesSpawnManager(t *testing.T, resolver *fakeSelectionResolver) (*Manager, *fakeHerdr, *bytes.Buffer, string) {
	t.Helper()
	root := t.TempDir()
	writeTestRecord(t, root)
	client := &fakeHerdr{
		sessions:    []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot:    testSnapshot(),
		createdTab:  herdr.Tab{TabID: "t2", WorkspaceID: "w1"},
		createdPane: herdr.Pane{PaneID: "w1:p2", TabID: "t2", WorkspaceID: "w1"},
	}
	manager, output := newTestManager(client, &fakeConfirmer{})
	manager.lookPath = installedTestHarness
	manager.selector = resolver
	return manager, client, output, root
}

func newTestManager(client *fakeHerdr, confirmer *fakeConfirmer) (*Manager, *bytes.Buffer) {
	var output bytes.Buffer
	manager := NewManager(client, confirmer, nil, &output)
	manager.random = bytes.NewReader(make([]byte, 16))
	manager.getenv = func(string) string { return "" }
	manager.watchLauncher = func(string) error { return nil }
	manager.watchRunner = func(context.Context, watchproc.Options) error { return nil }
	manager.watchStopper = func(string, string) error { return nil }
	return manager, &output
}

func writeTestRecord(t *testing.T, root string) {
	t.Helper()
	initTestProject(t, root)
	if err := initializeStateDirectory(root); err != nil {
		t.Fatal(err)
	}
	sessionID, err := messaging.New(root, testSessionName).Initialize()
	if err != nil {
		t.Fatal(err)
	}
	created, err := createRecord(root, record{
		Version: recordVersion, SessionName: testSessionName, MessagingSessionID: sessionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("record already existed")
	}
}

// generatedPromptFile is the absolute path of a project's generated
// orchestrator prompt, which is also the reference Fledge hands to harnesses.
func generatedPromptFile(root string) string {
	return filepath.Join(root, stateDirectory, "profiles", "generated", "orchestrator.md")
}

// generatedPrompt renders the prompt Start would generate and returns the
// reference the harness receives for it. project.EnsureGeneratedOrchestratorPrompt
// owns that reference's form, so tests follow it instead of hardcoding one.
func generatedPrompt(t *testing.T, root, instructions string) string {
	t.Helper()
	reference, err := project.EnsureGeneratedOrchestratorPrompt(root, instructions)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func initTestProject(t *testing.T, root string) {
	t.Helper()
	if _, err := project.Init(root); err != nil {
		t.Fatal(err)
	}
}

type attachCall struct {
	name string
	dir  string
}

type fakeHerdr struct {
	protocol          int
	checkErr          error
	attachErr         error
	listErr           error
	stopErr           error
	deleteErr         error
	sessions          []herdr.Session
	listSequence      [][]herdr.Session
	snapshot          herdr.Snapshot
	snapshotErr       error
	checkCalls        int
	listCalls         int
	attachCalls       []attachCall
	stopCalls         []string
	deleteCalls       []string
	stopHook          func()
	calls             []string
	startAgent        startAgentCall
	prompt            string
	promptCalls       []promptCall
	createdTab        herdr.Tab
	createdPane       herdr.Pane
	createdWorkspace  herdr.Workspace
	createWorkspace   createWorkspaceCall
	createCall        createTabCall
	startErr          error
	startHook         func()
	promptErr         error
	promptHook        func()
	renameErr         error
	focusErr          error
	closeCalls        []string
	closeErr          error
	closePaneCalls    []string
	closePaneErr      error
	serverErr         error
	waitErr           error
	closeCtxErr       error
	stopCtxErr        error
	serverEnvironment map[string]string
	splitEnvironment  map[string]string
}

type startAgentCall struct {
	session string
	name    string
	kind    string
	pane    string
	timeout time.Duration
	args    []string
}

type createTabCall struct {
	session, workspace, cwd, label string
	environment                    map[string]string
}

type createWorkspaceCall struct {
	session, dir, label string
}

type promptCall struct {
	session, recipient, prompt string
}

func (f *fakeHerdr) Check() error {
	f.calls = append(f.calls, "check")
	f.checkCalls++
	return f.checkErr
}

func (f *fakeHerdr) Attach(_ context.Context, name, dir string) error {
	f.calls = append(f.calls, "attach")
	f.attachCalls = append(f.attachCalls, attachCall{name: name, dir: dir})
	return f.attachErr
}

func (f *fakeHerdr) StartServer(_ string, _ string, environment map[string]string) error {
	f.calls = append(f.calls, "start-server")
	f.serverEnvironment = cloneEnvironment(environment)
	return f.serverErr
}
func (f *fakeHerdr) WaitReady(context.Context, string, time.Duration) (herdr.Snapshot, error) {
	f.calls = append(f.calls, "wait-ready")
	return f.snapshot, f.waitErr
}
func (f *fakeHerdr) Snapshot(context.Context, string) (herdr.Snapshot, error) {
	f.calls = append(f.calls, "snapshot")
	return f.snapshot, f.snapshotErr
}
func (f *fakeHerdr) CreateWorkspace(_ context.Context, session, dir, label string) (herdr.Workspace, herdr.Tab, herdr.Pane, error) {
	f.calls = append(f.calls, "create-workspace")
	f.createWorkspace = createWorkspaceCall{session: session, dir: dir, label: label}
	return f.createdWorkspace, f.createdTab, f.createdPane, nil
}
func (f *fakeHerdr) RenameTab(context.Context, string, string, string) error {
	f.calls = append(f.calls, "rename-tab")
	return f.renameErr
}
func (f *fakeHerdr) RenamePane(context.Context, string, string, string) error {
	f.calls = append(f.calls, "rename-pane")
	return f.renameErr
}
func (f *fakeHerdr) SplitPane(_ context.Context, _, _, _ string, environment map[string]string) (herdr.Pane, error) {
	f.calls = append(f.calls, "split-pane")
	f.splitEnvironment = cloneEnvironment(environment)
	return herdr.Pane{}, nil
}
func (f *fakeHerdr) CreateTab(_ context.Context, session, workspace, cwd, label string, environment map[string]string) (herdr.Tab, herdr.Pane, error) {
	f.calls = append(f.calls, "create-tab")
	f.createCall = createTabCall{session: session, workspace: workspace, cwd: cwd, label: label, environment: cloneEnvironment(environment)}
	return f.createdTab, f.createdPane, nil
}

func cloneEnvironment(environment map[string]string) map[string]string {
	if environment == nil {
		return nil
	}
	cloned := make(map[string]string, len(environment))
	for key, value := range environment {
		cloned[key] = value
	}
	return cloned
}
func (f *fakeHerdr) CloseTab(ctx context.Context, _ string, tabID string) error {
	f.calls = append(f.calls, "close-tab")
	f.closeCalls = append(f.closeCalls, tabID)
	f.closeCtxErr = ctx.Err()
	return f.closeErr
}
func (f *fakeHerdr) ClosePane(_ context.Context, _ string, paneID string) error {
	f.calls = append(f.calls, "close-pane")
	f.closePaneCalls = append(f.closePaneCalls, paneID)
	return f.closePaneErr
}
func (f *fakeHerdr) FocusAgent(context.Context, string, string) error {
	f.calls = append(f.calls, "focus-agent")
	return f.focusErr
}
func (f *fakeHerdr) StartAgent(_ context.Context, session, name, kind, pane string, timeout time.Duration, args []string) error {
	f.calls = append(f.calls, "start-agent")
	f.startAgent = startAgentCall{session: session, name: name, kind: kind, pane: pane, timeout: timeout, args: append([]string(nil), args...)}
	if f.startHook != nil {
		f.startHook()
	}
	return f.startErr
}
func (f *fakeHerdr) PromptAgent(_ context.Context, session string, recipient string, prompt string) error {
	f.calls = append(f.calls, "prompt-agent")
	f.prompt = prompt
	f.promptCalls = append(f.promptCalls, promptCall{session: session, recipient: recipient, prompt: prompt})
	if f.promptHook != nil {
		f.promptHook()
	}
	return f.promptErr
}

func (f *fakeHerdr) List(context.Context) ([]herdr.Session, error) {
	f.calls = append(f.calls, "list")
	f.listCalls++
	if len(f.listSequence) > 0 {
		index := f.listCalls - 1
		if index >= len(f.listSequence) {
			index = len(f.listSequence) - 1
		}
		return f.listSequence[index], f.listErr
	}
	return f.sessions, f.listErr
}

func (f *fakeHerdr) Protocol(context.Context) (int, error) {
	if f.protocol != 0 {
		return f.protocol, nil
	}
	return watchproc.RequiredHerdrProtocol, nil
}

func (f *fakeHerdr) Stop(ctx context.Context, name string) error {
	f.calls = append(f.calls, "stop")
	f.stopCalls = append(f.stopCalls, name)
	f.stopCtxErr = ctx.Err()
	if f.stopHook != nil {
		f.stopHook()
	}
	return f.stopErr
}

func (f *fakeHerdr) Delete(_ context.Context, name string) error {
	f.calls = append(f.calls, "delete")
	f.deleteCalls = append(f.deleteCalls, name)
	return f.deleteErr
}

type fakeConfirmer struct {
	answer    bool
	err       error
	questions []string
}

func (f *fakeConfirmer) Confirm(question string) (bool, error) {
	f.questions = append(f.questions, question)
	return f.answer, f.err
}

type errorReader struct {
	err error
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func installedTestHarness(name string) (string, error) {
	if name == "codex" {
		return "/test/codex", nil
	}
	return "", os.ErrNotExist
}

func writeLegacyCodexRules(t *testing.T, path string) string {
	t.Helper()
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	policy := string(current)
	marker := `
prefix_rule(
    pattern = ["herdr", "agent",`
	index := strings.Index(policy, marker)
	if index < 0 {
		t.Fatal("generated Codex policy has no Herdr communication rule")
	}
	legacy := policy[:index] + `
prefix_rule(
    pattern = ["herdr", "agent", ["wait", "read"]],
    decision = "allow",
    justification = "Read-only Herdr agent coordination commands are allowed outside the sandbox.",
    match = [
        "herdr agent wait worker",
        "herdr agent read worker",
    ],
    not_match = [
        "herdr agent start worker",
        "herdr session delete project",
    ],
)
`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	return legacy
}

func testSnapshot() herdr.Snapshot {
	return herdr.Snapshot{
		FocusedWorkspaceID: "w1",
		FocusedTabID:       "t1",
		FocusedPaneID:      "w1:p1",
		Workspaces:         []herdr.Workspace{{WorkspaceID: "w1"}},
		Tabs:               []herdr.Tab{{TabID: "t1", WorkspaceID: "w1"}},
		Panes:              []herdr.Pane{{PaneID: "w1:p1", TabID: "t1", WorkspaceID: "w1"}},
	}
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestStartAndSpawnRejectZeroTimeoutIdentically(t *testing.T) {
	t.Parallel()

	const want = "agent startup timeout must be greater than 3s and no more than 5m"

	startRoot := t.TempDir()
	initTestProject(t, startRoot)
	startManager, _ := newTestManager(&fakeHerdr{snapshot: testSnapshot()}, &fakeConfirmer{})
	startManager.lookPath = installedTestHarness
	err := startManager.Start(context.Background(), startRoot, StartOptions{Harness: "codex", HarnessSet: true})
	if err == nil || err.Error() != want {
		t.Errorf("Start() error = %v, want %q", err, want)
	}

	spawnRoot := t.TempDir()
	writeTestRecord(t, spawnRoot)
	spawnManager, _ := newTestManager(&fakeHerdr{
		sessions: []herdr.Session{{Name: testSessionName, Running: true}},
		snapshot: testSnapshot(),
	}, &fakeConfirmer{})
	spawnManager.lookPath = installedTestHarness
	err = spawnManager.Spawn(context.Background(), spawnRoot, SpawnOptions{Name: "worker", Harness: "codex"})
	if err == nil || err.Error() != want {
		t.Errorf("Spawn() error = %v, want %q", err, want)
	}
}

func TestValidateAgentTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{name: "3s rejected", timeout: 3 * time.Second, wantErr: true},
		{name: "just above 3s accepted", timeout: 3*time.Second + time.Millisecond, wantErr: false},
		{name: "5m accepted", timeout: 5 * time.Minute, wantErr: false},
		{name: "just above 5m rejected", timeout: 5*time.Minute + time.Millisecond, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateAgentTimeout(test.timeout)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateAgentTimeout(%v) = %v, wantErr %v", test.timeout, err, test.wantErr)
			}
		})
	}
}
