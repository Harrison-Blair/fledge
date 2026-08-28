package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fledge/internal/session/types"
)

const schemaVersion = 1

// Record identifies a Herder session managed by this project checkout.
type Record struct {
	SchemaVersion    int
	HerdrSessionName string
	CreatedAt        time.Time
	Path             string
	Claimed          bool
	PendingChoice    *types.AgentChoice
}

type diskRecord struct {
	SchemaVersion    int    `json:"schema_version"`
	HerdrSessionName string `json:"herdr_session_name"`
	CreatedAt        string `json:"created_at"`
}

type diskClaim struct {
	SchemaVersion int `json:"schema_version"`
}

type diskPending struct {
	SchemaVersion int    `json:"schema_version"`
	Harness       string `json:"harness"`
	Model         string `json:"model"`
}

// Load reads and strictly validates every checkout-local session record. A
// project with no sessions directory has no records.
func Load(projectRoot string) ([]Record, error) {
	fledgeDir := filepath.Join(projectRoot, ".fledge")
	if err := requireDirectory(fledgeDir); err != nil {
		return nil, fmt.Errorf("load session records: %w", err)
	}

	sessionsDir := filepath.Join(fledgeDir, "sessions")
	info, err := os.Lstat(sessionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session records: inspect %q: %w", sessionsDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("load session records: %q is a symlink", sessionsDir)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("load session records: %q is not a directory", sessionsDir)
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("load session records: read %q: %w", sessionsDir, err)
	}

	records := make([]Record, 0, len(entries))
	seen := make(map[string]string, len(entries))
	var claimed string
	for _, entry := range entries {
		recordDir := filepath.Join(sessionsDir, entry.Name())
		record, err := loadRecord(recordDir, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load session records: %w", err)
		}
		if previous, exists := seen[record.HerdrSessionName]; exists {
			return nil, fmt.Errorf("load session records: duplicate Herder session name %q in %q and %q", record.HerdrSessionName, previous, record.Path)
		}
		seen[record.HerdrSessionName] = record.Path
		if record.Claimed {
			if claimed != "" {
				return nil, fmt.Errorf("load session records: multiple claimed records %q and %q", claimed, record.Path)
			}
			claimed = record.Path
		}
		records = append(records, record)
	}

	return records, nil
}

// Create generates a name and atomically publishes its local record. The
// unavailable set must contain all names reported by Herder, including stopped
// sessions. The returned record remains the caller's to keep after launch or
// stop failures.
func Create(projectRoot string, choice types.AgentChoice, maxNameLength int, unavailable map[string]struct{}, entropy io.Reader, now time.Time) (Record, error) {
	return create(projectRoot, choice, maxNameLength, unavailable, entropy, now, os.Rename)
}

