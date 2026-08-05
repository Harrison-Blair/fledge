// Package lifecycle manages Fledge's project-local session lifecycle.
package lifecycle

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agentcontext"
	"github.com/Harrison-Blair/fledge/internal/harness"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/logging"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/statedir"
	"github.com/Harrison-Blair/fledge/internal/tui"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

const (
	stateDirectory = ".fledge"
	recordFilename = "session.json"
	recordVersion  = 1
	ignoreContents = "session.json\npreferences.json\nlogs/\ntmp/\nprofiles/generated/\n"

	sessionNamePrefix    = "fledge-"
	sessionIDBytes       = 4
	sessionIDHexLength   = sessionIDBytes * 2
	maxSessionSlugLength = 64

	codexCoordinatorGuidance = "Codex coordination commands must run outside the sandbox. " +
		"Immediately request outside-sandbox execution for fledge commands other than fledge start or fledge stop. " +
		"Never run fledge start or fledge stop; " +
		"the user must run those lifecycle commands directly."

	mandatoryCoordinatorCommunicationPolicy = `Mandatory Fledge communication policy (overrides conflicting profile instructions):
- List installed harness model catalogs with fledge agent models [harness].
- Choose a worker model with fledge agent spawn --model <exact-model-value>; catalog entries are advisory, so custom model values remain valid.
- Initial worker tasks may be supplied with fledge agent spawn --prompt.
- After spawning, communicate with workers only through:
  fledge agent message send <recipient> <text>
  fledge agent message reply <message-id> <text>
- Treat an injected Fledge completion message as the worker's completion signal, then stop the completed worker with:
  fledge agent stop <name>
- Messages from sender watcher are actionable automated notifications. Act on them; never reply to watcher messages.
- Never poll with fledge agent message inbox. Wait for injected Fledge messages instead.
- Never use direct Herdr commands to communicate with, inspect, prompt, or collect output from agents. This includes herdr agent wait, read, get, list, prompt, send-keys, attach, and explain, plus Herdr API snapshots.`

	agentMessagingContext = `You are a worker in a Fledge-managed session.

Communicate with the orchestrator only through Fledge messaging:
- Send progress updates and a completion summary to the orchestrator with:
  fledge agent message send orchestrator <text>
- Reply to each incoming message with its message ID to preserve correlation:
  fledge agent message reply <message-id> <text>
- Never poll with fledge agent message inbox. Wait for injected Fledge messages instead.
- Never use direct Herdr commands to communicate with, inspect, prompt, or collect output from agents. This includes herdr agent wait, read, get, list, prompt, send-keys, attach, and explain, plus Herdr API snapshots.`

	agentStatusContext = `Watcher status reporting:
- Append status updates to this worker-specific file: %s
- Format each line as <verb>: <detail>, where <verb> is exactly one of: working|done|needs-decision|blocked|failed|paused
- The status file is append-only; never overwrite or truncate it.
- Status-file reporting supplements, and does not replace, Fledge progress and completion messaging to the orchestrator.`
)

var (
	legacySessionNamePattern = regexp.MustCompile(`^fledge-[0-9a-f]{32}$`)
	sessionNamePattern       = regexp.MustCompile(`^fledge-[a-z0-9]+(?:-[a-z0-9]+)*-[0-9a-f]{8}$`)
)

// Herdr is the part of the Herdr CLI used by the lifecycle manager.
type Herdr interface {
	Check() error
	Attach(context.Context, string, string) error
	StartServer(string, string, map[string]string) error
	WaitReady(context.Context, string, time.Duration) (herdr.Snapshot, error)
	Snapshot(context.Context, string) (herdr.Snapshot, error)
	CreateWorkspace(context.Context, string, string, string) (herdr.Workspace, herdr.Tab, herdr.Pane, error)
	RenameTab(context.Context, string, string, string) error
	RenamePane(context.Context, string, string, string) error
	SplitPane(context.Context, string, string, string, map[string]string) (herdr.Pane, error)
	CreateTab(context.Context, string, string, string, string, map[string]string) (herdr.Tab, herdr.Pane, error)
	CloseTab(context.Context, string, string) error
	ClosePane(context.Context, string, string) error
	FocusAgent(context.Context, string, string) error
	StartAgent(context.Context, string, string, string, string, time.Duration, []string) error
	PromptAgent(context.Context, string, string, string) error
	List(context.Context) ([]herdr.Session, error)
	Protocol(context.Context) (int, error)
	Stop(context.Context, string) error
	Delete(context.Context, string) error
}

// Confirmer asks the user to approve a destructive operation.
type Confirmer interface {
	Confirm(string) (bool, error)
}

// Manager coordinates project state, Herdr, and user confirmation.
type Manager struct {
	herdr         Herdr
	confirmer     Confirmer
	output        io.Writer
	random        io.Reader
	selector      selectionResolver
	lookPath      harness.LookPath
	getenv        func(string) string
	logFactory    func(root, session string) (*slog.Logger, io.Closer, error)
	watchLauncher func(root string) error
	watchRunner   func(context.Context, watchproc.Options) error
	watchStopper  func(root, session string) error
	homeDir       func() (string, error)
	contextDeps   func(context.Context, string) agentcontext.Deps
}

