// Package agentprofile stores deterministic, project-local agent profiles.
package agentprofile

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// SchemaVersion is the only profile document version understood by this package.
	SchemaVersion = 1
	maxNameBytes  = 128

	// Supported harness identifiers.
	HarnessClaude   = "claude"
	HarnessCodex    = "codex"
	HarnessOpenCode = "opencode"
	HarnessPi       = "pi"

	// Supported harness-independent reasoning effort values.
	EffortLow    = "low"
	EffortMedium = "medium"
	EffortHigh   = "high"
	EffortXHigh  = "xhigh"
	EffortMax    = "max"
)

var supportedHarnesses = map[string]struct{}{
	HarnessClaude:   {},
	HarnessCodex:    {},
	HarnessOpenCode: {},
	HarnessPi:       {},
}

var supportedEfforts = map[string]struct{}{
	EffortLow:    {},
	EffortMedium: {},
	EffortHigh:   {},
	EffortXHigh:  {},
	EffortMax:    {},
}

// Profile is the persisted agent configuration. Name is derived from the
// filename, appears in JSON/API values, and is never accepted from TOML.
type Profile struct {
	Name          string   `toml:"-" json:"name"`
	SchemaVersion int      `toml:"schema_version" json:"schema_version"`
	Description   string   `toml:"description,omitempty" json:"description,omitempty"`
	Harness       string   `toml:"harness,omitempty" json:"harness,omitempty"`
	Model         string   `toml:"model,omitempty" json:"model,omitempty"`
	Effort        string   `toml:"effort,omitempty" json:"effort,omitempty"`
	NativeArgs    []string `toml:"native_args,omitempty" json:"native_args,omitempty"`
	Instructions  string   `toml:"instructions,omitempty" multiline:"true" json:"instructions,omitempty"`
}

// Validate checks every invariant required to safely persist and consume p.
func Validate(p Profile) error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if p.SchemaVersion != SchemaVersion {
		return fieldError("schema_version", "unsupported version %d (want %d)", p.SchemaVersion, SchemaVersion)
	}
	if p.Harness == "" {
		if p.Model != "" {
			return fieldError("model", "requires harness to be set")
		}
	} else {
		if p.Harness != strings.TrimSpace(p.Harness) {
			return fieldError("harness", "must not contain surrounding whitespace")
		}
		if _, ok := supportedHarnesses[p.Harness]; !ok {
			return fieldError("harness", "unsupported harness %q", p.Harness)
		}
	}
	if p.Effort != "" {
		if _, ok := supportedEfforts[p.Effort]; !ok {
			return fieldError("effort", "unsupported effort %q (want low, medium, high, xhigh, or max)", p.Effort)
		}
	}
	if err := validateText("description", p.Description); err != nil {
		return err
	}
	if err := validateText("model", p.Model); err != nil {
		return err
	}
	if err := validateText("instructions", p.Instructions); err != nil {
		return err
	}
	if p.Model != "" && strings.TrimSpace(p.Model) == "" {
		return fieldError("model", "must not be blank")
	}
	for i, arg := range p.NativeArgs {
		field := fmt.Sprintf("native_args[%d]", i)
		if arg == "" {
			return fieldError(field, "must not be empty")
		}
		if !utf8.ValidString(arg) {
			return fieldError(field, "must be valid UTF-8")
		}
		if strings.IndexByte(arg, 0) >= 0 {
			return fieldError(field, "must not contain NUL bytes")
		}
	}
	return nil
}

// ValidateName rejects names that could escape the profile directory or map
// ambiguously to hidden/path-like files.
func ValidateName(name string) error {
	if name == "" {
		return fieldError("name", "is required")
	}
	if len(name) > maxNameBytes {
		return fieldError("name", "must be at most %d bytes", maxNameBytes)
	}
	if !utf8.ValidString(name) {
		return fieldError("name", "must be valid UTF-8")
	}
	for i, r := range name {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
		if i > 0 {
			valid = valid || r == '-' || r == '_' || r == '.'
		}
		if !valid {
			return fieldError("name", "must begin with an ASCII letter or digit and contain only letters, digits, '.', '-', or '_'")
		}
	}
	return nil
}

func fieldError(field, format string, args ...any) error {
	return &ValidationError{Field: field, Reason: fmt.Sprintf(format, args...)}
}

func validateText(field, value string) error {
	if !utf8.ValidString(value) {
		return fieldError(field, "must be valid UTF-8")
	}
	if strings.IndexByte(value, 0) >= 0 {
		return fieldError(field, "must not contain NUL bytes")
	}
	return nil
}

func prepareForWrite(p Profile) (Profile, error) {
	if p.SchemaVersion == 0 {
		p.SchemaVersion = SchemaVersion
	}
	if len(p.NativeArgs) == 0 {
		p.NativeArgs = []string{}
	} else {
		p.NativeArgs = append([]string(nil), p.NativeArgs...)
	}
	if err := Validate(p); err != nil {
		return p, err
	}
	return p, nil
}