func create(projectRoot string, choice types.AgentChoice, maxNameLength int, unavailable map[string]struct{}, entropy io.Reader, now time.Time, rename func(string, string) error) (Record, error) {
	if choice.Model != "" && choice.Harness == "" {
		return Record{}, fmt.Errorf("create session record: model requires harness")
	}
	if maxNameLength < MinSessionLength {
		return Record{}, fmt.Errorf("create session record: maximum length %d is too short", maxNameLength)
	}
	if maxNameLength > MaxSessionLength {
		maxNameLength = MaxSessionLength
	}
	records, err := Load(projectRoot)
	if err != nil {
		return Record{}, err
	}

	taken := make(map[string]struct{}, len(unavailable)+len(records))
	for name := range unavailable {
		taken[name] = struct{}{}
	}
	for _, record := range records {
		taken[record.HerdrSessionName] = struct{}{}
	}

	fledgeDir := filepath.Join(projectRoot, ".fledge")
	sessionsDir := filepath.Join(fledgeDir, "sessions")
	if err := ensureSessionsDirectory(sessionsDir); err != nil {
		return Record{}, fmt.Errorf("create session record: %w", err)
	}

	for {
		name, err := GenerateName(filepath.Base(filepath.Clean(projectRoot)), maxNameLength, taken, entropy)
		if err != nil {
			return Record{}, err
		}

		finalDir := filepath.Join(sessionsDir, name)
		if _, err := os.Lstat(finalDir); err == nil {
			if _, err := loadRecord(finalDir, name); err != nil {
				return Record{}, fmt.Errorf("create session record: %w", err)
			}
			taken[name] = struct{}{}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return Record{}, fmt.Errorf("create session record: inspect %q: %w", finalDir, err)
		}

		createdAt := now.UTC().Truncate(time.Second)
		disk := diskRecord{
			SchemaVersion:    schemaVersion,
			HerdrSessionName: name,
			CreatedAt:        createdAt.Format(time.RFC3339),
		}
		data, err := json.Marshal(disk)
		if err != nil {
			return Record{}, fmt.Errorf("create session record: encode config: %w", err)
		}
		data = append(data, '\n')

		claimData := []byte(`{"schema_version":1}` + "\n")
		pendingData, err := json.Marshal(diskPending{SchemaVersion: schemaVersion, Harness: choice.Harness, Model: choice.Model})
		if err != nil {
			return Record{}, fmt.Errorf("create session record: encode pending: %w", err)
		}
		pendingData = append(pendingData, '\n')

		if err := publishRecord(fledgeDir, finalDir, data, claimData, pendingData, rename); err != nil {
			if _, statErr := os.Lstat(finalDir); statErr == nil {
				if _, loadErr := loadRecord(finalDir, name); loadErr != nil {
					return Record{}, fmt.Errorf("create session record: %w", loadErr)
				}
				taken[name] = struct{}{}
				continue
			}
			return Record{}, fmt.Errorf("create session record: publish %q: %w", finalDir, err)
		}

		return Record{
			SchemaVersion:    schemaVersion,
			HerdrSessionName: name,
			CreatedAt:        createdAt,
			Path:             finalDir,
			Claimed:          true,
			PendingChoice:    &choice,
		}, nil
	}
}

func publishRecord(fledgeDir, finalDir string, config, claim, pending []byte, rename func(string, string) error) (err error) {
	temporaryDir, err := os.MkdirTemp(fledgeDir, ".session-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(temporaryDir); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temporary directory: %w", cleanupErr))
		}
	}()

	for name, data := range map[string][]byte{"config.json": config, "claim.json": claim, "pending.json": pending} {
		if err := os.WriteFile(filepath.Join(temporaryDir, name), data, 0o644); err != nil {
			return fmt.Errorf("write temporary %s: %w", name, err)
		}
	}
	if err := rename(temporaryDir, finalDir); err != nil {
		return err
	}
	return nil
}

