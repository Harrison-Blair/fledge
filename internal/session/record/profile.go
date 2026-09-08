package record

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"

	"fledge/internal/profile"
)

const (
	profileFileName      = "profile.json"
	instructionsFileName = "profile.md"
)

type diskProfile struct {
	SchemaVersion int                 `json:"schema_version"`
	Name          string              `json:"name"`
	Description   string              `json:"description"`
	Instructions  string              `json:"instructions"`
	Defaults      diskProfileDefaults `json:"defaults"`
}

type diskProfileDefaults struct {
	Harness string   `json:"harness"`
	Model   string   `json:"model"`
	Args    []string `json:"args"`
}

// ProfileInstructionsPath returns the validated file containing the session's
// pinned profile instructions. A session without a profile returns an empty
// path. The profile metadata and instruction artifact are revalidated so a
// caller never launches from inconsistent sidecars.
func ProfileInstructionsPath(record Record) (string, error) {
	configured, path, err := loadProfileSnapshot(record.Path)
	if err != nil {
		return "", fmt.Errorf("load profile instructions for session %q: %w", record.HerdrSessionName, err)
	}
	if !sameProfile(configured, record.Profile) {
		return "", fmt.Errorf("load profile instructions for session %q: record profile does not match persisted snapshot", record.HerdrSessionName)
	}
	return path, nil
}

func encodeProfileSnapshot(configured *profile.Profile) ([]byte, []byte, error) {
	if configured == nil {
		return nil, nil, nil
	}
	if configured.Defaults.Model != "" && configured.Defaults.Harness == "" {
		return nil, nil, fmt.Errorf("default model requires harness")
	}
	disk := diskProfile{
		SchemaVersion: schemaVersion,
		Name:          configured.Name,
		Description:   configured.Description,
		Instructions:  configured.Instructions,
		Defaults: diskProfileDefaults{
			Harness: configured.Defaults.Harness,
			Model:   configured.Defaults.Model,
			Args:    append([]string{}, configured.Defaults.Args...),
		},
	}
	data, err := json.Marshal(disk)
	if err != nil {
		return nil, nil, err
	}
	return append(data, '\n'), []byte(configured.Instructions), nil
}

func loadProfileSnapshot(recordDir string) (*profile.Profile, string, error) {
	value, profileExists, err := readSidecar(filepath.Join(recordDir, profileFileName), decodeDiskProfile)
	if err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", profileFileName, err)
	}
	instructionValue, instructionsExist, err := readSidecar(filepath.Join(recordDir, instructionsFileName), func(data []byte) (any, error) {
		return append([]byte(nil), data...), nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", instructionsFileName, err)
	}
	if profileExists != instructionsExist {
		return nil, "", fmt.Errorf("%s and %s must either both exist or both be absent", profileFileName, instructionsFileName)
	}
	if !profileExists {
		return nil, "", nil
	}

	configured := value.(profile.Profile)
	instructions := instructionValue.([]byte)
	if !bytes.Equal(instructions, []byte(configured.Instructions)) {
		return nil, "", fmt.Errorf("%s does not match the instructions pinned in %s", instructionsFileName, profileFileName)
	}
	return cloneProfile(&configured), filepath.Join(recordDir, instructionsFileName), nil
}

func decodeDiskProfile(data []byte) (any, error) {
	fields, err := decodeObject(data, "profile", []string{"schema_version", "name", "description", "instructions", "defaults"})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"schema_version", "name", "description", "instructions", "defaults"} {
		if isJSONNull(fields[name]) {
			return nil, fmt.Errorf("profile %s must not be null", name)
		}
	}

	var version int
	if err := json.Unmarshal(fields["schema_version"], &version); err != nil {
		return nil, fmt.Errorf("schema_version: %w", err)
	}
	if version != schemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", version)
	}
	var configured profile.Profile
	if err := json.Unmarshal(fields["name"], &configured.Name); err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}
	if err := json.Unmarshal(fields["description"], &configured.Description); err != nil {
		return nil, fmt.Errorf("description: %w", err)
	}
	if err := json.Unmarshal(fields["instructions"], &configured.Instructions); err != nil {
		return nil, fmt.Errorf("instructions: %w", err)
	}

	defaults, err := decodeProfileDefaults(fields["defaults"])
	if err != nil {
		return nil, err
	}
	configured.Defaults = defaults
	return configured, nil
}

func decodeProfileDefaults(data []byte) (profile.Defaults, error) {
	fields, err := decodeObject(data, "profile defaults", []string{"harness", "model", "args"})
	if err != nil {
		return profile.Defaults{}, err
	}
	for _, name := range []string{"harness", "model", "args"} {
		if isJSONNull(fields[name]) {
			return profile.Defaults{}, fmt.Errorf("profile defaults %s must not be null", name)
		}
	}

	var defaults profile.Defaults
	if err := json.Unmarshal(fields["harness"], &defaults.Harness); err != nil {
		return profile.Defaults{}, fmt.Errorf("profile defaults harness: %w", err)
	}
	if err := json.Unmarshal(fields["model"], &defaults.Model); err != nil {
		return profile.Defaults{}, fmt.Errorf("profile defaults model: %w", err)
	}
	defaults.Args, err = decodeStringArray(fields["args"], "profile defaults args")
	if err != nil {
		return profile.Defaults{}, err
	}
	if defaults.Model != "" && defaults.Harness == "" {
		return profile.Defaults{}, fmt.Errorf("profile default model requires harness")
	}
	return defaults, nil
}

func decodeStringArray(data []byte, field string) ([]string, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	values := make([]string, len(raw))
	for i, value := range raw {
		if isJSONNull(value) {
			return nil, fmt.Errorf("%s[%d] must not be null", field, i)
		}
		if err := json.Unmarshal(value, &values[i]); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
	}
	return values, nil
}

func cloneProfile(configured *profile.Profile) *profile.Profile {
	if configured == nil {
		return nil
	}
	cloned := *configured
	cloned.Defaults.Args = append([]string(nil), configured.Defaults.Args...)
	return &cloned
}

func sameProfile(left, right *profile.Profile) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.Name != right.Name || left.Description != right.Description || left.Instructions != right.Instructions ||
		left.Defaults.Harness != right.Defaults.Harness || left.Defaults.Model != right.Defaults.Model ||
		len(left.Defaults.Args) != len(right.Defaults.Args) {
		return false
	}
	for i := range left.Defaults.Args {
		if left.Defaults.Args[i] != right.Defaults.Args[i] {
			return false
		}
	}
	return true
}
