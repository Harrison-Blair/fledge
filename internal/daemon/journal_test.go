package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

// writeJournal drops a fixture journal into a temp dir and returns its path.
func writeJournal(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	if err := os.WriteFile(path, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReplayMissingJournal(t *testing.T) {
	s, err := replay(filepath.Join(t.TempDir(), "absent.jsonl"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(s.agents) != 0 || len(s.pending) != 0 {
		t.Fatalf("want empty state, got %d agents %d pending", len(s.agents), len(s.pending))
	}
}

func TestReplayRebuildsRoster(t *testing.T) {
	path := writeJournal(t, `{"event":"daemon.started"}
{"event":"agent.registered","name":"engineer-emperor","type":"engineer","species":"emperor","pid":101}
{"event":"agent.registered","name":"reviewer-king","type":"reviewer","species":"king","pid":102}
`)

	s, err := replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(s.agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(s.agents))
	}
	got := s.agents["engineer-emperor"]
	if got.Type != "engineer" || got.Species != "emperor" || got.PID != 101 {
		t.Fatalf("engineer-emperor replayed as %+v", got)
	}
	if want := []string{"engineer-emperor", "reviewer-king"}; len(s.order) != 2 || s.order[0] != want[0] || s.order[1] != want[1] {
		t.Fatalf("registration order = %v, want %v", s.order, want)
	}
}

func TestReplayPendingExcludesDelivered(t *testing.T) {
	path := writeJournal(t, `{"event":"agent.registered","name":"a-emperor","type":"a","species":"emperor","pid":1}
{"event":"agent.registered","name":"b-emperor","type":"b","species":"emperor","pid":2}
{"event":"msg.sent","id":"aa","from":"a-emperor","to":"b-emperor","body":"first"}
{"event":"msg.sent","id":"bb","from":"a-emperor","to":"b-emperor","body":"second","reply_to":"aa"}
{"event":"msg.delivered","id":"aa","to":"b-emperor"}
`)

	s, err := replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(s.pending) != 1 {
		t.Fatalf("want 1 pending, got %d: %+v", len(s.pending), s.pending)
	}
	m := s.pending[0]
	if m.ID != "bb" || m.Body != "second" || m.ReplyTo != "aa" {
		t.Fatalf("pending message = %+v", m)
	}
}

func TestReplayPendingKeepsSendOrder(t *testing.T) {
	path := writeJournal(t, `{"event":"agent.registered","name":"b-emperor","type":"b","species":"emperor","pid":2}
{"event":"msg.sent","id":"11","to":"b-emperor","body":"one"}
{"event":"msg.sent","id":"22","to":"b-emperor","body":"two"}
{"event":"msg.sent","id":"33","to":"b-emperor","body":"three"}
{"event":"msg.delivered","id":"22","to":"b-emperor"}
`)

	s, err := replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(s.pending) != 2 || s.pending[0].ID != "11" || s.pending[1].ID != "33" {
		t.Fatalf("pending = %+v, want ids 11 then 33", s.pending)
	}
}

func TestReplayLastRegistrationWins(t *testing.T) {
	// A dead agent's name may be reclaimed; replay must keep the newest PID
	// and must not list the name twice.
	path := writeJournal(t, `{"event":"agent.registered","name":"a-emperor","type":"a","species":"emperor","pid":1}
{"event":"agent.registered","name":"a-emperor","type":"a","species":"emperor","pid":999}
`)

	s, err := replay(path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(s.order) != 1 {
		t.Fatalf("order = %v, want one entry", s.order)
	}
	if pid := s.agents["a-emperor"].PID; pid != 999 {
		t.Fatalf("pid = %d, want 999", pid)
	}
}

func TestReplayIncompleteLaunchingAttemptIsOrphaned(t *testing.T) {
	path := writeJournal(t, `{"event":"agent.registered","name":"worker-emperor","type":"worker","species":"emperor","pid":-1}
{"event":"agent.launching","name":"worker-emperor","integration":"claude","model":"claude-opus-4","token_hash":"secret-hash","instruction_hash":"instructions"}
`)
	s, err := replay(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.agents["worker-emperor"]; got.State != stateOrphaned || got.Integration != "claude" {
		t.Fatalf("replayed incomplete launch = %+v", got)
	}
	if _, ok := s.tokens["worker-emperor"]; ok {
		t.Fatal("incomplete launch retained its readiness token")
	}
}

func TestReplayCompleteLaunchingAttemptRestoresStarting(t *testing.T) {
	path := writeJournal(t, `{"event":"agent.registered","name":"worker-emperor","type":"worker","species":"emperor","pid":-1}
{"event":"agent.launching","name":"worker-emperor","integration":"codex","model":"gpt-5.6-sol","token_hash":"secret-hash","instruction_hash":"instructions"}
{"event":"agent.spawned","name":"worker-emperor","pid":42,"pane_id":"w1:p2"}
`)
	s, err := replay(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.agents["worker-emperor"]; got.State != stateStarting || got.PID != 42 || got.PaneID != "w1:p2" || got.Integration != "codex" {
		t.Fatalf("replayed complete launch = %+v", got)
	}
	if s.tokens["worker-emperor"] != "secret-hash" {
		t.Fatalf("tokens = %v", s.tokens)
	}
}