type selectionResolver interface {
	Select(context.Context, tui.SelectionRequest) (tui.Selection, error)
}

// NewManager creates a session lifecycle manager.
func NewManager(client Herdr, confirmer Confirmer, input io.Reader, output io.Writer) *Manager {
	manager := &Manager{
		herdr:         client,
		confirmer:     confirmer,
		output:        output,
		random:        rand.Reader,
		lookPath:      exec.LookPath,
		getenv:        os.Getenv,
		watchLauncher: launchWatcher,
		watchRunner:   watchproc.Run,
		watchStopper:  watchproc.Stop,
		homeDir:       os.UserHomeDir,
		contextDeps:   agentcontext.ProductionDeps,
	}
	stdin, stdinOK := input.(*os.File)
	stdout, stdoutOK := output.(*os.File)
	if stdinOK && stdoutOK {
		manager.selector = tui.NewSelector(stdin, stdout)
	} else {
		manager.selector = (*tui.Selector)(nil)
	}
	manager.logFactory = func(root, session string) (*slog.Logger, io.Closer, error) {
		return logging.Open(statedir.Session(root, session), logging.ParseLevel(manager.getenv(logging.LevelEnvVar)))
	}
	return manager
}

// SetOutput redirects the manager's plain output. The writer passed to
// NewManager still decides whether the interactive picker is available, so
// callers that need terminal detection must construct with the real file and
// redirect afterwards.
func (m *Manager) SetOutput(output io.Writer) {
	m.output = output
}

// sessionLogger opens the session's debug log. Logging must never break a
// lifecycle operation, so on failure it warns on output and discards records.
func (m *Manager) sessionLogger(root, session string) (*slog.Logger, func()) {
	logger, closer, err := m.logFactory(root, session)
	if err != nil {
		fmt.Fprintf(m.output, "Warning: session debug log unavailable: %v\n", err)
		return logging.Discard(), func() {}
	}
	return logger, func() { _ = closer.Close() }
}

// logSessionEvent records a single event for commands that do not hold a
// session logger open.
func (m *Manager) logSessionEvent(root, session, message string, args ...any) {
	logger, closeLog := m.sessionLogger(root, session)
	logger.Info(message, args...)
	closeLog()
}

// logOutcome records a command's terminal result on the session debug log.
func logOutcome(logger *slog.Logger, command string, err error) {
	if err != nil {
		logger.Error("command failed", "command", command, "err", err.Error())
		return
	}
	logger.Debug("command completed", "command", command)
}

// Init creates or validates tracked Fledge project metadata.
func (m *Manager) Init(path string) (string, error) {
	root, err := project.Init(path)
	if err != nil {
		return "", err
	}
	_, err = fmt.Fprintf(m.output, "Initialized Fledge project in %s.\n", root)
	return root, err
}

