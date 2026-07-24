package daemon

import (
	"os"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

const (
	validManagedAnalyzerRequest = `{"schema_version":1,"group_id":"core","purpose":"Core files","instructions_before":"Analyze group core now.","total_size":1,"files":[{"path":"core.go","size":1}],"instructions_after":"Reply once with the analyzer reply schema."}`
	bareManagedAnalyzerRequest  = `{"schema_version":1,"group_id":"core","purpose":"Core files","total_size":1,"files":[{"path":"core.go","size":1}]}`
	validManagedAnalyzerReply   = `{"schema_version":1,"status":"error","group_id":"core","errors":[{"path":"core.go","code":"read-failed","message":"could not read file"}]}`
)

func registerManagedContextPair(t *testing.T, d *Daemon) (protocol.Response, protocol.Response) {
	t.Helper()
	forager, err := d.register(&protocol.Request{
		Type: contextForagerType, Agent: contextForagerType, PID: os.Getpid(),
	})
	if err != nil {
		t.Fatal(err)
	}
	analyzer, err := d.register(&protocol.Request{
		Type: contextAnalyzerType, Agent: contextAnalyzerType, PID: os.Getpid(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return forager, analyzer
}

func TestManagedContextSendRejectsInvalidTrafficBeforeJournal(t *testing.T) {
	d := newTestDaemon(t)
	forager, analyzer := registerManagedContextPair(t, d)

	if _, err := d.send(&protocol.Request{
		From: forager.Name, To: analyzer.Name, Body: `{`,
	}); err == nil || !strings.Contains(err.Error(), "request rejected before send") {
		t.Fatalf("malformed request error = %v", err)
	}
	if len(d.messageOrder) != 0 {
		t.Fatalf("malformed request entered journal state: %v", d.messageOrder)
	}

	if _, err := d.send(&protocol.Request{
		From: forager.Name, To: analyzer.Name, Body: bareManagedAnalyzerRequest,
	}); err == nil || !strings.Contains(err.Error(), "instructions_before") {
		t.Fatalf("instruction-less request error = %v", err)
	}
	if len(d.messageOrder) != 0 {
		t.Fatalf("instruction-less request entered journal state: %v", d.messageOrder)
	}

	dispatch, err := d.send(&protocol.Request{
		From: forager.Name, To: analyzer.Name, Body: validManagedAnalyzerRequest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.inbox(&protocol.Request{As: analyzer.Name}); err != nil {
		t.Fatal(err)
	}

	for name, tt := range map[string]struct {
		request protocol.Request
		want    string
	}{
		"missing correlation": {
			request: protocol.Request{
				From: analyzer.Name, To: forager.Name, Body: validManagedAnalyzerReply,
			},
			want: "requires reply_to",
		},
		"unknown correlation": {
			request: protocol.Request{
				From: analyzer.Name, To: forager.Name, Body: validManagedAnalyzerReply,
				ReplyTo: "not-a-message",
			},
			want: "unknown message",
		},
		"malformed reply": {
			request: protocol.Request{
				From: analyzer.Name, To: forager.Name, Body: `{`,
				ReplyTo: dispatch.ID,
			},
			want: "reply rejected before send",
		},
		"mismatched reply": {
			request: protocol.Request{
				From: analyzer.Name, To: forager.Name,
				Body:    strings.Replace(validManagedAnalyzerReply, `"group_id":"core"`, `"group_id":"other"`, 1),
				ReplyTo: dispatch.ID,
			},
			want: "group_id",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := d.send(&tt.request); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
	if len(d.messageOrder) != 1 {
		t.Fatalf("rejected replies entered journal state: %v", d.messageOrder)
	}

	reply, err := d.send(&protocol.Request{
		From: analyzer.Name, To: forager.Name, Body: validManagedAnalyzerReply,
		ReplyTo: dispatch.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.messageOrder) != 2 || d.messages[reply.ID].ReplyTo != dispatch.ID {
		t.Fatalf("valid managed reply state = order %v, message %+v", d.messageOrder, d.messages[reply.ID])
	}
}

func TestManagedContextStructuredReplyValidatesBeforeJournal(t *testing.T) {
	d := newTestDaemon(t)
	forager, analyzer := registerManagedContextPair(t, d)
	dispatch, err := d.send(&protocol.Request{
		From: forager.Name, To: analyzer.Name, Body: validManagedAnalyzerRequest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.inbox(&protocol.Request{As: analyzer.Name}); err != nil {
		t.Fatal(err)
	}

	if _, err := d.reply(&protocol.Request{
		From: analyzer.Name, ID: dispatch.ID, Body: `{"schema_version":1`,
	}); err == nil || !strings.Contains(err.Error(), "reply rejected before send") {
		t.Fatalf("malformed structured reply error = %v", err)
	}
	if len(d.messageOrder) != 1 {
		t.Fatalf("malformed structured reply entered journal state: %v", d.messageOrder)
	}

	reply, err := d.reply(&protocol.Request{
		From: analyzer.Name, ID: dispatch.ID, Body: validManagedAnalyzerReply,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := d.messages[reply.ID]
	if got.To != forager.Name || got.ReplyTo != dispatch.ID {
		t.Fatalf("structured managed reply = %+v", got)
	}
}
