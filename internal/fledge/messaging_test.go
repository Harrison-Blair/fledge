package fledge

import (
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
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

func TestMessageSendInjectAckAndLinkedReply(t *testing.T) {
	service, fake := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
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
	if err := service.closeMessageRun(runID, "test close"); err != nil {
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
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	replayed, err := service.ShowMessage(queued.Message.ID)
	if err != nil || replayed.Status != messaging.StatusAwaitingAck ||
		len(replayed.DeliveryAttempts) != 2 {
		t.Fatalf("replayed = %#v, %v", replayed, err)
	}
}

func TestRetryForceAndCancellation(t *testing.T) {
	service, _ := newFakeLifecycle(t)
	startTestMessageRun(t, service)
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
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
	if _, err := service.StartAgent(t.Context(), AgentStartOptions{
		Name: "worker", Kind: "codex", Timeout: 30 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
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
	for _, name := range []string{"", "-bad", "bad/name", "user", "has space"} {
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
