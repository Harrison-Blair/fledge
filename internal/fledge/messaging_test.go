package fledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/state"
)

func startTestMessageRun(t *testing.T, service *Service) string {
	t.Helper()
	store := service.messageStore()
	runID, err := store.StartRun(messaging.RunHeader{
		Fledge: buildinfo.Current(), Herdr: "test", Protocol: 17,
		ProjectRoot: service.Project.Root, Session: service.Project.Session,
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.ActiveRunID = runID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return runID
}

func seedLiveMessagingAgent(
	t *testing.T,
	service *Service,
	fake *fakeLifecycle,
	runID, name, paneID, tabID, activationID string,
) {
	t.Helper()
	seedMessagingAgent(t, service, fake, runID, name, paneID, tabID, activationID, true)
}

// seedBootingMessagingAgent seeds an agent whose harness process is running
// but has not yet become interactive-ready.
func seedBootingMessagingAgent(
	t *testing.T,
	service *Service,
	fake *fakeLifecycle,
	runID, name, paneID, tabID, activationID string,
) {
	t.Helper()
	seedMessagingAgent(t, service, fake, runID, name, paneID, tabID, activationID, false)
}

func seedMessagingAgent(
	t *testing.T,
	service *Service,
	fake *fakeLifecycle,
	runID, name, paneID, tabID, activationID string,
	interactiveReady bool,
) {
	t.Helper()
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents[name] = state.Agent{
			Name: name, PaneID: paneID, TabID: tabID, Placement: "tab",
			ActivationID: activationID, CWD: service.Project.Root,
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	kind, agentName := "codex", name
	fake.mu.Lock()
	fake.snapshot.Panes = append(fake.snapshot.Panes, herdr.PaneInfo{
		PaneID: paneID, TabID: tabID, WorkspaceID: "w1", Agent: &kind, AgentStatus: StateIdle,
	})
	fake.snapshot.Agents = append(fake.snapshot.Agents, herdr.AgentInfo{
		Agent: &kind, AgentStatus: StateIdle, Name: &agentName,
		PaneID: paneID, TabID: tabID, WorkspaceID: "w1", InteractiveReady: interactiveReady,
	})
	fake.mu.Unlock()
	if _, err := service.messageStore().Append(runID, messaging.Event{
		Type: messaging.EventAgentActivated, Agent: name,
		ActivationID: activationID, PaneID: paneID,
	}); err != nil {
		t.Fatal(err)
	}
}

func messageBySender(t *testing.T, service *Service, runID, sender string) *messaging.Message {
	t.Helper()
	run, err := service.messageStore().ReadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range run.Messages {
		if message.Sender == sender {
			return message
		}
	}
	t.Fatalf("run contains no message from %q: %#v", sender, run.Messages)
	return nil
}

func TestMessageSendInjectAckAndLinkedReply(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "do the thing")
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	promptTarget := fake.promptTarget
	fake.mu.Unlock()
	if sent.Message.Status != messaging.StatusAwaitingAck || promptTarget != "p1" {
		t.Fatalf("send = %#v target=%q", sent, promptTarget)
	}
	service.CallerPaneID = "p1"
	acked, err := service.AckMessage(t.Context(), sent.Message.ID)
	if err != nil || acked.Message.Status != messaging.StatusAcknowledged {
		t.Fatalf("ack = %#v, %v", acked, err)
	}
	if again, err := service.AckMessage(t.Context(), sent.Message.ID); err != nil ||
		again.Message.Acknowledgement.Sequence != acked.Message.Acknowledgement.Sequence {
		t.Fatalf("idempotent ack = %#v, %v", again, err)
	}

	second, err := func() (MessageResult, error) {
		service.CallerPaneID = ""
		return service.SendMessage(t.Context(), "worker", "question")
	}()
	if err != nil {
		t.Fatal(err)
	}
	service.CallerPaneID = "p1"
	reply, err := service.ReplyMessage(t.Context(), second.Message.ID, "answer")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Message.Status != messaging.StatusQueued || reply.Message.ReplyTo != second.Message.ID {
		t.Fatalf("reply = %#v", reply)
	}
	original, err := service.ShowMessage(second.Message.ID)
	if err != nil || original.Status != messaging.StatusAcknowledged ||
		original.Acknowledgement.ViaReply != reply.Message.ID {
		t.Fatalf("original = %#v, %v", original, err)
	}
}

func TestMessageQueuedForStoppedAgentAndFailsAtRunClose(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["sleeper"] = state.Agent{Name: "sleeper", PaneID: "missing"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	result, err := service.SendMessage(t.Context(), "sleeper", "wake up")
	if err != nil || result.Message.Status != messaging.StatusQueued {
		t.Fatalf("send = %#v, %v", result, err)
	}
	if err := service.messages().closeRun(runID, "test close"); err != nil {
		t.Fatal(err)
	}
	message, err := service.ShowMessage(result.Message.ID)
	if err != nil || message.Status != messaging.StatusFailed || message.ActiveRun {
		t.Fatalf("closed message = %#v, %v", message, err)
	}
}

func TestMessageDeliveryClassifiesDefiniteAndAmbiguousFailures(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	fake.mu.Lock()
	fake.failMethod = "agent.prompt"
	fake.mu.Unlock()
	definite, err := service.SendMessage(t.Context(), "worker", "definite")
	if err != nil {
		t.Fatal(err)
	}
	if definite.Message.Status != messaging.StatusQueued || definite.DeliveryError == "" {
		t.Fatalf("definite = %#v", definite)
	}
	fake.mu.Lock()
	fake.failMethod, fake.dropMethod = "", "agent.prompt"
	fake.mu.Unlock()
	ambiguous, err := service.SendMessage(t.Context(), "worker", "ambiguous")
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Message.Status != messaging.StatusUncertain || ambiguous.DeliveryError == "" {
		t.Fatalf("ambiguous = %#v", ambiguous)
	}
	if _, err := service.RetryMessage(t.Context(), ambiguous.Message.ID, true); err == nil ||
		Translate(err).Code != "message_state_conflict" {
		t.Fatalf("uncertain retry error = %v", err)
	}
}

func TestSendToBootingAgentStaysQueuedWithoutDeliveryAttempt(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	seedBootingMessagingAgent(t, service, fake, runID, "worker", "p-worker", "t-worker", "act-worker")

	result, err := service.SendMessage(t.Context(), "worker", "task before readiness")
	if err != nil {
		t.Fatal(err)
	}
	if result.Message.Status != messaging.StatusQueued || result.DeliveryError != "" ||
		len(result.Message.DeliveryAttempts) != 0 {
		t.Fatalf("send to booting agent = %#v", result)
	}
	fake.mu.Lock()
	promptTarget := fake.promptTarget
	fake.mu.Unlock()
	if promptTarget != "" {
		t.Fatalf("prompt was injected into booting pane %q", promptTarget)
	}
}

func TestAgentInteractiveReadinessPredicate(t *testing.T) {
	kind := "codex"
	for _, testCase := range []struct {
		name  string
		agent herdr.AgentInfo
		want  bool
	}{
		{"ready harness", herdr.AgentInfo{Agent: &kind, InteractiveReady: true}, true},
		{"no harness", herdr.AgentInfo{InteractiveReady: true}, false},
		{"not interactive yet", herdr.AgentInfo{Agent: &kind}, false},
		{"launch still pending", herdr.AgentInfo{Agent: &kind, InteractiveReady: true, LaunchPending: true}, false},
	} {
		if got := agentInteractiveReady(testCase.agent); got != testCase.want {
			t.Errorf("%s: ready = %t, want %t", testCase.name, got, testCase.want)
		}
	}
}

func TestFailedAttemptIsRetriedByTheNextDrainInTheSameActivation(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	fake.mu.Lock()
	fake.failMethod = "agent.prompt"
	fake.mu.Unlock()
	sent, err := service.SendMessage(t.Context(), "worker", "flaky delivery")
	if err != nil || sent.Message.Status != messaging.StatusQueued || sent.DeliveryError == "" {
		t.Fatalf("failed send = %#v, %v", sent, err)
	}
	fake.mu.Lock()
	fake.failMethod = ""
	fake.mu.Unlock()

	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	client := &herdr.Client{Socket: serviceSessionSocket(t, service.Binary)}
	if err := service.messages().drainAgentMessages(t.Context(), client, deliveryTarget{
		runID: runID, agent: "worker", paneID: "p1",
		activationID: st.Agents["worker"].ActivationID,
	}); err != nil {
		t.Fatal(err)
	}
	message, err := service.ShowMessage(sent.Message.ID)
	if err != nil || message.Status != messaging.StatusAwaitingAck || len(message.DeliveryAttempts) != 2 {
		t.Fatalf("redelivered = %#v, %v", message, err)
	}
}

func TestDrainDeliversLaterMessagesPastADefiniteFailure(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "gone", CWD: service.Project.Root}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	poisoned, err := service.SendMessage(t.Context(), "worker", "poisoned delivery")
	if err != nil || poisoned.Message.Status != messaging.StatusQueued {
		t.Fatalf("poisoned = %#v, %v", poisoned, err)
	}
	second, err := service.SendMessage(t.Context(), "worker", "healthy delivery")
	if err != nil || second.Message.Status != messaging.StatusQueued {
		t.Fatalf("second = %#v, %v", second, err)
	}
	fake.mu.Lock()
	fake.failPromptContaining = "poisoned"
	fake.mu.Unlock()

	mustStartAgent(t, service, "worker")

	failed, err := service.ShowMessage(poisoned.Message.ID)
	if err != nil || failed.Status != messaging.StatusQueued || len(failed.DeliveryAttempts) != 1 {
		t.Fatalf("poisoned after drain = %#v, %v", failed, err)
	}
	delivered, err := service.ShowMessage(second.Message.ID)
	if err != nil || delivered.Status != messaging.StatusAwaitingAck {
		t.Fatalf("later message = %#v, %v", delivered, err)
	}
}

func TestDeliverActivationExpiryIsRecordedInTheRunLog(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	seedBootingMessagingAgent(t, service, fake, runID, "worker", "p-worker", "t-worker", "act-worker")

	if err := service.DeliverActivation(t.Context(), "worker", "act-worker", 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(service.messageStore().Dir, runID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), messaging.EventDeliveryExpired) {
		t.Fatalf("run log has no expiry event: %s", data)
	}
	if _, err := service.messageStore().ReadRun(runID); err != nil {
		t.Fatalf("run log no longer reconstructs: %v", err)
	}
}

func TestDeliverActivationWaitsForInteractiveReadiness(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	seedBootingMessagingAgent(t, service, fake, runID, "worker", "p-worker", "t-worker", "act-worker")
	queued, err := service.SendMessage(t.Context(), "worker", "waits for readiness")
	if err != nil {
		t.Fatal(err)
	}
	statusBeforeReady := make(chan string, 1)
	go func() {
		time.Sleep(250 * time.Millisecond)
		if message, showErr := service.ShowMessage(queued.Message.ID); showErr == nil {
			statusBeforeReady <- message.Status
		} else {
			statusBeforeReady <- showErr.Error()
		}
		fake.mu.Lock()
		for i := range fake.snapshot.Agents {
			fake.snapshot.Agents[i].InteractiveReady = true
		}
		fake.mu.Unlock()
	}()
	if err := service.DeliverActivation(t.Context(), "worker", "act-worker", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if status := <-statusBeforeReady; status != messaging.StatusQueued {
		t.Fatalf("message left the queue before the harness was ready: %s", status)
	}
	message, err := service.ShowMessage(queued.Message.ID)
	if err != nil || message.Status != messaging.StatusAwaitingAck {
		t.Fatalf("message after readiness = %#v, %v", message, err)
	}
}

func TestQueuedReplayAndUnacknowledgedActivationFailure(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	if err := service.Store.WithLocked(service.Project.Session, service.Project.Root, func(st *state.Session) error {
		st.Agents["worker"] = state.Agent{Name: "worker", PaneID: "gone", CWD: service.Project.Root}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	queued, err := service.SendMessage(t.Context(), "worker", "oldest")
	if err != nil || queued.Message.Status != messaging.StatusQueued {
		t.Fatalf("queued = %#v, %v", queued, err)
	}
	mustStartAgent(t, service, "worker")
	injected, err := service.ShowMessage(queued.Message.ID)
	if err != nil || injected.Status != messaging.StatusAwaitingAck ||
		len(injected.DeliveryAttempts) != 1 {
		t.Fatalf("injected = %#v, %v", injected, err)
	}
	if _, err := service.StopAgent(t.Context(), "worker", time.Second, false); err != nil {
		t.Fatal(err)
	}
	failed, err := service.ShowMessage(queued.Message.ID)
	if err != nil || failed.Status != messaging.StatusFailed {
		t.Fatalf("failed = %#v, %v", failed, err)
	}
	mustStartAgent(t, service, "worker")
	replayed, err := service.ShowMessage(queued.Message.ID)
	if err != nil || replayed.Status != messaging.StatusAwaitingAck ||
		len(replayed.DeliveryAttempts) != 2 {
		t.Fatalf("replayed = %#v, %v", replayed, err)
	}
}

func TestUnansweredTaskPushesCorrelatedSystemFailureWithoutRecursion(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	coordinator := mustStartAgent(t, service, "coordinator")
	seedLiveMessagingAgent(t, service, fake, runID, "worker", "p-worker", "t-worker", "act-worker")

	service.CallerPaneID = coordinator.Agent.PaneID
	sent, err := service.SendMessage(t.Context(), "worker", "complete the task")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.messages().deactivateAgent("worker", "recipient agent exited"); err != nil {
		t.Fatal(err)
	}

	original, err := service.ShowMessage(sent.Message.ID)
	if err != nil || original.Status != messaging.StatusFailed {
		t.Fatalf("original task = %#v, %v", original, err)
	}
	notification := messageBySender(t, service, runID, systemMailbox)
	if notification.Recipient != "coordinator" || notification.ReplyTo != original.ID ||
		notification.Status != messaging.StatusAwaitingAck ||
		!strings.Contains(notification.Body, original.ID) || !strings.Contains(notification.Body, `agent "worker"`) {
		t.Fatalf("system notification = %#v", notification)
	}
	fake.mu.Lock()
	promptTarget, promptText := fake.promptTarget, fake.promptText
	fake.mu.Unlock()
	if promptTarget != coordinator.Agent.PaneID || !strings.Contains(promptText, "[Fledge system message]") ||
		strings.Contains(promptText, "message reply") {
		t.Fatalf("system delivery target/text = %q / %q", promptTarget, promptText)
	}

	if _, err := service.ReplyMessage(t.Context(), notification.ID, "retry it"); Translate(err).Code != "message_state_conflict" {
		t.Fatalf("system notification reply error = %v", err)
	}
	if err := service.messages().deactivateAgent("coordinator", "recipient agent exited"); err != nil {
		t.Fatal(err)
	}
	run, err := service.messageStore().ReadRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Messages) != 2 {
		t.Fatalf("system failure recursively created notifications: %#v", run.Messages)
	}
	notification = messageBySender(t, service, runID, systemMailbox)
	if notification.Status != messaging.StatusFailed {
		t.Fatalf("undelivered system notification = %#v", notification)
	}
	acked, err := service.AckMessage(t.Context(), notification.ID)
	if err != nil || acked.Message.Status != messaging.StatusAcknowledged {
		t.Fatalf("system notification acknowledgement = %#v, %v", acked, err)
	}
}

func TestSystemFailureQueuesForInactiveSenderAndReplaysOnActivation(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	coordinator := mustStartAgent(t, service, "coordinator")
	seedLiveMessagingAgent(t, service, fake, runID, "worker", "p-worker", "t-worker", "act-worker")
	service.CallerPaneID = coordinator.Agent.PaneID
	if _, err := service.SendMessage(t.Context(), "worker", "complete the task"); err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	fake.snapshot.Agents = fake.snapshot.Agents[1:]
	fake.snapshot.Panes[0].Agent = nil
	fake.mu.Unlock()
	if err := service.messages().deactivateAgent("worker", "recipient agent exited"); err != nil {
		t.Fatal(err)
	}
	notification := messageBySender(t, service, runID, systemMailbox)
	if notification.Status != messaging.StatusQueued {
		t.Fatalf("inactive-sender notification = %#v", notification)
	}

	kind, name := "codex", "coordinator"
	fake.mu.Lock()
	fake.snapshot.Panes[0].Agent = &kind
	fake.snapshot.Panes[0].AgentStatus = StateIdle
	fake.snapshot.Agents = append(fake.snapshot.Agents, herdr.AgentInfo{
		Agent: &kind, AgentStatus: StateIdle, Name: &name,
		PaneID: coordinator.Agent.PaneID, TabID: coordinator.Agent.TabID,
		WorkspaceID: "w1", InteractiveReady: true,
	})
	fake.mu.Unlock()
	client := &herdr.Client{Socket: serviceSessionSocket(t, service.Binary)}
	if err := service.messages().activateAgent(
		t.Context(), client, "coordinator", coordinator.Agent.PaneID,
	); err != nil {
		t.Fatal(err)
	}
	notification = messageBySender(t, service, runID, systemMailbox)
	if notification.Status != messaging.StatusAwaitingAck {
		t.Fatalf("replayed system notification = %#v", notification)
	}
}

func TestRetryForceAndCancellation(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryMessage(t.Context(), sent.Message.ID, false); err == nil ||
		Translate(err).Code != "message_state_conflict" {
		t.Fatalf("retry error = %v", err)
	}
	retried, err := service.RetryMessage(t.Context(), sent.Message.ID, true)
	if err != nil || len(retried.Message.DeliveryAttempts) != 2 {
		t.Fatalf("retried = %#v, %v", retried, err)
	}
	cancelled, err := service.CancelMessage(t.Context(), sent.Message.ID, "obsolete")
	if err != nil || cancelled.Message.Status != messaging.StatusCancelled ||
		cancelled.Message.Cancellation.Reason != "obsolete" {
		t.Fatalf("cancelled = %#v, %v", cancelled, err)
	}
}

func TestPendingMessageCountsAppearInAgentAndProjectStatus(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	inbound, err := service.SendMessage(t.Context(), "worker", "pending")
	if err != nil || inbound.Message.Status != messaging.StatusAwaitingAck {
		t.Fatalf("inbound = %#v, %v", inbound, err)
	}
	agents, err := service.ListAgents(t.Context())
	if err != nil || len(agents) != 1 || agents[0].PendingMessages != 1 {
		t.Fatalf("agents = %#v, %v", agents, err)
	}
	service.CallerPaneID = "p1"
	if result, err := service.SendMessage(t.Context(), "user", "for owner"); err != nil ||
		result.Message.Status != messaging.StatusQueued {
		t.Fatalf("user message = %#v, %v", result, err)
	}
	status, err := service.Status(t.Context())
	if err != nil || status.UserPendingMessages != 1 {
		t.Fatalf("status = %#v, %v", status, err)
	}
}

func TestPreFeatureSessionRejectsMessaging(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	_, err := service.SendMessage(t.Context(), "worker", "hello")
	if err == nil || Translate(err).Code != "message_run_unavailable" {
		t.Fatalf("error = %v", err)
	}
}

func TestMessageValidationAndAuthority(t *testing.T) {
	for _, name := range []string{"", "-bad", "bad/name", "user", "fledge", "has space"} {
		if err := ValidateAgentName(name); err == nil {
			t.Errorf("accepted agent name %q", name)
		}
	}
	for _, name := range []string{"a", "A_1", "worker-name"} {
		if err := ValidateAgentName(name); err != nil {
			t.Errorf("rejected agent name %q: %v", name, err)
		}
	}
	if err := ValidateMessageBody(string([]byte{0xff})); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
	if err := ValidateMessageBody(" \n\t"); err == nil {
		t.Fatal("accepted blank body")
	}
	if err := ValidateMessageBody(string(make([]byte, MaxMessageBodyBytes+1))); err == nil {
		t.Fatal("accepted oversized body")
	}
}

// messageCommands are the four per-message commands that share the
// active-run resolution and, for two of them, the sender authorization.
type messageCommand struct {
	name       string
	verb       string
	invoke     func(t *testing.T, service *Service, messageID string) error
	authorized bool
}

func messageCommands() []messageCommand {
	return []messageCommand{
		{"reply", "replied to", func(t *testing.T, service *Service, id string) error {
			_, err := service.ReplyMessage(t.Context(), id, "answer")
			return err
		}, false},
		{"ack", "acknowledged", func(t *testing.T, service *Service, id string) error {
			_, err := service.AckMessage(t.Context(), id)
			return err
		}, false},
		{"retry", "retried", func(t *testing.T, service *Service, id string) error {
			_, err := service.RetryMessage(t.Context(), id, true)
			return err
		}, true},
		{"cancel", "cancelled", func(t *testing.T, service *Service, id string) error {
			_, err := service.CancelMessage(t.Context(), id, "obsolete")
			return err
		}, true},
	}
}

func TestArchivedMessagesRejectEveryCommandWithItsOwnVerb(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.messages().closeRun(runID, "test close"); err != nil {
		t.Fatal(err)
	}

	for _, command := range messageCommands() {
		t.Run(command.name, func(t *testing.T) {
			want := "archived messages cannot be " + command.verb
			translated := Translate(command.invoke(t, service, sent.Message.ID))
			if translated == nil || translated.Code != "message_state_conflict" || translated.Message != want {
				t.Fatalf("error = %#v, want %q", translated, want)
			}
		})
	}
}

func TestMessagesFromAnEarlierRunAreRejectedByEveryCommand(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil {
		t.Fatal(err)
	}
	// The first run stays open on disk but is no longer the session's run.
	startTestMessageRun(t, service)

	// Every command rejects identically here, so one flat loop reports all
	// four rather than four subtests asserting the same string.
	for _, command := range messageCommands() {
		translated := Translate(command.invoke(t, service, sent.Message.ID))
		if translated == nil || translated.Code != "message_state_conflict" ||
			translated.Message != "message is not part of the active run" {
			t.Errorf("%s error = %#v", command.name, translated)
		}
	}
}

func TestRetryAndCancelAreRestrictedToTheSenderOrUser(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	fromUser, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil {
		t.Fatal(err)
	}

	service.CallerPaneID = "p1"
	for _, command := range messageCommands() {
		if !command.authorized {
			continue
		}
		t.Run("denied/"+command.name, func(t *testing.T) {
			want := "only the sender or user can " + command.name + " this message"
			translated := Translate(command.invoke(t, service, fromUser.Message.ID))
			if translated == nil || translated.Code != "message_forbidden" || translated.Message != want {
				t.Fatalf("error = %#v, want %q", translated, want)
			}
		})
	}

	fromWorker, err := service.SendMessage(t.Context(), "user", "question")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RetryMessage(t.Context(), fromWorker.Message.ID, true); err != nil {
		t.Fatalf("sender was not allowed to retry its own message: %v", err)
	}
	if _, err := service.CancelMessage(t.Context(), fromWorker.Message.ID, "obsolete"); err != nil {
		t.Fatalf("sender was not allowed to cancel its own message: %v", err)
	}

	service.CallerPaneID = ""
	if _, err := service.CancelMessage(t.Context(), fromUser.Message.ID, "obsolete"); err != nil {
		t.Fatalf("user was not allowed to cancel a message it sent: %v", err)
	}

	// The user's authority is not merely "is the sender": it extends to
	// messages somebody else sent.
	service.CallerPaneID = "p1"
	fromAgent, err := service.SendMessage(t.Context(), "user", "another")
	if err != nil {
		t.Fatal(err)
	}
	service.CallerPaneID = ""
	if _, err := service.CancelMessage(t.Context(), fromAgent.Message.ID, "obsolete"); err != nil {
		t.Fatalf("user was not allowed to cancel an agent's message: %v", err)
	}
}

func TestMessageRecipientRulesRejectTheWrongCaller(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")
	sent, err := service.SendMessage(t.Context(), "worker", "hello")
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.ReplyMessage(t.Context(), sent.Message.ID, "answer")
	if translated := Translate(err); translated.Code != "message_wrong_recipient" ||
		translated.Message != "only the message recipient can reply" {
		t.Fatalf("reply error = %#v", translated)
	}
	_, err = service.AckMessage(t.Context(), sent.Message.ID)
	if translated := Translate(err); translated.Code != "message_wrong_recipient" ||
		translated.Message != "only the message recipient can acknowledge it" {
		t.Fatalf("ack error = %#v", translated)
	}
	_, err = service.SendMessage(t.Context(), "ghost", "hello")
	if translated := Translate(err); translated.Code != "message_wrong_recipient" ||
		translated.Message != `recipient "ghost" is not known in the active run` {
		t.Fatalf("unknown recipient error = %#v", translated)
	}
	service.CallerPaneID = "p1"
	_, err = service.SendMessage(t.Context(), "worker", "hello")
	if translated := Translate(err); translated.Code != "message_wrong_recipient" ||
		translated.Message != "cannot send a message to yourself" {
		t.Fatalf("self-addressed error = %#v", translated)
	}
}

func TestPrepareMessagingActivationWithoutActiveRunReturnsZeroTarget(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	mustStartAgent(t, service, "worker")

	target, err := service.messages().prepareActivation("worker", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if target != (deliveryTarget{}) {
		t.Fatalf("target without an active run = %#v", target)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Agents["worker"].ActivationID != "" {
		t.Fatalf("activation was recorded without a run: %#v", st.Agents)
	}
}

func TestPrepareMessagingActivationRecordsActivationForActiveRun(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	runID := startTestMessageRun(t, service)
	mustStartAgent(t, service, "worker")

	target, err := service.messages().prepareActivation("worker", "p1")
	if err != nil {
		t.Fatal(err)
	}
	st, err := service.Store.Read(service.Project.Session, service.Project.Root)
	if err != nil {
		t.Fatal(err)
	}
	if target.runID != runID || target.agent != "worker" || target.paneID != "p1" ||
		target.activationID == "" || target.activationID != st.Agents["worker"].ActivationID {
		t.Fatalf("target = %#v, persisted agent = %#v", target, st.Agents["worker"])
	}
}