// Start launches or attaches to the session belonging to dir.
func (m *Manager) Start(ctx context.Context, dir string, options StartOptions) error {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	if err := project.EnsureRuntimeIgnore(root); err != nil {
		return err
	}
	if err := m.herdr.Check(); err != nil {
		return err
	}
	existingRecord, recordFound, err := readRecord(root)
	if err != nil {
		return err
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return err
	}
	if recordFound {
		if session, exists := sessionByName(sessions, existingRecord.SessionName); exists && session.Running {
			if options.HasSelection() {
				return errors.New("startup selection flags cannot be used when reattaching to an existing orchestrator")
			}
			m.logSessionEvent(root, existingRecord.SessionName, "reattached to running session", "session", existingRecord.SessionName)
			m.launchWatcherWarn(root)
			return m.herdr.Attach(ctx, existingRecord.SessionName, root)
		}
	}
	if err := ValidateAgentTimeout(options.Timeout); err != nil {
		return err
	}

	profile, err := project.LoadOrchestratorProfile(root)
	if err != nil {
		return err
	}
	_, selectedHarness, nativeArgs, _, err := m.resolveSelection(ctx, selectionInput{
		Name: "orchestrator", Harness: options.Harness, Model: options.Model, ModelSet: options.ModelSet,
		NativeArgs: options.NativeArgs, Root: root,
	})
	if err != nil {
		return err
	}
	if selectedHarness.ID == "codex" {
		if err := project.EnsureCodexRules(root); err != nil {
			return err
		}
	}

	value := existingRecord
	recordCreated := false
	if !recordFound {
		value, recordCreated, err = m.loadOrCreateRecord(root)
		if err != nil {
			return err
		}
		if !recordCreated {
			return errors.New("another fledge start initialized this project concurrently; retry the command")
		}
	}
	unlock, err := lockSessionRecord(ctx, root)
	if err != nil {
		if recordCreated {
			return errors.Join(err, removeRecordIfMatches(root, value.SessionName))
		}
		return err
	}
	if err := revalidateLockedRecord(root, value.SessionName, recordCreated); err != nil {
		return errors.Join(err, unlock())
	}
	if recordFound {
		lockedSessions, listErr := m.herdr.List(ctx)
		if listErr != nil {
			return errors.Join(listErr, unlock())
		}
		if session, exists := sessionByName(lockedSessions, value.SessionName); exists && session.Running {
			if unlockErr := unlock(); unlockErr != nil {
				return unlockErr
			}
			if options.HasSelection() {
				return errors.New("startup selection flags cannot be used when reattaching to an existing orchestrator")
			}
			m.logSessionEvent(root, value.SessionName, "reattached to running session", "session", value.SessionName)
			m.launchWatcherWarn(root)
			return m.herdr.Attach(ctx, value.SessionName, root)
		}
	}
	generatedInstructions := orchestratorInstructions(profile.Instructions, "")
	generatedPrompt, err := project.EnsureGeneratedOrchestratorPrompt(root, generatedInstructions)
	if err != nil {
		return errors.Join(err, unlock())
	}
	instructions := orchestratorInstructions(profile.Instructions, selectedHarness.ID)
	nativeArgs, err = harness.AppendOrchestratorInstructions(selectedHarness, nativeArgs, instructions, generatedPrompt)
	if err != nil {
		return errors.Join(err, unlock())
	}
	runtime := openCodeRuntime{}
	if selectedHarness.ID == "opencode" {
		runtime, err = prepareOpenCodeRuntime(root, value.SessionName, generatedPrompt, m.getenv(openCodeConfigEnvironment))
		if err != nil {
			var recordErr error
			if recordCreated {
				recordErr = removeRecordIfMatches(root, value.SessionName)
			}
			watchErr := m.watchStopper(root, value.SessionName)
			var temporaryErr error
			if watchErr == nil {
				temporaryErr = removeSessionTemporaryState(root, value.SessionName)
			}
			return errors.Join(err, watchErr, temporaryErr, recordErr, unlock())
		}
	}
	logger, closeLog := m.sessionLogger(root, value.SessionName)
	logger.Info("start invoked", "session", value.SessionName, "harness", selectedHarness.ID, "timeout", options.Timeout.String(), "native_args", len(nativeArgs))
	serverOwned, initErr := m.initializeOrchestrator(ctx, logger, root, value.SessionName, selectedHarness, options.Timeout, nativeArgs, runtime)
	if initErr != nil {
		logger.Error("orchestrator start failed", "session", value.SessionName, "server_owned", serverOwned, "err", initErr.Error())
		logger.Warn("rolling back failed start", "session", value.SessionName)
		// Close the log before rollback: RemoveAll deletes the session
		// directory, and an open handle would block that on Windows.
		closeLog()
		cleanupErr := m.rollbackOwnedSession(ctx, root, value.SessionName, serverOwned)
		return errors.Join(initErr, cleanupErr, unlock())
	}
	logger.Info("orchestrator started", "session", value.SessionName)
	closeLog()
	if err := unlock(); err != nil {
		return fmt.Errorf("orchestrator started, but release of its startup lock failed: %w", err)
	}

	m.launchWatcherWarn(root)
	return m.herdr.Attach(ctx, value.SessionName, root)
}

// revalidateLockedRecord re-reads the session record after the startup lock is
// acquired, removing a record this start attempt created when it can no longer
// be read back.
func revalidateLockedRecord(root, sessionName string, recordCreated bool) error {
	lockedRecord, stillPresent, readErr := readRecord(root)
	if readErr != nil {
		if recordCreated {
			return errors.Join(readErr, removeRecord(root))
		}
		return readErr
	}
	if !stillPresent || lockedRecord.SessionName != sessionName {
		return errors.New("Fledge session record changed while waiting for startup; retry the command")
	}
	return nil
}

type selectionInput struct {
	Name       string
	Harness    string
	Model      string
	ModelSet   bool
	NativeArgs []string
	Snapshot   herdr.Snapshot
	SnapshotOK bool
	Root       string
}

