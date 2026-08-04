// Package messaging stores the audit trail for messages in the active Fledge
// session. The log lives in the session's log folder and is append-only
// between Initialize and RemoveAll calls.
package messaging

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const (
	logFilename  = "messages.jsonl"
	lockFilename = "messages.lock"
	eventVersion = 1
)

// MaxBodyBytes is the largest permitted UTF-8 message body.
const MaxBodyBytes = 64 * 1024

var (
	ErrNotInitialized  = errors.New("messaging session is not initialized")
	ErrNotFound        = errors.New("message not found")
	ErrUnauthorized    = errors.New("message operation is not authorized")
	ErrInvalidBody     = errors.New("invalid message body")
	ErrCorruptLog      = errors.New("corrupt messaging log")
	ErrSessionMismatch = errors.New("messaging log belongs to a different session")
)

// Status is the reconstructed state of a message.
type Status string

const (
	StatusPending   Status = "pending"
	StatusUncertain Status = "uncertain"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Message is the stable, reconstructed view of one message.
type Message struct {
	ID            string
	Sender        string
	Recipient     string
	ReplyTo       string
	Body          string
	Status        Status
	RecipientPane string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	AttemptedAt   time.Time
	DeliveredAt   time.Time
	Failure       string
}

// CreateParams describes a new outbound message. RecipientPane binds the
// message to the current recipient process; it must be empty for recipient
// "user" and non-empty for every other recipient.
type CreateParams struct {
	Sender        string
	Recipient     string
	Body          string
	RecipientPane string
}

// Option customizes a Store, primarily to make IDs and timestamps testable.
type Option func(*Store)

// WithClock replaces the wall clock used for new events.
func WithClock(clock func() time.Time) Option {
	return func(s *Store) { s.clock = clock }
}

// WithIDGenerator replaces the cryptographically random ID generator.
func WithIDGenerator(generator func() (string, error)) Option {
	return func(s *Store) { s.generateID = generator }
}

// Store owns one Herdr session's message log beneath root/.fledge/logs.
type Store struct {
	root       string
	session    string
	clock      func() time.Time
	generateID func() (string, error)
}

// New constructs a Store for the named Herdr session. It does not create or
// modify any files; an unusable session name is reported by the first
// operation that touches the filesystem.
func New(root, session string, options ...Option) *Store {
	s := &Store{root: root, session: session, clock: time.Now, generateID: randomID}
	for _, option := range options {
		option(s)
	}
	return s
}

// ValidateBody checks the CLI-level body contract: UTF-8, nonblank, and no
// larger than 64 KiB when encoded.
func ValidateBody(body string) error {
	if !utf8.ValidString(body) {
		return fmt.Errorf("%w: body is not valid UTF-8", ErrInvalidBody)
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("%w: body must not be blank", ErrInvalidBody)
	}
	if len(body) > MaxBodyBytes {
		return fmt.Errorf("%w: body exceeds 64 KiB", ErrInvalidBody)
	}
	return nil
}

// Initialize replaces any prior message log with a fresh session and returns
// its durable session ID. Call this only when creating a new Herdr server, not
// when reattaching to an existing one.
func (s *Store) Initialize() (string, error) {
	var sessionID string
	err := s.withLock(func() error {
		id, err := s.newID()
		if err != nil {
			return err
		}
		sessionID = id
		e := event{Version: eventVersion, Type: eventSessionStart, At: s.now(), SessionID: id, Session: s.session}
		return s.replaceLog([]event{e})
	})
	if err != nil {
		return "", err
	}
	return sessionID, s.removeLegacyFiles()
}

// Ensure validates that an existing log belongs to the store's session. If the
// log is absent or empty (as with sessions created before messaging support),
// Ensure initializes it without disturbing a non-empty valid log.
func (s *Store) Ensure() (string, error) {
	var sessionID string
	err := s.withLock(func() error {
		state, err := s.loadState()
		if errors.Is(err, ErrNotInitialized) {
			id, idErr := s.newID()
			if idErr != nil {
				return idErr
			}
			e := event{Version: eventVersion, Type: eventSessionStart, At: s.now(), SessionID: id, Session: s.session}
			if err := s.replaceLog([]event{e}); err != nil {
				return err
			}
			sessionID = id
			return nil
		}
		if err != nil {
			return err
		}
		if state.session != s.session {
			return fmt.Errorf("%w: log has %q, active session is %q", ErrSessionMismatch, state.session, s.session)
		}
		sessionID = state.sessionID
		return nil
	})
	return sessionID, err
}

// RemoveLock deletes the session's lock file and keeps its log. It is intended
// for use after successful session deletion or confirmed stale-session
// cleanup.
func (s *Store) RemoveLock() error {
	if err := s.ensureStateDirectory(); err != nil {
		return err
	}
	unlock, err := s.acquireLock()
	if err != nil {
		return err
	}
	if err := unlock(); err != nil {
		return err
	}
	lockPath := s.lockPath()
	if err := rejectSymlink(lockPath); err != nil {
		return err
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove messaging lock %q: %w", lockPath, err)
	}
	return syncDirectory(s.statePath())
}

// RemoveAll deletes the session's whole log folder. It is intended for rolling
// back a session that never became usable.
func (s *Store) RemoveAll() error {
	if err := s.ensureStateDirectory(); err != nil {
		return err
	}
	unlock, err := s.acquireLock()
	if err != nil {
		return err
	}
	if err := unlock(); err != nil {
		return err
	}
	path := s.statePath()
	if err := rejectSymlink(path); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove messaging session directory %q: %w", path, err)
	}
	return syncDirectory(statedir.Logs(s.root))
}

