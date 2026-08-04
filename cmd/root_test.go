package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lifecycle"
	"github.com/Harrison-Blair/fledge/internal/messaging"
)

func TestRootCommandShowsHelp(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(nil)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, expected := range []string{"fledge [command]", "agent", "init", "start", "stop", "watch"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("help output = %q, want %q", output.String(), expected)
		}
	}
}

func TestRootCommandCapturesManagerOutput(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	manager := lifecycle.NewManager(nil, nil, nil, io.Discard)
	command := newRootCommand(manager, func() (string, error) { return project, nil })
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"init", project})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "Initialized Fledge project in") {
		t.Errorf("output = %q, want the Manager's own output captured by SetOut", output.String())
	}
}

func TestWatchRoutesAttachedAndDaemonModes(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		args   []string
		daemon bool
	}{
		{name: "attached", args: []string{"watch"}},
		{name: "daemon", args: []string{"watch", "--daemon"}, daemon: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(manager.watchDirs) != 1 || manager.watchDirs[0] != "/project/nested" || len(manager.watchOptions) != 1 || manager.watchOptions[0].Daemon != test.daemon {
				t.Fatalf("Watch() dirs/options = %#v/%#v", manager.watchDirs, manager.watchOptions)
			}
		})
	}
}

func TestWatchDaemonFlagIsHidden(t *testing.T) {
	t.Parallel()

	command := newWatchCommand(&fakeManager{}, func() (string, error) { return "/project", nil })
	flag := command.Flags().Lookup("daemon")
	if flag == nil || !flag.Hidden {
		t.Fatalf("daemon flag = %#v, want hidden", flag)
	}
}

func TestStartOptionsAndNativeArguments(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
	command.SetArgs([]string{"start", "-k", "codex", "-m", "gpt-custom", "-t", "45s", "--", "--sandbox", "read-only"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.startOptions) != 1 {
		t.Fatalf("Start() options = %#v", manager.startOptions)
	}
	got := manager.startOptions[0]
	if got.Harness != "codex" || got.Model != "gpt-custom" || got.Timeout != 45*time.Second || !got.HarnessSet || !got.ModelSet || !got.TimeoutSet {
		t.Errorf("StartOptions = %#v", got)
	}
	if strings.Join(got.NativeArgs, " ") != "--sandbox read-only" {
		t.Errorf("native args = %#v", got.NativeArgs)
	}
}

func TestAgentSpawnOptionsAndNativeArguments(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
	command.SetArgs([]string{"agent", "spawn", "-n", "reviewer", "-k", "claude", "-m", "opus", "-C", "pkg", "--prompt", "Review", "-t", "1m", "--", "--dangerously-skip-permissions"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.spawnOptions) != 1 || len(manager.spawnDirs) != 1 {
		t.Fatalf("Spawn() = dirs %#v options %#v", manager.spawnDirs, manager.spawnOptions)
	}
	got := manager.spawnOptions[0]
	if got.Name != "reviewer" || got.Harness != "claude" || got.Model != "opus" || got.Cwd != "pkg" || got.Prompt != "Review" || got.Timeout != time.Minute || !got.ModelSet {
		t.Errorf("SpawnOptions = %#v", got)
	}
	if strings.Join(got.NativeArgs, " ") != "--dangerously-skip-permissions" {
		t.Errorf("native args = %#v", got.NativeArgs)
	}
}

func TestNativeArgumentsMustNotPrecedeDash(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"start", "bogus", "--", "--resume"},
		{"agent", "spawn", "reviewer", "--", "--resume"},
	} {
		manager := &fakeManager{}
		command := newRootCommand(manager, func() (string, error) { return "/project", nil })
		command.SetArgs(args)
		err := command.Execute()
		if err == nil || !strings.Contains(err.Error(), "must follow --") {
			t.Errorf("Execute(%v) error = %v, want positional rejection", args, err)
		}
		if len(manager.startDirs)+len(manager.spawnDirs) != 0 {
			t.Errorf("Execute(%v) reached the manager: %#v", args, manager)
		}
	}
}