func loadRecord(recordDir, directoryName string) (Record, error) {
	info, err := os.Lstat(recordDir)
	if err != nil {
		return Record{}, fmt.Errorf("inspect record %q: %w", recordDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Record{}, fmt.Errorf("record %q is a symlink", recordDir)
	}
	if !info.IsDir() {
		return Record{}, fmt.Errorf("record %q is not a directory", recordDir)
	}
	if !validHerderName(directoryName) {
		return Record{}, fmt.Errorf("record directory name %q is not a valid Herder session name", directoryName)
	}

	configPath := filepath.Join(recordDir, "config.json")
	configInfo, err := os.Lstat(configPath)
	if err != nil {
		return Record{}, fmt.Errorf("inspect record config %q: %w", configPath, err)
	}
	if configInfo.Mode()&os.ModeSymlink != 0 {
		return Record{}, fmt.Errorf("record config %q is a symlink", configPath)
	}
	if !configInfo.Mode().IsRegular() {
		return Record{}, fmt.Errorf("record config %q is not a regular file", configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return Record{}, fmt.Errorf("read record config %q: %w", configPath, err)
	}
	disk, err := decodeDiskRecord(data)
	if err != nil {
		return Record{}, fmt.Errorf("decode record config %q: %w", configPath, err)
	}
	if disk.SchemaVersion != schemaVersion {
		return Record{}, fmt.Errorf("record config %q has unsupported schema_version %d", configPath, disk.SchemaVersion)
	}
	if disk.HerdrSessionName != directoryName {
		return Record{}, fmt.Errorf("record config %q names session %q, want directory name %q", configPath, disk.HerdrSessionName, directoryName)
	}
	if !validHerderName(disk.HerdrSessionName) {
		return Record{}, fmt.Errorf("record config %q has invalid Herder session name %q", configPath, disk.HerdrSessionName)
	}
	createdAt, err := time.Parse(time.RFC3339, disk.CreatedAt)
	if err != nil {
		return Record{}, fmt.Errorf("record config %q has invalid created_at: %w", configPath, err)
	}
	_, offset := createdAt.Zone()
	if offset != 0 {
		return Record{}, fmt.Errorf("record config %q created_at is not UTC", configPath)
	}

	record := Record{
		SchemaVersion:    disk.SchemaVersion,
		HerdrSessionName: disk.HerdrSessionName,
		CreatedAt:        createdAt.UTC(),
		Path:             recordDir,
	}
	claim, claimExists, err := readSidecar(filepath.Join(recordDir, "claim.json"), decodeDiskClaim)
	if err != nil {
		return Record{}, fmt.Errorf("decode record claim %q: %w", recordDir, err)
	}
	if claimExists {
		record.Claimed = claim.(diskClaim).SchemaVersion == schemaVersion
	}
	pending, pendingExists, err := readSidecar(filepath.Join(recordDir, "pending.json"), decodeDiskPending)
	if err != nil {
		return Record{}, fmt.Errorf("decode record pending %q: %w", recordDir, err)
	}
	if pendingExists {
		if !record.Claimed {
			return Record{}, fmt.Errorf("record %q has pending metadata without a claim", recordDir)
		}
		choice := types.AgentChoice{Harness: pending.(diskPending).Harness, Model: pending.(diskPending).Model}
		record.PendingChoice = &choice
	}
	return record, nil
}

func decodeDiskRecord(data []byte) (diskRecord, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return diskRecord{}, err
	}
	if opening != json.Delim('{') {
		return diskRecord{}, fmt.Errorf("config must be a JSON object")
	}

	fields := make(map[string]json.RawMessage, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return diskRecord{}, err
		}
		name, ok := token.(string)
		if !ok {
			return diskRecord{}, fmt.Errorf("config field name is not a string")
		}
		if name != "schema_version" && name != "herdr_session_name" && name != "created_at" {
			return diskRecord{}, fmt.Errorf("unknown field %q", name)
		}
		if _, exists := fields[name]; exists {
			return diskRecord{}, fmt.Errorf("duplicate field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return diskRecord{}, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return diskRecord{}, err
	}
	if len(fields) != 3 {
		return diskRecord{}, fmt.Errorf("config must contain schema_version, herdr_session_name, and created_at")
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return diskRecord{}, err
	}

	var record diskRecord
	if err := json.Unmarshal(fields["schema_version"], &record.SchemaVersion); err != nil {
		return diskRecord{}, fmt.Errorf("schema_version: %w", err)
	}
	if err := json.Unmarshal(fields["herdr_session_name"], &record.HerdrSessionName); err != nil {
		return diskRecord{}, fmt.Errorf("herdr_session_name: %w", err)
	}
	if err := json.Unmarshal(fields["created_at"], &record.CreatedAt); err != nil {
		return diskRecord{}, fmt.Errorf("created_at: %w", err)
	}
	if isJSONNull(fields["schema_version"]) || isJSONNull(fields["herdr_session_name"]) || isJSONNull(fields["created_at"]) {
		return diskRecord{}, fmt.Errorf("config fields must not be null")
	}
	return record, nil
}

// Claim permanently selects a historical record. It never replaces a claim
// written by another starter.
func Claim(record Record) error {
	return claim(record, os.Link)
}

func claim(record Record, link func(string, string) error) (err error) {
	path := filepath.Join(record.Path, "claim.json")
	file, err := os.CreateTemp(record.Path, ".claim-")
	if err != nil {
		return fmt.Errorf("claim session record %q: %w", record.HerdrSessionName, err)
	}
	temporaryPath := file.Name()
	defer func() {
		if cleanupErr := os.Remove(temporaryPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary claim %q: %w", record.HerdrSessionName, cleanupErr))
		}
	}()

	if err := file.Chmod(0o644); err != nil {
		closeErr := file.Close()
		return fmt.Errorf("claim session record %q: %w", record.HerdrSessionName, errors.Join(err, closeErr))
	}
	if _, writeErr := file.Write([]byte(`{"schema_version":1}` + "\n")); writeErr != nil {
		closeErr := file.Close()
		return fmt.Errorf("claim session record %q: %w", record.HerdrSessionName, errors.Join(writeErr, closeErr))
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("claim session record %q: %w", record.HerdrSessionName, closeErr)
	}
	if err := link(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("claim session record %q: already claimed", record.HerdrSessionName)
		}
		return fmt.Errorf("claim session record %q: publish claim: %w", record.HerdrSessionName, err)
	}
	return nil
}

// ClearPending marks bootstrap metadata as consumed. A concurrent watcher may
// have already removed it, which is success.
func ClearPending(record Record) error {
	err := os.Remove(filepath.Join(record.Path, "pending.json"))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("clear pending session metadata %q: %w", record.HerdrSessionName, err)
}

// Unclaim discards a record's claim so a later start creates a fresh session.
// Pending metadata is removed first because a record with pending metadata and
// no claim fails to load.
func Unclaim(record Record) error {
	if err := ClearPending(record); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(record.Path, "claim.json"))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("unclaim session record %q: %w", record.HerdrSessionName, err)
}