func (m *Manager) resolveSelection(ctx context.Context, input selectionInput) (tui.Selection, harness.Harness, []string, tui.CallerKind, error) {
	installed := harness.Installed(m.lookPath)
	harnessChoices := make([]tui.Choice, 0, len(installed))
	for _, candidate := range installed {
		harnessChoices = append(harnessChoices, tui.Choice{Value: candidate.ID, Label: candidate.Name})
	}

	caller := callerInput(m.getenv("HERDR_PANE_ID"), input.Snapshot, input.SnapshotOK)
	var lastUsed *tui.LastUsed
	if input.Harness == "" && input.Root != "" {
		saved, found, err := m.loadPreferences(input.Root)
		if err != nil {
			return tui.Selection{}, harness.Harness{}, nil, tui.ClassifyCaller(caller), err
		}
		if found {
			if remembered, ok := harness.Resolve(installed, saved.Harness); ok {
				lastUsed = &tui.LastUsed{Harness: remembered.ID, Model: saved.Model}
			}
		}
	}

	request := tui.SelectionRequest{
		Name: input.Name, Harness: input.Harness, Model: input.Model, ModelSet: input.ModelSet,
		Harnesses: harnessChoices, Caller: caller, LastUsed: lastUsed,
		Models: func(ctx context.Context, harnessID string) ([]tui.Choice, error) {
			selected, ok := harness.Resolve(installed, harnessID)
			if !ok {
				return nil, fmt.Errorf("harness %q is not installed", harnessID)
			}
			catalog := harness.Discover(ctx, selected, harness.DiscoveryOptions{})
			if catalog.Warning != "" {
				if _, err := fmt.Fprintf(m.output, "Warning: %s\n", catalog.Warning); err != nil {
					return nil, err
				}
			}
			return modelChoices(catalog.Models), nil
		},
	}
	selection, err := m.selector.Select(ctx, request)
	if err != nil {
		return tui.Selection{}, harness.Harness{}, nil, tui.ClassifyCaller(caller), err
	}
	selected, ok := harness.Resolve(installed, selection.Harness)
	if !ok {
		return tui.Selection{}, harness.Harness{}, nil, tui.ClassifyCaller(caller), fmt.Errorf("harness %q is not installed", selection.Harness)
	}
	nativeArgs, err := harness.BuildArgs(selected, selection.Model, input.NativeArgs)
	if err != nil {
		return tui.Selection{}, harness.Harness{}, nil, tui.ClassifyCaller(caller), err
	}
	if selection.Prompted && input.Root != "" {
		saveErr := writePreferences(input.Root, preferences{Version: preferencesVersion, Harness: selected.ID, Model: selection.Model})
		if saveErr != nil {
			if _, err := fmt.Fprintf(m.output, "Warning: could not save picker preferences: %v\n", saveErr); err != nil {
				return tui.Selection{}, harness.Harness{}, nil, tui.ClassifyCaller(caller), err
			}
		}
	}
	return selection, selected, nativeArgs, tui.ClassifyCaller(caller), nil
}

// loadPreferences reads remembered picker preferences, treating a corrupt file
// as absent so a stale cache never blocks startup.
func (m *Manager) loadPreferences(root string) (preferences, bool, error) {
	value, found, err := readPreferences(root)
	if err != nil {
		if _, writeErr := fmt.Fprintf(m.output, "Warning: ignoring saved picker preferences: %v\n", err); writeErr != nil {
			return preferences{}, false, writeErr
		}
		return preferences{}, false, nil
	}
	return value, found, nil
}

func callerInput(paneID string, snapshot herdr.Snapshot, available bool) tui.CallerInput {
	input := tui.CallerInput{PaneID: paneID, SessionAgentsAvailable: available}
	for _, pane := range snapshot.Panes {
		input.PaneIDs = append(input.PaneIDs, pane.PaneID)
	}
	for _, agent := range snapshot.Agents {
		kind := ""
		if agent.Agent != nil {
			kind = *agent.Agent
		}
		input.Agents = append(input.Agents, tui.PaneAgent{
			PaneID: agent.PaneID, Harness: kind, Recognized: agent.Agent != nil || agent.Name != nil,
		})
	}
	return input
}

func modelChoices(models []harness.Model) []tui.Choice {
	choices := make([]tui.Choice, 0, len(models))
	for _, model := range models {
		group := model.Maker
		if model.Provider != "" {
			group = harness.ProviderName(model.Provider)
			if harness.ProviderUsesCreatorGroups(model.Provider) && model.Maker != "" {
				group += " / " + model.Maker
			}
		}
		choices = append(choices, tui.Choice{Value: model.ID, Label: model.Name, Group: group})
	}
	return choices
}

func (m *Manager) initializeOrchestrator(
	ctx context.Context,
	logger *slog.Logger,
	root, session string,
	selectedHarness harness.Harness,
	timeout time.Duration,
	nativeArgs []string,
	runtime openCodeRuntime,
) (bool, error) {
	if err := m.herdr.StartServer(session, root, runtime.serverEnvironment); err != nil {
		return false, err
	}
	logger.Debug("herdr server started", "session", session)
	messagingSessionID, err := messaging.New(root, session).Initialize()
	if err != nil {
		return true, fmt.Errorf("initialize session messaging: %w", err)
	}
	logger.Debug("session messaging initialized", "messaging_session_id", messagingSessionID)
	snapshot, err := m.herdr.WaitReady(ctx, session, 10*time.Second)
	if err != nil {
		return true, err
	}
	logger.Debug("herdr session ready", "tabs", len(snapshot.Tabs), "panes", len(snapshot.Panes))
	var tab herdr.Tab
	var pane herdr.Pane
	if len(snapshot.Tabs) == 0 && len(snapshot.Panes) == 0 {
		_, tab, pane, err = m.herdr.CreateWorkspace(ctx, session, root, "orchestrator")
	} else {
		tab, pane, err = initialLayout(snapshot)
	}
	if err != nil {
		return true, err
	}
	logger.Debug("orchestrator layout selected", "tab", tab.TabID, "pane", pane.PaneID)
	if err := m.herdr.RenameTab(ctx, session, tab.TabID, "orchestrator"); err != nil {
		return true, err
	}
	if err := m.herdr.RenamePane(ctx, session, pane.PaneID, "orchestrator"); err != nil {
		return true, err
	}
	if _, err := m.herdr.SplitPane(ctx, session, pane.PaneID, root, runtime.paneEnvironment); err != nil {
		return true, err
	}
	if err := m.herdr.StartAgent(ctx, session, "orchestrator", selectedHarness.ID, pane.PaneID, timeout, nativeArgs); err != nil {
		return true, err
	}
	logger.Debug("orchestrator agent started", "harness", selectedHarness.ID, "pane", pane.PaneID)
	if err := m.herdr.FocusAgent(ctx, session, "orchestrator"); err != nil {
		return true, err
	}
	return true, nil
}