func TestStartAcceptsNativeArgumentsAfterDash(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"start", "--", "--flag"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(manager.startOptions) != 1 || strings.Join(manager.startOptions[0].NativeArgs, " ") != "--flag" {
		t.Fatalf("StartOptions = %#v", manager.startOptions)
	}
}

func TestAgentStopUsesNameAndCurrentDirectory(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
	command.SetArgs([]string{"agent", "stop", "reviewer"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.stopAgentDirs) != 1 || manager.stopAgentDirs[0] != "/project/nested" {
		t.Fatalf("StopAgent() dirs = %#v", manager.stopAgentDirs)
	}
	if len(manager.stopAgentNames) != 1 || manager.stopAgentNames[0] != "reviewer" {
		t.Fatalf("StopAgent() names = %#v", manager.stopAgentNames)
	}
}

func TestAgentStopRequiresExactlyOneName(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"agent", "stop"}, {"agent", "stop", "one", "two"}} {
		manager := &fakeManager{}
		command := newRootCommand(manager, func() (string, error) { return "/project", nil })
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Errorf("Execute(%v) error = nil", args)
		}
		if len(manager.stopAgentNames) != 0 {
			t.Errorf("Execute(%v) called StopAgent: %#v", args, manager.stopAgentNames)
		}
	}
}

func TestAgentStopWrapsCurrentDirectoryError(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "", errors.New("cwd failed") })
	command.SetArgs([]string{"agent", "stop", "reviewer"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "get current directory") {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(manager.stopAgentNames) != 0 {
		t.Fatalf("StopAgent() names = %#v", manager.stopAgentNames)
	}
}

func TestAgentMessageCommandsRouteAndPrintResults(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{
		sendResult:  messaging.Message{ID: "msg-send", Recipient: "reviewer"},
		replyResult: messaging.Message{ID: "msg-reply"},
		inboxResult: []messaging.Message{
			{ID: "msg-failed", Sender: "user", Recipient: "reviewer", Body: "outgoing", Status: messaging.StatusFailed, Failure: "submission\nrejected"},
			{ID: "msg-inbox", Sender: "reviewer", Recipient: "user", ReplyTo: "msg-send", Body: "line one\nline two", Status: messaging.StatusDelivered},
		},
	}
	var output bytes.Buffer

	for _, args := range [][]string{
		{"agent", "message", "send", "reviewer", "please review"},
		{"agent", "message", "reply", "msg-send", "looks good"},
		{"agent", "message", "inbox", "user"},
	} {
		command := newRootCommand(manager, func() (string, error) { return "/project/nested", nil })
		command.SetOut(&output)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
	}

	if len(manager.messageSends) != 1 || manager.messageSends[0] != (messageSendCall{"/project/nested", "reviewer", "please review"}) {
		t.Errorf("SendMessage calls = %#v", manager.messageSends)
	}
	if len(manager.messageReplies) != 1 || manager.messageReplies[0] != (messageReplyCall{"/project/nested", "msg-send", "looks good"}) {
		t.Errorf("ReplyMessage calls = %#v", manager.messageReplies)
	}
	if len(manager.inboxCalls) != 1 || manager.inboxCalls[0] != (inboxCall{"/project/nested", "user"}) {
		t.Errorf("MessageInbox calls = %#v", manager.inboxCalls)
	}
	for _, want := range []string{
		"Sent message msg-send to reviewer.",
		"Replied to message msg-send with msg-reply.",
		"msg-failed  failed  sent to reviewer",
		"failure: submission\n  rejected",
		"msg-inbox  delivered  received from reviewer",
		"reply-to: msg-send",
		"line one\n  line two",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output = %q, want %q", output.String(), want)
		}
	}
}

func TestAgentMessageArgumentAndBodyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "send missing body", args: []string{"agent", "message", "send", "worker"}},
		{name: "send extra body", args: []string{"agent", "message", "send", "worker", "body", "extra"}},
		{name: "reply missing body", args: []string{"agent", "message", "reply", "id"}},
		{name: "ack unavailable", args: []string{"agent", "message", "ack", "id"}},
		{name: "inbox extra identity", args: []string{"agent", "message", "inbox", "user", "worker"}},
		{name: "blank body", args: []string{"agent", "message", "send", "worker", " \n\t"}},
		{name: "invalid UTF-8", args: []string{"agent", "message", "reply", "id", string([]byte{0xff})}},
		{name: "oversize body", args: []string{"agent", "message", "send", "worker", strings.Repeat("x", messaging.MaxBodyBytes+1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) { return "/project", nil })
			command.SetArgs(test.args)
			if err := command.Execute(); err == nil {
				t.Fatalf("Execute(%v) error = nil", test.args)
			}
			if len(manager.messageSends)+len(manager.messageReplies)+len(manager.inboxCalls) != 0 {
				t.Fatalf("manager called for invalid args: %#v", manager)
			}
		})
	}
}

func TestAgentMessageWrapsCurrentDirectoryFailures(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"agent", "message", "send", "worker", "body"},
		{"agent", "message", "reply", "id", "body"},
		{"agent", "message", "inbox"},
	} {
		manager := &fakeManager{}
		command := newRootCommand(manager, func() (string, error) { return "", errors.New("cwd failed") })
		command.SetArgs(args)
		err := command.Execute()
		if err == nil || !strings.Contains(err.Error(), "get current directory") {
			t.Errorf("Execute(%v) error = %v", args, err)
		}
	}
}

func TestAgentMessageBodiesComeFromFileOrStdin(t *testing.T) {
	t.Parallel()

	large := strings.Repeat("x", messaging.MaxBodyBytes)
	path := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(path, []byte(large), 0o600); err != nil {
		t.Fatal(err)
	}

	fileManager := &fakeManager{}
	command := newRootCommand(fileManager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"agent", "message", "send", "worker", "-F", path})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(fileManager.messageSends) != 1 || fileManager.messageSends[0].body != large {
		t.Errorf("SendMessage calls = %d, body length = %d, want the whole file", len(fileManager.messageSends), len(fileManager.messageSends[0].body))
	}

	stdinManager := &fakeManager{}
	command = newRootCommand(stdinManager, func() (string, error) { return "/project", nil })
	command.SetIn(strings.NewReader("piped\nbody"))
	command.SetArgs([]string{"agent", "message", "reply", "msg-1", "--file", "-"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(stdinManager.messageReplies) != 1 || stdinManager.messageReplies[0].body != "piped\nbody" {
		t.Errorf("ReplyMessage calls = %#v, want the piped body", stdinManager.messageReplies)
	}
}

func TestAgentMessageBodySourcesAreExclusive(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "body.txt")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "send both", args: []string{"agent", "message", "send", "worker", "inline", "-F", path}, want: "not both"},
		{name: "send neither", args: []string{"agent", "message", "send", "worker"}, want: "--file"},
		{name: "reply both", args: []string{"agent", "message", "reply", "msg-1", "inline", "-F", path}, want: "not both"},
		{name: "reply neither", args: []string{"agent", "message", "reply", "msg-1"}, want: "--file"},
		{name: "missing file", args: []string{"agent", "message", "send", "worker", "-F", filepath.Join(t.TempDir(), "absent")}, want: "read message body"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) { return "/project", nil })
			command.SetArgs(test.args)
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute(%v) error = %v, want %q", test.args, err, test.want)
			}
			if len(manager.messageSends)+len(manager.messageReplies) != 0 {
				t.Fatalf("manager called for an invalid body source: %#v", manager)
			}
		})
	}
}

