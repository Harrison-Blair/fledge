package wake

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	entryQueued    = "queued"
	entryDelivered = "delivered"
)

// Kind classifies why the watcher wants to wake the orchestrator.
type Kind string

const (
	KindStatus    Kind = "status"
	KindEvent     Kind = "event"
	KindDead      Kind = "dead"
	KindHeartbeat Kind = "heartbeat"
)

// ValidKind reports whether kind is a wake kind the ledger accepts.
func ValidKind(kind Kind) bool {
	switch kind {
	case KindStatus, KindEvent, KindDead, KindHeartbeat:
		return true
	}
	return false
}

// Record is the reconstructed view of one wake that is still owed to the
// orchestrator. Repeated wakes for one kind and key collapse into a single
// Record carrying the latest reason, so IDs holds every ledger entry the
// Record speaks for, in first-seen order, ending with ID. Pass all of IDs to
// MarkDelivered: retiring ID alone leaves the entries it superseded queued,
// and they resurface on the next drain with their stale reasons.
type Record struct {
	ID       string
	IDs      []string
	WakeKind Kind
	Key      string
	Reason   string
	Time     time.Time
}

// entry is one durable line of the wake ledger. Queued entries carry the wake
// itself; delivered entries retire a previously queued ID.
type entry struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	WakeKind  Kind      `json:"wake_kind,omitempty"`
	Key       string    `json:"key,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	Time      time.Time `json:"time"`
}

func decodeEntry(data []byte, result *entry) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("empty entry")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return validateEntry(*result)
}

func validateEntry(e entry) error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("entry ID is missing")
	}
	if strings.ContainsAny(e.ID, "\r\n") {
		return errors.New("entry ID contains a line break")
	}
	if e.Time.IsZero() {
		return errors.New("entry timestamp is missing")
	}
	switch e.Kind {
	case entryQueued:
		if !ValidKind(e.WakeKind) {
			return fmt.Errorf("unknown wake kind %q", e.WakeKind)
		}
		if e.MessageID != "" {
			return errors.New("queued entry has a message ID")
		}
	case entryDelivered:
		if strings.TrimSpace(e.MessageID) == "" {
			return errors.New("delivered entry is missing its message ID")
		}
		if e.WakeKind != "" || e.Key != "" || e.Reason != "" {
			return errors.New("delivered entry has wake fields")
		}
	default:
		return fmt.Errorf("unknown entry kind %q", e.Kind)
	}
	return nil
}

// foldPending reconstructs the wakes that are still owed to the orchestrator:
// every queued entry no delivered entry retires. Survivors are grouped by wake
// kind and key — every heartbeat shares one group — so a repeated wake
// collapses to its latest reason while holding the position it was first seen
// at, and the collapsed record names every entry it speaks for.
func foldPending(entries []entry) []Record {
	delivered := make(map[string]bool)
	for _, e := range entries {
		if e.Kind == entryDelivered {
			delivered[e.ID] = true
		}
	}
	position := make(map[string]int)
	records := make([]Record, 0, len(entries))
	for _, e := range entries {
		if e.Kind != entryQueued || delivered[e.ID] {
			continue
		}
		record := Record{ID: e.ID, IDs: []string{e.ID}, WakeKind: e.WakeKind, Key: e.Key, Reason: e.Reason, Time: e.Time}
		group := groupKey(e.WakeKind, e.Key)
		if at, seen := position[group]; seen {
			record.IDs = append(records[at].IDs, e.ID)
			records[at] = record
			continue
		}
		position[group] = len(records)
		records = append(records, record)
	}
	return records
}

// retainPending returns the entries a compacted ledger keeps: every queued
// entry still owed to the orchestrator, in log order and unchanged. A retired
// queued entry and the delivered marker naming it are dropped together, so
// nothing a dropped marker was suppressing can come back and foldPending reads
// the compacted log exactly as it read the full one. A delivered marker naming
// an entry the log no longer holds is kept rather than discarded: it retires
// nothing, and dropping records nobody can interpret is not compaction's call.
func retainPending(entries []entry) []entry {
	queued := make(map[string]bool)
	delivered := make(map[string]bool)
	for _, e := range entries {
		switch e.Kind {
		case entryQueued:
			queued[e.ID] = true
		case entryDelivered:
			delivered[e.ID] = true
		}
	}
	kept := make([]entry, 0, len(entries))
	for _, e := range entries {
		switch e.Kind {
		case entryQueued:
			if !delivered[e.ID] {
				kept = append(kept, e)
			}
		case entryDelivered:
			if !queued[e.ID] {
				kept = append(kept, e)
			}
		}
	}
	return kept
}

func groupKey(kind Kind, key string) string {
	if kind == KindHeartbeat {
		return string(KindHeartbeat)
	}
	return string(kind) + "\x00" + key
}