func initialLayout(snapshot herdr.Snapshot) (herdr.Tab, herdr.Pane, error) {
	if len(snapshot.Tabs) == 0 || len(snapshot.Panes) == 0 {
		return herdr.Tab{}, herdr.Pane{}, errors.New("fresh Herdr session has no initial tab and pane")
	}
	tab := snapshot.Tabs[0]
	for _, candidate := range snapshot.Tabs {
		if candidate.TabID == snapshot.FocusedTabID {
			tab = candidate
			break
		}
	}
	for _, candidate := range snapshot.Panes {
		if candidate.PaneID == snapshot.FocusedPaneID && candidate.TabID == tab.TabID {
			return tab, candidate, nil
		}
	}
	for _, candidate := range snapshot.Panes {
		if candidate.TabID == tab.TabID {
			return tab, candidate, nil
		}
	}
	return herdr.Tab{}, herdr.Pane{}, fmt.Errorf("initial Herdr tab %q has no root pane", tab.TabID)
}

func orchestratorInstructions(profileInstructions, harnessID string) string {
	instructions := profileInstructions
	if harnessID == "codex" {
		instructions += "\n\n" + codexCoordinatorGuidance
	}
	return instructions + "\n\n" + mandatoryCoordinatorCommunicationPolicy
}

func (m *Manager) rollbackOwnedSession(ctx context.Context, root, session string, serverOwned bool) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if serverOwned {
		stopErr := m.herdr.Stop(cleanupCtx, session)
		deleteErr := m.herdr.Delete(cleanupCtx, session)
		if deleteErr != nil {
			return errors.Join(stopErr, deleteErr)
		}
		if watcherErr := m.watchStopper(root, session); watcherErr != nil {
			return errors.Join(stopErr, watcherErr)
		}
		return errors.Join(
			stopErr,
			messaging.New(root, session).RemoveAll(),
			removeOpenCodeRuntime(root, session),
			removeSessionTemporaryState(root, session),
			removeRecordIfMatches(root, session),
		)
	}
	if watcherErr := m.watchStopper(root, session); watcherErr != nil {
		return watcherErr
	}
	return errors.Join(removeOpenCodeRuntime(root, session), removeSessionTemporaryState(root, session), removeRecordIfMatches(root, session))
}