func TestAgentMessageInboxRendersDirectionForTheResolvedIdentity(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{
		inboxIdentity: "orchestrator",
		inboxResult: []messaging.Message{
			{ID: "msg-1", Sender: "orchestrator", Recipient: "worker", Body: "start", Status: messaging.StatusDelivered},
		},
	}
	var output bytes.Buffer
	command := newRootCommand(manager, func() (string, error) { return "/project", nil })
	command.SetOut(&output)
	command.SetArgs([]string{"agent", "message", "inbox"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "sent to worker") {
		t.Errorf("output = %q, want direction rendered against the manager's resolved identity", output.String())
	}
}

func TestCommandsPropagateManagerErrorsWithoutPrinting(t *testing.T) {
	t.Parallel()

	failure := errors.New("manager failed")
	for _, test := range []struct {
		name    string
		args    []string
		manager *fakeManager
	}{
		{name: "init", args: []string{"init", "/explicit"}, manager: &fakeManager{initErr: failure}},
		{name: "send", args: []string{"agent", "message", "send", "worker", "body"}, manager: &fakeManager{sendErr: failure}},
		{name: "reply", args: []string{"agent", "message", "reply", "msg-1", "body"}, manager: &fakeManager{replyErr: failure}},
		{name: "inbox", args: []string{"agent", "message", "inbox"}, manager: &fakeManager{inboxErr: failure}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			command := newRootCommand(test.manager, func() (string, error) { return "/project", nil })
			command.SetOut(&output)
			command.SetArgs(test.args)
			if err := command.Execute(); !errors.Is(err, failure) {
				t.Fatalf("Execute(%v) error = %v, want %v", test.args, err, failure)
			}
			if output.Len() != 0 {
				t.Errorf("output = %q, want nothing printed for a failed command", output.String())
			}
		})
	}
}

func TestAgentMessageEmptyInboxOutput(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	command := newRootCommand(&fakeManager{}, func() (string, error) { return "/project", nil })
	command.SetOut(&output)
	command.SetArgs([]string{"agent", "message", "inbox"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "Inbox is empty.\n" {
		t.Errorf("output = %q", output.String())
	}
}

func TestInitUsesExplicitOrCurrentPath(t *testing.T) {
	t.Parallel()

	manager := &fakeManager{}
	command := newRootCommand(manager, func() (string, error) { return "/current", nil })
	command.SetArgs([]string{"init", "/explicit"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	command = newRootCommand(manager, func() (string, error) { return "/current", nil })
	command.SetArgs([]string{"init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(manager.initPaths, ","); got != "/explicit,/current" {
		t.Errorf("Init() paths = %q", got)
	}
}

func TestCommandsForwardTimeoutsToTheManagerUnchanged(t *testing.T) {
	t.Parallel()

	// The Manager is the single validator, so the commands must not coerce or
	// reject timeouts themselves; a bare command still sends the flag default.
	startManager := &fakeManager{}
	command := newRootCommand(startManager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"start", "-t", "0"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(startManager.startOptions) != 1 || startManager.startOptions[0].Timeout != 0 || !startManager.startOptions[0].TimeoutSet {
		t.Errorf("StartOptions = %#v, want a forwarded zero timeout", startManager.startOptions)
	}

	spawnManager := &fakeManager{}
	command = newRootCommand(spawnManager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"agent", "spawn", "-n", "worker", "-t", "0"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(spawnManager.spawnOptions) != 1 || spawnManager.spawnOptions[0].Timeout != 0 {
		t.Errorf("SpawnOptions = %#v, want a forwarded zero timeout", spawnManager.spawnOptions)
	}

	defaultManager := &fakeManager{}
	command = newRootCommand(defaultManager, func() (string, error) { return "/project", nil })
	command.SetArgs([]string{"start"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(defaultManager.startOptions) != 1 || defaultManager.startOptions[0].Timeout != lifecycle.DefaultAgentTimeout {
		t.Errorf("StartOptions = %#v, want the flag default", defaultManager.startOptions)
	}
}

func TestSessionCommandsUseCurrentDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantStart int
		wantStop  int
	}{
		{name: "start", args: []string{"start"}, wantStart: 1},
		{name: "stop", args: []string{"stop"}, wantStop: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) { return "/project", nil })
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(manager.startDirs) != test.wantStart {
				t.Errorf("Start() calls = %d, want %d", len(manager.startDirs), test.wantStart)
			}
			if len(manager.stopDirs) != test.wantStop {
				t.Errorf("Stop() calls = %d, want %d", len(manager.stopDirs), test.wantStop)
			}
			for _, dir := range append(manager.startDirs, manager.stopDirs...) {
				if dir != "/project" {
					t.Errorf("manager directory = %q, want /project", dir)
				}
			}
		})
	}
}

func TestSessionCommandsRejectArguments(t *testing.T) {
	t.Parallel()

	for _, subcommand := range []string{"start", "stop"} {
		t.Run(subcommand, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) { return "/project", nil })
			command.SetArgs([]string{subcommand, "extra"})

			if err := command.Execute(); err == nil {
				t.Fatal("Execute() error = nil, want argument error")
			}
			if len(manager.startDirs) != 0 || len(manager.stopDirs) != 0 {
				t.Fatalf("manager called with extra arguments: %#v", manager)
			}
		})
	}
}

func TestCurrentDirectoryErrorIsWrapped(t *testing.T) {
	t.Parallel()

	for _, subcommand := range []string{"start", "stop"} {
		t.Run(subcommand, func(t *testing.T) {
			t.Parallel()

			manager := &fakeManager{}
			command := newRootCommand(manager, func() (string, error) {
				return "", errors.New("cwd failed")
			})
			command.SetArgs([]string{subcommand})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "get current directory") {
				t.Fatalf("Execute() error = %v, want wrapped cwd error", err)
			}
			if len(manager.startDirs) != 0 || len(manager.stopDirs) != 0 {
				t.Fatalf("manager called after cwd error: %#v", manager)
			}
		})
	}
}

