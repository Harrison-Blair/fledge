package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"
)

// WorkspacesFileName is the per-session sidecar mapping managed roles to their
// persisted workspace identifiers.
const WorkspacesFileName = "workspaces.json"

var workspaceRolePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

type diskWorkspaces struct {
	SchemaVersion int               `json:"schema_version"`
	Workspaces    map[string]string `json:"workspaces"`
}

// ReadWorkspaces returns the session's persisted role-to-workspace-ID map. An
// absent sidecar is upgraded missing state and returns an empty map. Every call
// reads fresh from disk; callers serialize access under the project lock.
func ReadWorkspaces(recordPath string) (map[string]string, error) {
	if recordPath == "" {
		return nil, fmt.Errorf("read managed workspaces: record path is empty")
	}
	path := filepath.Join(recordPath, WorkspacesFileName)
	value, exists, err := readSidecar(path, decodeDiskWorkspaces)
	if err != nil {
		return nil, fmt.Errorf("read managed workspaces %q: %w", recordPath, err)
	}
	if !exists {
		return map[string]string{}, nil
	}
	return value.(map[string]string), nil
}

func decodeDiskWorkspaces(data []byte) (any, error) {
	fields, err := decodeObject(data, "workspaces", []string{"schema_version", "workspaces"})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"schema_version", "workspaces"} {
		if isJSONNull(fields[name]) {
			return nil, fmt.Errorf("workspaces %s must not be null", name)
		}
	}
	var version int
	if err := json.Unmarshal(fields["schema_version"], &version); err != nil {
		return nil, fmt.Errorf("schema_version: %w", err)
	}
	if version != schemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", version)
	}
	ids, err := decodeWorkspaceMap(fields["workspaces"])
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// decodeWorkspaceMap tokenizes the role-to-ID object so duplicate role keys are
// rejected rather than silently collapsed by a map unmarshal. Unknown but valid
// roles are preserved, so future managed roles need no schema fields.
func decodeWorkspaceMap(data []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("workspaces must be a JSON object")
	}
	ids := make(map[string]string)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		role, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("workspace role name is not a string")
		}
		if !workspaceRolePattern.MatchString(role) {
			return nil, fmt.Errorf("invalid workspace role %q", role)
		}
		if _, exists := ids[role]; exists {
			return nil, fmt.Errorf("duplicate workspace role %q", role)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if isJSONNull(value) {
			return nil, fmt.Errorf("workspace %q must not be null", role)
		}
		// Validate the raw string bytes: json.Unmarshal would silently replace
		// invalid UTF-8 with U+FFFD, admitting malformed input and breaking the
		// exact round trip. The check must precede the decode for that reason.
		if !utf8.Valid(value) {
			return nil, fmt.Errorf("workspace %q must be valid UTF-8", role)
		}
		var id string
		if err := json.Unmarshal(value, &id); err != nil {
			return nil, fmt.Errorf("workspace %q: %w", role, err)
		}
		if id == "" {
			return nil, fmt.Errorf("workspace %q must not be empty", role)
		}
		ids[role] = id
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	return ids, nil
}

// WriteWorkspaces atomically replaces the session's role-to-workspace-ID map.
// Role keys must match the managed-role pattern and identifiers must be
// non-empty; the map is validated in full before any bytes are published.
func WriteWorkspaces(recordPath string, ids map[string]string) (err error) {
	if recordPath == "" {
		return fmt.Errorf("write managed workspaces: record path is empty")
	}
	workspaces := make(map[string]string, len(ids))
	for role, id := range ids {
		if !workspaceRolePattern.MatchString(role) {
			return fmt.Errorf("write managed workspaces %q: invalid workspace role %q", recordPath, role)
		}
		if id == "" {
			return fmt.Errorf("write managed workspaces %q: workspace %q must not be empty", recordPath, role)
		}
		// A Go string may hold invalid UTF-8; json.Marshal would rewrite it to
		// U+FFFD, so reject it before publishing rather than normalize silently.
		if !utf8.ValidString(id) {
			return fmt.Errorf("write managed workspaces %q: workspace %q must be valid UTF-8", recordPath, role)
		}
		workspaces[role] = id
	}
	data, err := json.Marshal(diskWorkspaces{SchemaVersion: schemaVersion, Workspaces: workspaces})
	if err != nil {
		return fmt.Errorf("write managed workspaces %q: encode: %w", recordPath, err)
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(recordPath, ".workspaces-")
	if err != nil {
		return fmt.Errorf("write managed workspaces %q: create temporary file: %w", recordPath, err)
	}
	temporaryPath := file.Name()
	defer func() {
		if cleanupErr := os.Remove(temporaryPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove temporary managed workspaces: %w", cleanupErr))
		}
	}()
	if chmodErr := file.Chmod(0o644); chmodErr != nil {
		return fmt.Errorf("write managed workspaces %q: %w", recordPath, errors.Join(chmodErr, file.Close()))
	}
	if _, writeErr := file.Write(data); writeErr != nil {
		return fmt.Errorf("write managed workspaces %q: %w", recordPath, errors.Join(writeErr, file.Close()))
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("write managed workspaces %q: %w", recordPath, closeErr)
	}
	if err := os.Rename(temporaryPath, filepath.Join(recordPath, WorkspacesFileName)); err != nil {
		return fmt.Errorf("write managed workspaces %q: publish: %w", recordPath, err)
	}
	return nil
}
