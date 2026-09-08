package record

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const StopIntentFileName = "stop.json"

var stopIntentPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

type StopIntent struct {
	ID     string
	Exists bool
}

type diskStopIntent struct {
	SchemaVersion int    `json:"schema_version"`
	IntentID      string `json:"intent_id"`
}

func GenerateStopIntent(entropy io.Reader) (string, error) {
	if entropy == nil {
		return "", fmt.Errorf("stop intent entropy is nil")
	}
	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", fmt.Errorf("read stop intent entropy: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func ReadStopIntent(record Record) (StopIntent, error) {
	if record.Path == "" {
		return StopIntent{}, fmt.Errorf("read stop intent for session %q: record path is empty", record.HerdrSessionName)
	}
	path := filepath.Join(record.Path, StopIntentFileName)
	value, exists, err := readSidecar(path, decodeDiskStopIntent)
	if err != nil {
		return StopIntent{}, fmt.Errorf("read stop intent for session %q: %w", record.HerdrSessionName, err)
	}
	if !exists {
		return StopIntent{}, nil
	}
	return StopIntent{ID: value.(diskStopIntent).IntentID, Exists: true}, nil
}

func decodeDiskStopIntent(data []byte) (any, error) {
	fields, err := decodeObject(data, "stop intent", []string{"schema_version", "intent_id"})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"schema_version", "intent_id"} {
		if isJSONNull(fields[name]) {
			return nil, fmt.Errorf("stop intent %s must not be null", name)
		}
	}
	var intent diskStopIntent
	if err := json.Unmarshal(fields["schema_version"], &intent.SchemaVersion); err != nil {
		return nil, fmt.Errorf("schema_version: %w", err)
	}
	if err := json.Unmarshal(fields["intent_id"], &intent.IntentID); err != nil {
		return nil, fmt.Errorf("intent_id: %w", err)
	}
	if intent.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", intent.SchemaVersion)
	}
	if !stopIntentPattern.MatchString(intent.IntentID) {
		return nil, fmt.Errorf("intent_id must be 32 lowercase hexadecimal characters")
	}
	return intent, nil
}

func WriteStopIntent(record Record, id string) (err error) {
	if record.Path == "" {
		return fmt.Errorf("write stop intent for session %q: record path is empty", record.HerdrSessionName)
	}
	if !stopIntentPattern.MatchString(id) {
		return fmt.Errorf("write stop intent for session %q: invalid intent ID", record.HerdrSessionName)
	}
	data, err := json.Marshal(diskStopIntent{SchemaVersion: schemaVersion, IntentID: id})
	if err != nil {
		return fmt.Errorf("write stop intent for session %q: encode: %w", record.HerdrSessionName, err)
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(record.Path, ".stop-")
	if err != nil {
		return fmt.Errorf("write stop intent for session %q: create temporary file: %w", record.HerdrSessionName, err)
	}
	temporaryPath := file.Name()
	defer func() {
		if cleanupErr := os.Remove(temporaryPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary stop intent: %w", cleanupErr))
		}
	}()
	if chmodErr := file.Chmod(0o644); chmodErr != nil {
		return fmt.Errorf("write stop intent for session %q: %w", record.HerdrSessionName, errors.Join(chmodErr, file.Close()))
	}
	if _, writeErr := file.Write(data); writeErr != nil {
		return fmt.Errorf("write stop intent for session %q: %w", record.HerdrSessionName, errors.Join(writeErr, file.Close()))
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("write stop intent for session %q: %w", record.HerdrSessionName, closeErr)
	}
	if err := os.Rename(temporaryPath, filepath.Join(record.Path, StopIntentFileName)); err != nil {
		return fmt.Errorf("write stop intent for session %q: publish: %w", record.HerdrSessionName, err)
	}
	return nil
}

func RestoreStopIntent(record Record, previous StopIntent) error {
	if record.Path == "" {
		return fmt.Errorf("restore stop intent for session %q: record path is empty", record.HerdrSessionName)
	}
	if previous.Exists {
		if err := WriteStopIntent(record, previous.ID); err != nil {
			return fmt.Errorf("restore stop intent for session %q: %w", record.HerdrSessionName, err)
		}
		return nil
	}
	err := os.Remove(filepath.Join(record.Path, StopIntentFileName))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("restore stop intent for session %q: %w", record.HerdrSessionName, err)
}
