package watchproc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/wake"
	"github.com/Harrison-Blair/fledge/internal/watch"
)

// wakeLedger adapts the durable ledger to the engine's Ledger interface. The
// two speak the same types, so the only translation left is the corruption
// sentinel the engine degrades on.
type wakeLedger struct{ ledger *wake.Ledger }

func (l wakeLedger) Append(kind watch.WakeKind, key, reason string) (watch.WakeRecord, error) {
	record, err := l.ledger.Append(kind, key, reason)
	return record, mapLedgerError(err)
}

func (l wakeLedger) Pending() ([]watch.WakeRecord, error) {
	records, err := l.ledger.Pending()
	return records, mapLedgerError(err)
}

func (l wakeLedger) MarkDelivered(ids []string, messageID string) error {
	return mapLedgerError(l.ledger.MarkDelivered(ids, messageID))
}

func (l wakeLedger) Compact() error { return mapLedgerError(l.ledger.Compact()) }

func (l wakeLedger) LoadMarkers() (watch.Markers, error) {
	markers, err := l.ledger.LoadMarkers()
	return markers, mapLedgerError(err)
}

func (l wakeLedger) SaveMarkers(markers watch.Markers) error {
	return mapLedgerError(l.ledger.SaveMarkers(markers))
}

func mapLedgerError(err error) error {
	if errors.Is(err, wake.ErrCorruptLog) {
		return fmt.Errorf("%w: %v", watch.ErrCorruptLog, err)
	}
	return err
}

type deliveryWaker DeliverFunc

func (w deliveryWaker) Deliver(ctx context.Context, body string) (string, error) {
	return DeliverFunc(w)(ctx, body)
}

type messageLister interface {
	List() ([]messaging.Message, error)
}

type completionAudit struct{ store messageLister }

func (a completionAudit) CompletionSince(worker string, since time.Time) (bool, error) {
	messages, err := a.store.List()
	if err != nil {
		return false, err
	}
	for _, message := range messages {
		if message.Sender == worker && message.Recipient == "orchestrator" && !message.CreatedAt.Before(since) {
			return true, nil
		}
	}
	return false, nil
}