func readSidecar(path string, decode func([]byte) (any, error)) (any, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("%q is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("%q is not a regular file", path)
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && filepath.Base(path) == "pending.json" {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	value, err := decode(data)
	return value, true, err
}

func decodeDiskClaim(data []byte) (any, error) {
	fields, err := decodeObject(data, "claim", []string{"schema_version"})
	if err != nil {
		return nil, err
	}
	if isJSONNull(fields["schema_version"]) {
		return nil, fmt.Errorf("claim schema_version must not be null")
	}
	var claim diskClaim
	if err := json.Unmarshal(fields["schema_version"], &claim.SchemaVersion); err != nil {
		return nil, fmt.Errorf("schema_version: %w", err)
	}
	if claim.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", claim.SchemaVersion)
	}
	return claim, nil
}

func decodeDiskPending(data []byte) (any, error) {
	fields, err := decodeObject(data, "pending", []string{"schema_version", "harness", "model"})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"schema_version", "harness", "model"} {
		if isJSONNull(fields[name]) {
			return nil, fmt.Errorf("pending %s must not be null", name)
		}
	}
	var pending diskPending
	if err := json.Unmarshal(fields["schema_version"], &pending.SchemaVersion); err != nil {
		return nil, fmt.Errorf("schema_version: %w", err)
	}
	if err := json.Unmarshal(fields["harness"], &pending.Harness); err != nil {
		return nil, fmt.Errorf("harness: %w", err)
	}
	if err := json.Unmarshal(fields["model"], &pending.Model); err != nil {
		return nil, fmt.Errorf("model: %w", err)
	}
	if pending.SchemaVersion != schemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", pending.SchemaVersion)
	}
	if pending.Model != "" && pending.Harness == "" {
		return nil, fmt.Errorf("model requires harness")
	}
	return pending, nil
}

func decodeObject(data []byte, kind string, allowed []string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be a JSON object", kind)
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("%s field name is not a string", kind)
		}
		if _, ok := allowedSet[name]; !ok {
			return nil, fmt.Errorf("unknown field %q", name)
		}
		if _, ok := fields[name]; ok {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if len(fields) != len(allowed) {
		return nil, fmt.Errorf("%s must contain %s", kind, joinFields(allowed))
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}
	return fields, nil
}

func joinFields(fields []string) string {
	if len(fields) == 1 {
		return fields[0]
	}
	return strings.Join(fields[:len(fields)-1], ", ") + ", and " + fields[len(fields)-1]
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unexpected trailing JSON value")
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", path)
	}
	return nil
}

func ensureSessionsDirectory(path string) error {
	if err := os.Mkdir(path, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create sessions directory %q: %w", path, err)
	}
	return requireDirectory(path)
}
