package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Sender attributes a delivered prompt to the calling pane.
// Kind is named, unnamed, pane (not an agent), or unknown; Error records why
// attribution failed without failing the delivery.
type Sender struct {
	Name  *string `json:"name"`
	Pane  *string `json:"pane"`
	Kind  string  `json:"kind"`
	Error *string `json:"error"`
}

// NewMessageID returns a random correlation ID of the form m-<6 lowercase hex>.
func NewMessageID() string { return messageID(rand.Reader) }

// messageID draws the ID's three random bytes from r.
func messageID(r io.Reader) string {
	b := make([]byte, 3)
	io.ReadFull(r, b)
	return "m-" + hex.EncodeToString(b)
}

// ResolveSender identifies the caller's pane through agent.get.
func ResolveSender(ctx context.Context, c Client) Sender {
	if c.CallerPane == "" {
		return CallerSender("", nil)
	}
	a, err := c.Get(ctx, c.CallerPane)
	var remote *herdr.Error
	switch {
	case err == nil:
		return CallerSender(c.CallerPane, &a)
	case errors.As(err, &remote) && remote.Code == "agent_not_found":
		return CallerSender(c.CallerPane, nil)
	}
	msg := err.Error()
	return Sender{Pane: Pointer(c.CallerPane), Kind: "unknown", Error: &msg}
}

// CallerSender is ResolveSender from a finished lookup of the caller's pane:
// caller is the agent there, or nil when the pane hosts none. An empty pane is
// a caller outside Herdr.
func CallerSender(pane string, caller *herdr.AgentDetails) Sender {
	switch {
	case pane == "":
		msg := "HERDR_PANE_ID is not set"
		return Sender{Kind: "unknown", Error: &msg}
	case caller == nil:
		return Sender{Pane: Pointer(pane), Kind: "pane"}
	case caller.Pane.Name != nil && *caller.Pane.Name != "":
		return Sender{Name: caller.Pane.Name, Pane: Pointer(caller.Pane.PaneID), Kind: "named"}
	}
	return Sender{Pane: Pointer(caller.Pane.PaneID), Kind: "unnamed"}
}

// String describes the sender as it appears in headers and human output.
func (s Sender) String() string {
	switch s.Kind {
	case "named":
		return fmt.Sprintf("%s (%s)", *s.Name, *s.Pane)
	case "unnamed":
		return fmt.Sprintf("unnamed agent (%s)", *s.Pane)
	case "pane":
		return "pane " + *s.Pane
	}
	return "unknown sender"
}

// WithHeader prefixes body with the one-line sender header; only named
// senders get a reply command.
func WithHeader(id string, s Sender, body string) string {
	header := fmt.Sprintf("ᛉ fledge message from %s · id %s", s, id)
	if s.Kind == "named" {
		header += " · reply: fledge agent message --name " + *s.Name
	}
	return header + "\n" + body
}
