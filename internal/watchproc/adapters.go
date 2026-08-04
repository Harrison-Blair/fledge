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

type wakeLedger struct{ ledger *wake.Ledger }

func (l wakeLedger) Append(kind watch.WakeKind, key, reason string) (watch.WakeRecord, error) {
	record, err := l.ledger.Append(wake.Kind(kind), key, reason)
	return toWatchRecord(record), mapLedgerError(err)
}

func (l wakeLedger) Pending() ([]watch.WakeRecord, error) {
	records, err := l.ledger.Pending()
	if err != nil {
		return nil, mapLedgerError(err)
	}
	result := make([]watch.WakeRecord, 0, len(records))
	for _, record := range records {
		result = append(result, toWatchRecord(record))
	}
	return result, nil
}

func (l wakeLedger) MarkDelivered(ids []string, messageID string) error {
	return mapLedgerError(l.ledger.MarkDelivered(ids, messageID))
}

func (l wakeLedger) Compact() error { return mapLedgerError(l.ledger.Compact()) }

func (l wakeLedger) LoadMarkers() (watch.Markers, error) {
	markers, err := l.ledger.LoadMarkers()
	return watch.Markers{
		StatusSeen:      toWatchStatusSeen(markers.StatusSeen),
		EventEscalated:  markers.EventEscalated,
		DoneGrace:       markers.DoneGrace,
		KnownAgents:     markers.KnownAgents,
		LastWakeUnix:    markers.LastWakeUnix,
		HeartbeatStreak: markers.HeartbeatStreak,
	}, mapLedgerError(err)
}

func (l wakeLedger) SaveMarkers(markers watch.Markers) error {
	return mapLedgerError(l.ledger.SaveMarkers(wake.Markers{
		StatusSeen:      toWakeStatusSeen(markers.StatusSeen),
		EventEscalated:  markers.EventEscalated,
		DoneGrace:       markers.DoneGrace,
		KnownAgents:     markers.KnownAgents,
		LastWakeUnix:    markers.LastWakeUnix,
		HeartbeatStreak: markers.HeartbeatStreak,
	}))
}

func toWatchRecord(record wake.Record) watch.WakeRecord {
	return watch.WakeRecord{
		ID: record.ID, IDs: append([]string(nil), record.IDs...), Kind: watch.WakeKind(record.WakeKind),
		Key: record.Key, Reason: record.Reason,
	}
}

func toWatchStatusSeen(values map[string]wake.StatusSeen) map[string]watch.StatusSeen {
	if values == nil {
		return nil
	}
	result := make(map[string]watch.StatusSeen, len(values))
	for name, seen := range values {
		result[name] = watch.StatusSeen{Size: seen.Size, MtimeUnix: seen.MtimeUnix, Offset: seen.Offset}
	}
	return result
}

func toWakeStatusSeen(values map[string]watch.StatusSeen) map[string]wake.StatusSeen {
	if values == nil {
		return nil
	}
	result := make(map[string]wake.StatusSeen, len(values))
	for name, seen := range values {
		result[name] = wake.StatusSeen{Size: seen.Size, MtimeUnix: seen.MtimeUnix, Offset: seen.Offset}
	}
	return result
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
