package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const schemaVersion = 1

// Record identifies a Herder session managed by this project checkout.
type Record struct {
	SchemaVersion    int
	HerdrSessionName string
	CreatedAt        time.Time
	Path             string
}

type diskRecord struct {
	SchemaVersion    int    `json:"schema_version"`
	HerdrSessionName string `json:"herdr_session_name"`
	CreatedAt        string `json:"created_at"`
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
		records = append(records, record)
	}

	return records, nil
}

// Create generates a name and atomically publishes its local record. The
// unavailable set must contain all names reported by Herder, including stopped
// sessions. The returned record remains the caller's to keep after launch or
// stop failures.
func Create(projectRoot string, unavailable map[string]struct{}, entropy io.Reader, now time.Time) (Record, error) {
	return create(projectRoot, unavailable, entropy, now, os.Rename)
}

func create(projectRoot string, unavailable map[string]struct{}, entropy io.Reader, now time.Time, rename func(string, string) error) (Record, error) {
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
		name, err := GenerateName(filepath.Base(filepath.Clean(projectRoot)), taken, entropy)
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

		if err := publishRecord(fledgeDir, finalDir, data, rename); err != nil {
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
		}, nil
	}
}

func publishRecord(fledgeDir, finalDir string, data []byte, rename func(string, string) error) (err error) {
	temporaryDir, err := os.MkdirTemp(fledgeDir, ".session-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(temporaryDir); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temporary directory: %w", cleanupErr))
		}
	}()

	temporaryConfig := filepath.Join(temporaryDir, "config.json")
	if err := os.WriteFile(temporaryConfig, data, 0o644); err != nil {
		return fmt.Errorf("write temporary config: %w", err)
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

	return Record{
		SchemaVersion:    disk.SchemaVersion,
		HerdrSessionName: disk.HerdrSessionName,
		CreatedAt:        createdAt.UTC(),
		Path:             recordDir,
	}, nil
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
	return record, nil
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