// removeLegacyFiles deletes the pre-session-folder message log and lock that
// older Fledge versions kept directly in .fledge.
func (s *Store) removeLegacyFiles() error {
	directory := statedir.Root(s.root)
	var removeErr error
	for _, name := range []string{logFilename, lockFilename} {
		path := filepath.Join(directory, name)
		if err := rejectSymlink(path); err != nil {
			removeErr = errors.Join(removeErr, err)
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErr = errors.Join(removeErr, fmt.Errorf("remove legacy messaging file %q: %w", path, err))
		}
	}
	return errors.Join(removeErr, syncDirectory(directory))
}

// Create appends a new pending message.
func (s *Store) Create(params CreateParams) (Message, error) {
	if err := validateCreate(params); err != nil {
		return Message{}, err
	}
	var created Message
	err := s.withState(func(state *logState) error {
		id, err := s.uniqueID(state)
		if err != nil {
			return err
		}
		e := event{
			Version: eventVersion, Type: eventMessageCreated, At: s.now(), SessionID: state.sessionID,
			MessageID: id, Sender: params.Sender, Recipient: params.Recipient,
			Body: params.Body, RecipientPane: params.RecipientPane,
		}
		if err := s.appendEvents([]event{e}); err != nil {
			return err
		}
		if err := applyEvent(state, e); err != nil {
			return err
		}
		created = state.messages[id]
		return nil
	})
	return created, err
}

// RecordAttempt persists intent to submit a message before the Herdr call.
// After this event, an absent outcome reconstructs as uncertain.
func (s *Store) RecordAttempt(messageID string) (Message, error) {
	return s.transition(messageID, eventDeliveryAttempt, false, "")
}

// RecordDelivery persists the reported Herdr outcome. accepted=true becomes
// delivered; accepted=false becomes failed and is never retried by storage.
func (s *Store) RecordDelivery(messageID string, accepted bool, detail string) (Message, error) {
	return s.transition(messageID, eventDeliveryOutcome, accepted, detail)
}

// Reply creates a correlated reply without changing the original message.
// replier and replierPane must identify the original recipient. The reply is
// bound to replyRecipientPane. Replies to "user" are locally delivered.
func (s *Store) Reply(originalID, replier, replierPane, body, replyRecipientPane string) (Message, error) {
	if err := ValidateBody(body); err != nil {
		return Message{}, err
	}
	var reply Message
	err := s.withState(func(state *logState) error {
		original, ok := state.messages[originalID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, originalID)
		}
		if err := authorize(original, replier, replierPane); err != nil {
			return err
		}
		if original.Status != StatusDelivered && original.Status != StatusUncertain {
			return fmt.Errorf("cannot reply to message %s in status %s", originalID, original.Status)
		}
		if original.Sender == "user" {
			if replyRecipientPane != "" {
				return errors.New("user recipient must not have a pane")
			}
		} else if replyRecipientPane == "" {
			return errors.New("agent recipient must have a pane")
		}
		id, err := s.uniqueID(state)
		if err != nil {
			return err
		}
		at := s.now()
		e := event{Version: eventVersion, Type: eventReplyCreated, At: at, SessionID: state.sessionID, MessageID: id, Sender: replier, Recipient: original.Sender, ReplyTo: originalID, Body: body, RecipientPane: replyRecipientPane}
		if err := s.appendEvents([]event{e}); err != nil {
			return err
		}
		if err := applyEvent(state, e); err != nil {
			return err
		}
		reply = state.messages[id]
		return nil
	})
	return reply, err
}