type fakeManager struct {
	initPaths      []string
	startDirs      []string
	startOptions   []lifecycle.StartOptions
	spawnDirs      []string
	spawnOptions   []lifecycle.SpawnOptions
	stopAgentDirs  []string
	stopAgentNames []string
	stopDirs       []string
	watchDirs      []string
	watchOptions   []lifecycle.WatchOptions
	messageSends   []messageSendCall
	messageReplies []messageReplyCall
	inboxCalls     []inboxCall
	sendResult     messaging.Message
	replyResult    messaging.Message
	inboxResult    []messaging.Message
	initErr        error
	startErr       error
	sendErr        error
	replyErr       error
	inboxErr       error
	inboxIdentity  string
	output         io.Writer
}

type messageSendCall struct{ dir, recipient, body string }
type messageReplyCall struct{ dir, id, body string }
type inboxCall struct{ dir, identity string }

func (f *fakeManager) Init(path string) (string, error) {
	f.initPaths = append(f.initPaths, path)
	return path, f.initErr
}

func (f *fakeManager) Start(_ context.Context, dir string, options lifecycle.StartOptions) error {
	f.startDirs = append(f.startDirs, dir)
	f.startOptions = append(f.startOptions, options)
	return f.startErr
}

func (f *fakeManager) SetOutput(output io.Writer) {
	f.output = output
}

func (f *fakeManager) Stop(_ context.Context, dir string) error {
	f.stopDirs = append(f.stopDirs, dir)
	return nil
}

func (f *fakeManager) Watch(_ context.Context, dir string, options lifecycle.WatchOptions) error {
	f.watchDirs = append(f.watchDirs, dir)
	f.watchOptions = append(f.watchOptions, options)
	return nil
}

func (f *fakeManager) Spawn(_ context.Context, dir string, options lifecycle.SpawnOptions) error {
	f.spawnDirs = append(f.spawnDirs, dir)
	f.spawnOptions = append(f.spawnOptions, options)
	return nil
}

func (f *fakeManager) StopAgent(_ context.Context, dir, name string) error {
	f.stopAgentDirs = append(f.stopAgentDirs, dir)
	f.stopAgentNames = append(f.stopAgentNames, name)
	return nil
}

func (f *fakeManager) SendMessage(_ context.Context, dir, recipient, body string) (messaging.Message, error) {
	f.messageSends = append(f.messageSends, messageSendCall{dir, recipient, body})
	return f.sendResult, f.sendErr
}

func (f *fakeManager) ReplyMessage(_ context.Context, dir, id, body string) (messaging.Message, error) {
	f.messageReplies = append(f.messageReplies, messageReplyCall{dir, id, body})
	return f.replyResult, f.replyErr
}

func (f *fakeManager) MessageInbox(_ context.Context, dir, identity string) ([]messaging.Message, string, error) {
	f.inboxCalls = append(f.inboxCalls, inboxCall{dir, identity})
	if f.inboxIdentity != "" {
		identity = f.inboxIdentity
	}
	return f.inboxResult, identity, f.inboxErr
}
