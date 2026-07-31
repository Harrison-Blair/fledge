package fledge

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

const MaxMessageBodyBytes = 256 << 10

// userMailbox is the reserved identity for the project owner's mailbox.
const userMailbox = "user"

type MessageResult struct {
	Message       *messaging.Message `json:"message"`
	DeliveryError string             `json:"delivery_error,omitempty"`
}

type MessageHistoryOptions struct {
	Agent   string
	With    string
	RunIDs  []string
	AllRuns bool
	Status  string
	Limit   int
}

func ValidateAgentName(name string) error {
	if name == userMailbox {
		return NewError("invalid_agent_name", `"user" is reserved for the project owner mailbox`)
	}
	if len(name) == 0 || len(name) > 64 {
		return NewError("invalid_agent_name", "agent names must contain 1 to 64 characters")
	}
	for i, r := range name {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			(i > 0 && (r == '_' || r == '-')) {
			continue
		}
		return NewError("invalid_agent_name", fmt.Sprintf("invalid agent name %q", name))
	}
	return nil
}

func ValidateMessageBody(body string) error {
	if !utf8.ValidString(body) {
		return NewError("invalid_message", "message body must be valid UTF-8")
	}
	if len(body) > MaxMessageBodyBytes {
		return NewError("invalid_message", fmt.Sprintf("message body exceeds %d bytes", MaxMessageBodyBytes))
	}
	if strings.TrimSpace(body) == "" {
		return NewError("invalid_message", "message body must not be blank")
	}
	return nil
}

func (s *Service) SendMessage(ctx context.Context, recipient, body string) (MessageResult, error) {
	return s.messages().send(ctx, recipient, body)
}

func (s *Service) ReplyMessage(ctx context.Context, messageID, body string) (MessageResult, error) {
	return s.messages().reply(ctx, messageID, body)
}

func (s *Service) AckMessage(ctx context.Context, messageID string) (MessageResult, error) {
	return s.messages().ack(ctx, messageID)
}

func (s *Service) RetryMessage(ctx context.Context, messageID string, force bool) (MessageResult, error) {
	return s.messages().retry(ctx, messageID, force)
}

func (s *Service) CancelMessage(ctx context.Context, messageID, reason string) (MessageResult, error) {
	return s.messages().cancel(ctx, messageID, reason)
}

func (s *Service) ShowMessage(messageID string) (*messaging.Message, error) {
	return s.messages().show(messageID)
}

func (s *Service) MessageRuns(limit int) (messaging.RunCollection, error) {
	return s.messages().runs(limit)
}

func (s *Service) MessageHistory(opts MessageHistoryOptions) (messaging.Collection, error) {
	return s.messages().history(opts)
}

func (s *Service) MessageInbox(ctx context.Context, identity string, limit int) (messaging.Collection, error) {
	return s.messages().inbox(ctx, identity, limit)
}

// DeliverActivation runs the bounded delivery helper for an in-pane spawn.
func (s *Service) DeliverActivation(ctx context.Context, name, activationID string, timeout time.Duration) error {
	return s.messages().deliverActivation(ctx, name, activationID, timeout)
}
