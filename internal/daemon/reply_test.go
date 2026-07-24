package daemon

import (
	"os"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/protocol"
)

func TestStructuredReplyDerivesClaimedInboundSenderAndCausality(t *testing.T) {
	d := newTestDaemon(t)
	sender, err := d.register(&protocol.Request{Type: "sender", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := d.register(&protocol.Request{Type: "receiver", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := d.send(&protocol.Request{
		From: sender.Name, To: receiver.Name, Body: "question",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.reply(&protocol.Request{
		From: receiver.Name, ID: inbound.ID, Body: "too early",
	}); err == nil || !strings.Contains(err.Error(), "has not been claimed") {
		t.Fatalf("unclaimed reply error = %v", err)
	}
	claimed, err := d.inbox(&protocol.Request{As: receiver.Name})
	if err != nil || claimed.Message == nil || claimed.Message.ID != inbound.ID {
		t.Fatalf("claim = %+v, %v", claimed.Message, err)
	}
	reply, err := d.reply(&protocol.Request{
		From: receiver.Name, ID: inbound.ID, Body: "answer",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.inbox(&protocol.Request{As: sender.Name})
	if err != nil {
		t.Fatal(err)
	}
	if got.Message == nil || got.Message.ID != reply.ID ||
		got.Message.From != receiver.Name || got.Message.To != sender.Name ||
		got.Message.ReplyTo != inbound.ID || got.Message.Body != "answer" {
		t.Fatalf("structured reply = %+v", got.Message)
	}
}

func TestStructuredReplyRejectsMessageInboundToAnotherIdentity(t *testing.T) {
	d := newTestDaemon(t)
	a, _ := d.register(&protocol.Request{Type: "a", PID: os.Getpid()})
	b, _ := d.register(&protocol.Request{Type: "b", PID: os.Getpid()})
	c, _ := d.register(&protocol.Request{Type: "c", PID: os.Getpid()})
	inbound, err := d.send(&protocol.Request{From: a.Name, To: b.Name, Body: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.inbox(&protocol.Request{As: b.Name}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.reply(&protocol.Request{
		From: c.Name, ID: inbound.ID, Body: "impersonation",
	}); err == nil || !strings.Contains(err.Error(), "is not inbound") {
		t.Fatalf("wrong identity reply error = %v", err)
	}
}
