// Package ledger implements the deterministic agent handoff ledger under
// .fledge/ledger/: status, verdict and escalation records addressed by
// (subject, kind), written and read atomically with latest-value-only
// semantics — one file per (subject, kind), no history.
package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record kinds. Each (subject, kind) pair addresses exactly one file.
const (
	KindStatus     = "status"
	KindVerdict    = "verdict"
	KindEscalation = "escalation"
)

// StaleAfter is the default lease TTL: a status record that declares no
// quiet period of its own is stalled if not refreshed within this window.
const StaleAfter = 5 * time.Minute

// Record is the JSON content of one .fledge/ledger/<subject>.<kind>.json file:
// a shared envelope plus the kind-specific payload.
type Record struct {
	Subject   string          `json:"subject"`
	Kind      string          `json:"kind"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Decode unmarshals the record's payload into v, which must be a pointer to
// the payload type matching r.Kind.
func (r *Record) Decode(v any) error {
	if err := json.Unmarshal(r.Payload, v); err != nil {
		return fmt.Errorf("decoding %s payload for %s: %w", r.Kind, r.Subject, err)
	}
	return nil
}

// StatusRecord is the payload of a status record: a worker's liveness lease.
// Expect is the declared quiet period, a time.ParseDuration-compatible
// string anchored to UpdatedAt — not an absolute deadline — so the record
// stays self-describing: what was claimed, and when.
type StatusRecord struct {
	Note      string `json:"note"`
	Expect    string `json:"expect"`
	UpdatedAt string `json:"updated_at"`
}

// VerdictRecord is the payload of a verdict record: a review outcome.
type VerdictRecord struct {
	Result string `json:"result"`
	Note   string `json:"note"`
}

// EscalationRecord is the payload of an escalation record: a blocker raised
// for the orchestrator.
type EscalationRecord struct {
	Message string `json:"message"`
}

// NotFoundError reports that no record exists for a (subject, kind).
type NotFoundError struct {
	Subject string
	Kind    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no %s record for %s", e.Kind, e.Subject)
}

// CorruptError reports a record file that exists but does not parse.
type CorruptError struct {
	Subject string
	Kind    string
	Err     error
}

func (e *CorruptError) Error() string {
	return fmt.Sprintf("corrupt %s record for %s: %v", e.Kind, e.Subject, e.Err)
}

func (e *CorruptError) Unwrap() error { return e.Err }

// InvalidSubjectError reports a subject that cannot address a ledger record.
type InvalidSubjectError struct {
	Subject string
	Reason  string
}

func (e *InvalidSubjectError) Error() string {
	return fmt.Sprintf("invalid ledger subject %q: %s", e.Subject, e.Reason)
}

// validSubject enforces the address space: a record always lives at
// dir/<subject>.<kind>.json and never escapes dir. Subjects are rejected, not
// sanitized — a repaired name would silently address a different record than
// the caller asked for.
func validSubject(subject string) error {
	switch {
	case subject == "":
		return &InvalidSubjectError{subject, "must not be empty"}
	case strings.ContainsAny(subject, `/\`):
		return &InvalidSubjectError{subject, `must not contain a path separator ("/" or "\")`}
	case subject == "." || subject == "..":
		return &InvalidSubjectError{subject, "must not be a path element"}
	}
	return nil
}

func recordPath(dir, subject, kind string) string {
	return filepath.Join(dir, subject+"."+kind+".json")
}

// Write atomically writes the record for (subject, kind), replacing any
// existing one. Unlike lock.Acquire this is an overwrite, not an exclusive
// claim, so it must succeed when the target already exists: the record is
// written to a temp file in dir and moved into place with os.Rename, which
// replaces the target in one atomic step. A reader therefore never observes a
// partial, zero-length, or mixed-generation file. Returns the record written.
// An invalid subject is rejected before anything is created on disk.
func Write(dir, subject, kind string, payload any) (*Record, error) {
	if err := validSubject(subject); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	rec := Record{
		Subject:   subject,
		Kind:      kind,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload:   raw,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".fledge-tmp-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return nil, err
	}
	if err := os.Rename(tmpName, recordPath(dir, subject, kind)); err != nil {
		cleanup()
		return nil, err
	}
	return &rec, nil
}

// Read returns the current record for (subject, kind): *NotFoundError when
// none has been written (the first-appearance case), *CorruptError when the
// file exists but does not parse, *InvalidSubjectError when the subject could
// not address a record inside dir — never a panic.
func Read(dir, subject, kind string) (*Record, error) {
	if err := validSubject(subject); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(recordPath(dir, subject, kind))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &NotFoundError{Subject: subject, Kind: kind}
		}
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, &CorruptError{Subject: subject, Kind: kind, Err: err}
	}
	return &rec, nil
}

// ClassifyLiveness reports whether a worker holding a status record is
// stalled, and why. Pure: it inspects no ledger files. Liveness consults
// only lease freshness against the worker's own declared quiet period
// (expect): a worker is stalled when the present moment is past its lease's
// update timestamp plus expect. There is no PID input — a recorded PID
// cannot answer this question (see PLM-035's Context) and a permanently
// misleading field is worse than none.
func ClassifyLiveness(lastUpdated time.Time, expect time.Duration, now time.Time) (stalled bool, reason string) {
	age := now.Sub(lastUpdated)
	if age > expect {
		return true, fmt.Sprintf("lease is %s old, past its declared %s quiet period", age.Round(time.Second), expect)
	}
	return false, fmt.Sprintf("lease is %s old, within its declared %s quiet period", age.Round(time.Second), expect)
}