// Spawn starts an ad-hoc agent in a new tab of the running project session.
func (m *Manager) Spawn(ctx context.Context, dir string, options SpawnOptions) (resultErr error) {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	if err := project.EnsureRuntimeIgnore(root); err != nil {
		return err
	}
	if err := m.herdr.Check(); err != nil {
		return err
	}
	if err := ValidateAgentTimeout(options.Timeout); err != nil {
		return err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("project has no Fledge session; run fledge start first")
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return err
	}
	session, exists := sessionByName(sessions, value.SessionName)
	if !exists || !session.Running {
		return errors.New("project's Fledge session is not running; run fledge start first")
	}
	snapshot, err := m.herdr.Snapshot(ctx, value.SessionName)
	if err != nil {
		return err
	}
	selection, selectedHarness, nativeArgs, caller, err := m.resolveSelection(ctx, selectionInput{
		Name: options.Name, Harness: options.Harness, Model: options.Model, ModelSet: options.ModelSet,
		NativeArgs: options.NativeArgs, Snapshot: snapshot, SnapshotOK: true, Root: root,
	})
	if err != nil {
		return err
	}
	if selection.Name == userIdentity || selection.Name == watcherIdentity {
		return fmt.Errorf("agent name %q is reserved for messaging", selection.Name)
	}
	if err := rejectDuplicate(selection.Name, snapshot); err != nil {
		return err
	}
	cwd, err := spawnDirectory(dir, root, options.Cwd)
	if err != nil {
		return err
	}
	workspaceID, err := targetWorkspace(snapshot)
	if err != nil {
		return err
	}
	if selectedHarness.ID == "codex" {
		if err := project.EnsureCodexRules(root); err != nil {
			return err
		}
	}
	paneEnvironment, err := openCodePaneEnvironment(root, value.SessionName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(statedir.StatusDir(root, value.SessionName), 0o700); err != nil {
		return fmt.Errorf("create watcher status directory: %w", err)
	}
	logger, closeLog := m.sessionLogger(root, value.SessionName)
	defer closeLog()
	defer func() { logOutcome(logger, "agent spawn", resultErr) }()
	logger.Info("agent spawn", "name", selection.Name, "harness", selectedHarness.ID, "cwd", cwd)
	tab, pane, err := m.herdr.CreateTab(ctx, value.SessionName, workspaceID, cwd, selection.Name, paneEnvironment)
	if err != nil {
		return err
	}
	logger.Debug("agent tab created", "name", selection.Name, "tab", tab.TabID, "pane", pane.PaneID)
	rollback := func() error {
		logger.Warn("spawn rollback: closing tab", "name", selection.Name, "tab", tab.TabID)
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return m.herdr.CloseTab(cleanupCtx, value.SessionName, tab.TabID)
	}
	if err := m.herdr.RenamePane(ctx, value.SessionName, pane.PaneID, selection.Name); err != nil {
		return errors.Join(err, rollback())
	}
	if err := m.herdr.StartAgent(ctx, value.SessionName, selection.Name, selectedHarness.ID, pane.PaneID, options.Timeout, nativeArgs); err != nil {
		return errors.Join(err, rollback())
	}
	logger.Info("agent started", "name", selection.Name, "harness", selectedHarness.ID, "pane", pane.PaneID)
	var focusErr error
	if caller == tui.CallerDirectUser {
		if err := m.herdr.FocusAgent(ctx, value.SessionName, selection.Name); err != nil {
			logger.Warn("agent focus failed", "name", selection.Name, "err", err.Error())
			focusErr = fmt.Errorf("agent %q started, but focusing it failed: %w", selection.Name, err)
		}
	}
	prompt := agentMessagingContext + "\n\n" + fmt.Sprintf(agentStatusContext, statedir.StatusFile(root, value.SessionName, selection.Name))
	if options.Prompt != "" {
		prompt += "\n\nYour task:\n" + options.Prompt
	}
	if err := m.herdr.PromptAgent(ctx, value.SessionName, selection.Name, prompt); err != nil {
		promptErr := fmt.Errorf("agent %q initial prompt failed: %w", selection.Name, err)
		return errors.Join(focusErr, promptErr, rollback())
	}
	m.launchWatcherWarn(root)
	m.refreshContext(ctx, root, value.SessionName)
	return focusErr
}

func (m *Manager) launchWatcherWarn(root string) {
	if err := m.watchLauncher(root); err != nil {
		_, _ = fmt.Fprintf(m.output, "Warning: watcher could not be started: %v\n", err)
	}
}

func launchWatcher(root string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve fledge executable: %w", err)
	}
	command, devNull, err := watcherCommand(executable, root)
	if err != nil {
		return err
	}
	defer devNull.Close()
	return startAndReap(command)
}

// watcherCommand builds the detached watcher daemon command. The returned file
// backs its null stdio and must be closed once the command has started.
func watcherCommand(executable, root string) (*exec.Cmd, *os.File, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open null device: %w", err)
	}
	command := exec.Command(executable, "watch", "--daemon")
	command.Dir = root
	command.Stdin = devNull
	command.Stdout = devNull
	command.Stderr = devNull
	command.SysProcAttr = watcherProcessAttributes()
	return command, devNull, nil
}