// Get returns one reconstructed message.
func (s *Store) Get(messageID string) (Message, error) {
	var result Message
	err := s.withState(func(state *logState) error {
		message, ok := state.messages[messageID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, messageID)
		}
		result = message
		return nil
	})
	return result, err
}

// List returns all messages in creation order.
func (s *Store) List() ([]Message, error) {
	var result []Message
	err := s.withState(func(state *logState) error {
		result = make([]Message, 0, len(state.order))
		for _, id := range state.order {
			result = append(result, state.messages[id])
		}
		return nil
	})
	return result, err
}

// Inbox returns the selected identity's complete transcript in creation order.
func (s *Store) Inbox(identity string) ([]Message, error) {
	var result []Message
	err := s.withState(func(state *logState) error {
		for _, id := range state.order {
			message := state.messages[id]
			if message.Sender == identity || message.Recipient == identity {
				result = append(result, message)
			}
		}
		return nil
	})
	return result, err
}

func (s *Store) transition(messageID, kind string, accepted bool, detail string) (Message, error) {
	var result Message
	err := s.withState(func(state *logState) error {
		message, ok := state.messages[messageID]
		if !ok {
			return fmt.Errorf("%w: %s", ErrNotFound, messageID)
		}
		if kind == eventDeliveryAttempt && message.Status != StatusPending {
			return fmt.Errorf("cannot attempt message %s in status %s", messageID, message.Status)
		}
		if kind == eventDeliveryOutcome && message.Status != StatusUncertain {
			return fmt.Errorf("cannot record outcome for message %s in status %s", messageID, message.Status)
		}
		e := event{Version: eventVersion, Type: kind, At: s.now(), SessionID: state.sessionID, MessageID: messageID, Detail: detail}
		if kind == eventDeliveryOutcome {
			e.Accepted = boolPointer(accepted)
		}
		if err := s.appendEvents([]event{e}); err != nil {
			return err
		}
		if err := applyEvent(state, e); err != nil {
			return err
		}
		result = state.messages[messageID]
		return nil
	})
	return result, err
}

func (s *Store) withState(operation func(*logState) error) error {
	return s.withLock(func() error {
		state, err := s.loadState()
		if err != nil {
			return err
		}
		return operation(state)
	})
}

func (s *Store) withLock(operation func() error) error {
	if err := s.ensureStateDirectory(); err != nil {
		return err
	}
	unlock, err := s.acquireLock()
	if err != nil {
		return err
	}
	operationErr := operation()
	return errors.Join(operationErr, unlock())
}

func (s *Store) ensureStateDirectory() error {
	if !statedir.ValidSessionDirName(s.session) {
		return fmt.Errorf("Herdr session name %q is not a valid messaging log directory name", s.session)
	}
	for _, path := range []string{statedir.Root(s.root), statedir.Logs(s.root), s.statePath()} {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.MkdirAll(path, 0o700); err != nil {
				return fmt.Errorf("create messaging state directory %q: %w", path, err)
			}
		case err != nil:
			return fmt.Errorf("inspect messaging state directory %q: %w", path, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("messaging state directory %q must not be a symlink", path)
		case !info.IsDir():
			return fmt.Errorf("messaging state path %q is not a directory", path)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure messaging state directory %q: %w", path, err)
		}
	}
	return nil
}

func (s *Store) loadState() (*logState, error) {
	data, err := s.readAndRepairLog()
	if err != nil {
		return nil, err
	}
	state := &logState{messages: make(map[string]Message)}
	lines := bytes.Split(data, []byte{'\n'})
	for index, line := range lines[:len(lines)-1] {
		var e event
		if err := decodeEvent(line, &e); err != nil {
			return nil, fmt.Errorf("%w at line %d: %v", ErrCorruptLog, index+1, err)
		}
		if err := applyEvent(state, e); err != nil {
			return nil, fmt.Errorf("%w at line %d: %v", ErrCorruptLog, index+1, err)
		}
	}
	if state.sessionID == "" {
		return nil, ErrNotInitialized
	}
	return state, nil
}