// startAndReap starts the detached watcher and waits on it in the background,
// so an exiting watcher is collected instead of lingering as a zombie.
func startAndReap(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

// ValidateAgentTimeout rejects agent startup timeouts outside (3s, 5m].
func ValidateAgentTimeout(timeout time.Duration) error {
	milliseconds := timeout.Milliseconds()
	if milliseconds <= 3000 || milliseconds > 300000 {
		return errors.New("agent startup timeout must be greater than 3s and no more than 5m")
	}
	return nil
}

func rejectDuplicate(name string, snapshot herdr.Snapshot) error {
	for _, agent := range snapshot.Agents {
		if agent.Name != nil && *agent.Name == name {
			return fmt.Errorf("a live agent named %q already exists", name)
		}
	}
	for _, tab := range snapshot.Tabs {
		if tab.Label == name {
			return fmt.Errorf("a tab labeled %q already exists", name)
		}
	}
	return nil
}

func spawnDirectory(invocationDir, root, requested string) (string, error) {
	if requested == "" {
		return root, nil
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(invocationDir, requested)
	}
	cwd, err := canonicalDirectory(requested)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, cwd)
	if err != nil {
		return "", fmt.Errorf("compare agent working directory %q with Fledge project %q: %w", cwd, root, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("agent working directory %q is outside the owning Fledge project %q", cwd, root)
	}
	owner, err := project.Find(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve Fledge project for agent working directory %q: %w", cwd, err)
	}
	if owner != root {
		return "", fmt.Errorf("agent working directory %q belongs to Fledge project %q, not the owning project %q", cwd, owner, root)
	}
	return cwd, nil
}

func targetWorkspace(snapshot herdr.Snapshot) (string, error) {
	if snapshot.FocusedWorkspaceID != "" {
		return snapshot.FocusedWorkspaceID, nil
	}
	if len(snapshot.Workspaces) == 0 {
		return "", errors.New("Herdr session has no workspace for the new agent tab")
	}
	return snapshot.Workspaces[0].WorkspaceID, nil
}

// StopAgent closes the pane belonging to one live ad-hoc agent.
func (m *Manager) StopAgent(ctx context.Context, dir, name string) (resultErr error) {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	if err := tui.ValidateAgentName(name); err != nil {
		return err
	}
	if name == "orchestrator" {
		return errors.New("cannot stop the orchestrator with fledge agent stop; use fledge stop instead")
	}
	if err := m.herdr.Check(); err != nil {
		return err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("project has no Fledge session; run fledge start first")
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return err
	}
	session, exists := sessionByName(sessions, value.SessionName)
	if !exists || !session.Running {
		return errors.New("project's Fledge session is not running; run fledge start first")
	}
	snapshot, err := m.herdr.Snapshot(ctx, value.SessionName)
	if err != nil {
		return err
	}
	logger, closeLog := m.sessionLogger(root, value.SessionName)
	defer closeLog()
	defer func() { logOutcome(logger, "agent stop", resultErr) }()
	logger.Info("agent stop", "name", name)
	paneID := ""
	for _, agent := range snapshot.Agents {
		if agent.Name != nil && *agent.Name == name {
			paneID = agent.PaneID
			break
		}
	}
	if paneID == "" {
		return fmt.Errorf("live agent %q was not found in the project's Fledge session", name)
	}
	if err := m.herdr.ClosePane(ctx, value.SessionName, paneID); err != nil {
		return err
	}
	logger.Info("agent pane closed", "name", name, "pane", paneID)
	m.refreshContext(ctx, root, value.SessionName)
	_, err = fmt.Fprintf(m.output, "Stopped agent %s and closed pane %s.\n", name, paneID)
	return err
}

// Stop confirms and removes the nearest Fledge session at or above dir.
func (m *Manager) Stop(ctx context.Context, dir string) error {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return err
	}
	if !found {
		_, err = fmt.Fprintf(m.output, "No active Fledge session found for %s.\n", root)
		return err
	}
	unlock, err := lockSessionRecord(ctx, root)
	if err != nil {
		return err
	}
	lockedValue, stillPresent, readErr := readRecord(root)
	if readErr != nil {
		return errors.Join(readErr, unlock())
	}
	if !stillPresent || lockedValue.SessionName != value.SessionName {
		return errors.Join(errors.New("Fledge session record changed while waiting to stop it; retry the command"), unlock())
	}
	logger, closeLog := m.sessionLogger(root, value.SessionName)
	defer closeLog()
	logger.Info("stop invoked", "session", value.SessionName)
	err = m.stopLocked(ctx, logger, root, value)
	logOutcome(logger, "stop", err)
	return errors.Join(err, unlock())
}

func (m *Manager) stopLocked(ctx context.Context, logger *slog.Logger, root string, value record) error {

	session, exists, err := m.resolveHerdrSession(ctx, value.SessionName)
	if err != nil {
		return err
	}

	confirmed, err := m.confirmStop(root, value.SessionName, session, exists)
	if !confirmed {
		logger.Info("stop canceled", "session", value.SessionName)
		return err
	}
	if err := m.watchStopper(root, value.SessionName); err != nil {
		return fmt.Errorf("stop session watcher: %w", err)
	}

	if !exists {
		return m.removeStaleSessionRecord(logger, root, value.SessionName)
	}

	return m.stopAndDeleteSession(ctx, logger, root, value, session)
}

func (m *Manager) resolveHerdrSession(ctx context.Context, name string) (herdr.Session, bool, error) {
	if err := m.herdr.Check(); err != nil {
		return herdr.Session{}, false, err
	}
	sessions, err := m.herdr.List(ctx)
	if err != nil {
		return herdr.Session{}, false, err
	}

	session, exists := sessionByName(sessions, name)
	return session, exists, nil
}

func (m *Manager) confirmStop(root, name string, session herdr.Session, exists bool) (bool, error) {
	confirmed, err := m.confirmer.Confirm(stopQuestion(root, name, session, exists))
	if err != nil {
		return false, err
	}
	if confirmed {
		return true, nil
	}

	_, err = fmt.Fprintln(m.output, "Canceled.")
	return false, err
}

func (m *Manager) removeStaleSessionRecord(logger *slog.Logger, root, session string) error {
	if err := removeRecord(root); err != nil {
		return err
	}
	if err := errors.Join(
		messaging.New(root, session).RemoveLock(),
		removeOpenCodeRuntime(root, session),
		removeSessionTemporaryState(root, session),
	); err != nil {
		return err
	}
	logger.Info("stale session record removed", "session", session)
	_, err := fmt.Fprintf(m.output, "Removed stale Fledge session record for %s.\n", root)
	return err
}

func (m *Manager) stopAndDeleteSession(ctx context.Context, logger *slog.Logger, root string, value record, session herdr.Session) error {
	if session.Running {
		if err := removeRecord(root); err != nil {
			return err
		}
		if err := m.herdr.Stop(ctx, value.SessionName); err != nil {
			return restoreRecord(root, value, err)
		}
	}
	if err := m.herdr.Delete(ctx, value.SessionName); err != nil {
		if session.Running {
			return restoreRecord(root, value, err)
		}
		return err
	}
	if !session.Running {
		if err := removeRecord(root); err != nil {
			return err
		}
	}
	if err := errors.Join(
		messaging.New(root, value.SessionName).RemoveLock(),
		removeOpenCodeRuntime(root, value.SessionName),
		removeSessionTemporaryState(root, value.SessionName),
	); err != nil {
		return err
	}
	logger.Info("session stopped and deleted", "session", value.SessionName)

	_, err := fmt.Fprintf(m.output, "Stopped and deleted Fledge session %s.\n", value.SessionName)
	return err
}

type record struct {
	Version     int    `json:"version"`
	SessionName string `json:"session_name"`
}

func (m *Manager) loadOrCreateRecord(root string) (record, bool, error) {
	existing, found, err := readRecord(root)
	if err != nil {
		return record{}, false, err
	}
	if found {
		if err := initializeStateDirectory(root); err != nil {
			return record{}, false, err
		}
		return existing, false, nil
	}

	if err := initializeStateDirectory(root); err != nil {
		return record{}, false, err
	}

	created := record{Version: recordVersion}
	created.SessionName, err = generateSessionName(root, m.random)
	if err != nil {
		return record{}, false, err
	}

	createdByUs, err := createRecord(root, created)
	if err != nil {
		return record{}, false, err
	}
	if createdByUs {
		return created, true, nil
	}

	// Another start won the race to initialize this directory.
	existing, found, err = readRecord(root)
	if err != nil {
		return record{}, false, err
	}
	if !found {
		return record{}, false, errors.New("Fledge session record disappeared during creation")
	}
	return existing, false, nil
}

func canonicalDirectory(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", resolved)
	}

	return filepath.Clean(resolved), nil
}

func initializeStateDirectory(root string) error {
	dir := filepath.Join(root, stateDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Fledge state directory: %w", err)
	}
	return project.EnsureRuntimeIgnore(root)
}

func generateSessionName(root string, random io.Reader) (string, error) {
	randomBytes := make([]byte, sessionIDBytes)
	if _, err := io.ReadFull(random, randomBytes); err != nil {
		return "", fmt.Errorf("generate Fledge session name: %w", err)
	}
	return sessionNamePrefix + sessionSlug(root) + "-" + hex.EncodeToString(randomBytes), nil
}

func sessionSlug(root string) string {
	var slug strings.Builder
	separatorPending := false

	for _, char := range filepath.Base(root) {
		switch {
		case char >= 'A' && char <= 'Z':
			if separatorPending && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteByte(byte(char + ('a' - 'A')))
			separatorPending = false
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			if separatorPending && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteByte(byte(char))
			separatorPending = false
		default:
			separatorPending = true
		}
	}

	value := slug.String()
	if len(value) > maxSessionSlugLength {
		value = strings.TrimRight(value[:maxSessionSlugLength], "-")
	}
	if value == "" {
		return "cwd"
	}
	return value
}

func createRecord(root string, value record) (bool, error) {
	path := recordPath(root)
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return false, fmt.Errorf("encode Fledge session record: %w", err)
	}
	contents = append(contents, '\n')

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("close %s: %w", path, err)
	}

	return true, nil
}

func readRecord(root string) (record, bool, error) {
	path := recordPath(root)
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return record{}, false, nil
	}
	if err != nil {
		return record{}, false, fmt.Errorf("read %s: %w", path, err)
	}

	var value record
	if err := json.Unmarshal(contents, &value); err != nil {
		return record{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	if value.Version != recordVersion {
		return record{}, false, fmt.Errorf("decode %s: unsupported record version %d", path, value.Version)
	}
	if !validSessionName(value.SessionName) {
		return record{}, false, fmt.Errorf("decode %s: invalid session name %q", path, value.SessionName)
	}

	return value, true, nil
}

func validSessionName(name string) bool {
	if legacySessionNamePattern.MatchString(name) {
		return true
	}
	if !sessionNamePattern.MatchString(name) {
		return false
	}

	slugLength := len(name) - len(sessionNamePrefix) - 1 - sessionIDHexLength
	return slugLength <= maxSessionSlugLength
}

func sessionByName(sessions []herdr.Session, name string) (herdr.Session, bool) {
	for _, session := range sessions {
		if session.Name == name {
			return session, true
		}
	}
	return herdr.Session{}, false
}

func stopQuestion(root, name string, session herdr.Session, exists bool) string {
	if !exists {
		return fmt.Sprintf("Herdr session %q is missing. Remove the stale Fledge session for %s?", name, root)
	}
	if !session.Running {
		return fmt.Sprintf("Delete stopped Fledge session %q for %s?", name, root)
	}
	return fmt.Sprintf("Stop and delete Fledge session %q for %s?", name, root)
}

func removeRecord(root string) error {
	path := recordPath(root)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func removeRecordIfMatches(root, session string) error {
	value, found, err := readRecord(root)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if value.SessionName != session {
		return fmt.Errorf("preserve Fledge session record for %q while cleaning up %q", value.SessionName, session)
	}
	return removeRecord(root)
}

func restoreRecord(root string, value record, operationErr error) error {
	_, err := createRecord(root, value)
	return errors.Join(operationErr, err)
}

func recordPath(root string) string {
	return filepath.Join(root, stateDirectory, recordFilename)
}