func (s *Store) readAndRepairLog() ([]byte, error) {
	path := s.logPath()
	if err := rejectSymlink(path); err != nil {
		return nil, err
	}
	file, err := openRegular(path, os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotInitialized
	}
	if err != nil {
		return nil, fmt.Errorf("open messaging log %q: %w", path, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure messaging log %q: %w", path, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read messaging log %q: %w", path, err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lastNewline := bytes.LastIndexByte(data, '\n')
		completeLength := int64(lastNewline + 1)
		if err := file.Truncate(completeLength); err != nil {
			return nil, fmt.Errorf("repair messaging log %q: %w", path, err)
		}
		if err := file.Sync(); err != nil {
			return nil, fmt.Errorf("sync repaired messaging log %q: %w", path, err)
		}
		data = data[:completeLength]
	}
	return data, nil
}

func (s *Store) replaceLog(events []event) error {
	if err := validateEvents(events); err != nil {
		return err
	}
	path := s.logPath()
	if err := rejectSymlink(path); err != nil {
		return err
	}
	file, err := openRegular(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create messaging log %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure messaging log %q: %w", path, err)
	}
	// Truncate only after openRegular has verified that the opened handle is the
	// same regular, non-symlink file currently named by path.
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return fmt.Errorf("truncate messaging log %q: %w", path, err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		return fmt.Errorf("seek messaging log %q: %w", path, err)
	}
	err = writeEvents(file, events)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("close messaging log %q: %w", path, closeErr)
	}
	return syncDirectory(s.statePath())
}

func (s *Store) appendEvents(events []event) error {
	if err := validateEvents(events); err != nil {
		return err
	}
	path := s.logPath()
	if err := rejectSymlink(path); err != nil {
		return err
	}
	file, err := openRegular(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open messaging log %q: %w", path, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure messaging log %q: %w", path, err)
	}
	return writeEvents(file, events)
}

func writeEvents(file *os.File, events []event) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, e := range events {
		if err := validateEvent(e); err != nil {
			return fmt.Errorf("validate messaging event: %w", err)
		}
		if err := encoder.Encode(e); err != nil {
			return fmt.Errorf("encode messaging event: %w", err)
		}
	}
	data := buffer.Bytes()
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return fmt.Errorf("append messaging events: %w", err)
		}
		data = data[written:]
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync messaging events: %w", err)
	}
	return nil
}

func validateEvents(events []event) error {
	for _, e := range events {
		if err := validateEvent(e); err != nil {
			return fmt.Errorf("validate messaging event: %w", err)
		}
	}
	return nil
}

func openRegular(path string, flags int, permission os.FileMode) (*os.File, error) {
	file, err := openFileNoFollow(path, flags, permission)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("path %q is not a regular file", path)
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, current) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("path %q changed while opening or is a symlink", path)
	}
	return file, nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect messaging path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("messaging path %q must not be a symlink", path)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open messaging state directory %q: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync messaging state directory %q: %w", path, err)
	}
	return nil
}

func validateCreate(params CreateParams) error {
	if strings.TrimSpace(params.Sender) == "" {
		return errors.New("message sender must not be blank")
	}
	if strings.TrimSpace(params.Recipient) == "" {
		return errors.New("message recipient must not be blank")
	}
	if params.Sender == params.Recipient {
		return errors.New("message sender and recipient must differ")
	}
	if params.Recipient == "user" && params.RecipientPane != "" {
		return errors.New("user recipient must not have a pane")
	}
	if params.Recipient != "user" && params.RecipientPane == "" {
		return errors.New("agent recipient must have a pane")
	}
	return ValidateBody(params.Body)
}

func authorize(message Message, recipient, pane string) error {
	if message.Recipient != recipient || message.RecipientPane != pane {
		return fmt.Errorf("%w for message %s", ErrUnauthorized, message.ID)
	}
	return nil
}

func (s *Store) uniqueID(state *logState) (string, error) {
	for attempts := 0; attempts < 100; attempts++ {
		id, err := s.newID()
		if err != nil {
			return "", err
		}
		if _, exists := state.messages[id]; !exists && id != state.sessionID {
			return id, nil
		}
	}
	return "", errors.New("message ID generator repeatedly returned duplicate IDs")
}

func (s *Store) newID() (string, error) {
	id, err := s.generateID()
	if err != nil {
		return "", fmt.Errorf("generate messaging ID: %w", err)
	}
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n") {
		return "", errors.New("message ID generator returned an invalid ID")
	}
	return id, nil
}

func (s *Store) now() time.Time { return s.clock().UTC() }

func (s *Store) statePath() string { return statedir.Session(s.root, s.session) }
func (s *Store) logPath() string   { return filepath.Join(s.statePath(), logFilename) }
func (s *Store) lockPath() string  { return filepath.Join(s.statePath(), lockFilename) }

func randomID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func boolPointer(value bool) *bool { return &value }
