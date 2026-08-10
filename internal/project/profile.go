package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

const (
	generatedProfilesDir          = "generated"
	generatedOrchestratorFilename = "orchestrator.md"
)

// DefaultOrchestratorInstructions are written for newly initialized projects.
const DefaultOrchestratorInstructions = `You are the coordinator for this Fledge project.

Spawn workers with explicit, noninteractive commands of this form:
fledge agent spawn --name <name> --harness <claude|codex|pi|opencode> --task <task>

Use --can-delegate only for workers that may delegate child work. A worker
delegating from its current assignment must supply --parent-task <task-id>.

Coordinate exclusively through Fledge's injected session messaging commands:
fledge agent message send <recipient> <text>
fledge agent message reply <message-id> <text>

Track work through fledge agent task assign/progress/blocked/needs-decision/resume/complete/fail/cancel/list/show. Task commands are durable and nonblocking; the event dispatcher wakes the right participant. Ordinary messages always wake their recipient. Stop workers only after their task is terminal. Never poll the Fledge inbox or use direct Herdr commands to inspect, prompt, or collect agent output. Never author or run sleep, shell wait, polling loops, or repeated status commands to await worker updates or task completion. After delegating, yield control; Fledge will wake you when an update requires attention. Do not start nested Fledge or Herdr sessions.`

const defaultProfileContents = "schema_version = 1\ninstructions = \"\"\"\n" + DefaultOrchestratorInstructions + "\"\"\"\n"

// OrchestratorProfile contains the editable coordinator instructions.
type OrchestratorProfile struct {
	SchemaVersion int
	Instructions  string
}

// LoadOrchestratorProfile strictly parses the orchestrator profile under root.
func LoadOrchestratorProfile(root string) (OrchestratorProfile, error) {
	return loadProfileFile(profilePath(root))
}

func profilePath(root string) string {
	return filepath.Join(root, stateDirectory, profilesDir, profileFilename)
}

// EnsureGeneratedOrchestratorPrompt writes the reusable rendered coordinator
// prompt when its contents changed and returns its absolute path. Harnesses
// resolve the returned path from their own pane working directory, so a
// project-relative reference would silently miss the file.
func EnsureGeneratedOrchestratorPrompt(root, instructions string) (string, error) {
	path := filepath.Join(root, stateDirectory, profilesDir, generatedProfilesDir, generatedOrchestratorFilename)
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, []byte(instructions)) {
		if err := os.Chmod(path, 0o600); err != nil {
			return "", fmt.Errorf("protect generated orchestrator prompt %q: %w", path, err)
		}
		return path, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read generated orchestrator prompt %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create generated profile directory: %w", err)
	}
	file, err := fsutil.OpenRegular(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("create generated orchestrator prompt %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("protect generated orchestrator prompt %q: %w", path, err)
	}
	_, writeErr := file.Write([]byte(instructions))
	closeErr := file.Close()
	if writeErr != nil {
		return "", fmt.Errorf("write generated orchestrator prompt %q: %w", path, writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close generated orchestrator prompt %q: %w", path, closeErr)
	}
	return path, nil
}

func loadProfileFile(path string) (OrchestratorProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return OrchestratorProfile{}, fmt.Errorf("read orchestrator profile %q: %w", path, err)
	}
	profile, err := parseProfile(string(data))
	if err != nil {
		return OrchestratorProfile{}, fmt.Errorf("parse orchestrator profile %q: %w", path, err)
	}
	return profile, nil
}

// parseProfile reads the two orchestrator profile keys (schema_version and
// instructions) into a struct. It is a deliberately small reader rather than a
// full TOML parser: it accepts bare-key assignments, blank lines and comments,
// and single- or triple-quoted string values taken verbatim (no escape or
// control-character grammar). Unknown keys, duplicate keys, missing keys, and
// empty instructions are rejected.
func parseProfile(input string) (OrchestratorProfile, error) {
	var profile struct {
		SchemaVersion int
		Instructions  string
	}
	seen := make(map[string]bool, 2)

	rest := input
	for {
		rest = skipSpaceAndComments(rest)
		if rest == "" {
			break
		}
		key, after := readKey(rest)
		if key == "" {
			return OrchestratorProfile{}, fmt.Errorf("expected a bare key")
		}
		if key != "schema_version" && key != "instructions" {
			return OrchestratorProfile{}, fmt.Errorf("unknown key %q", key)
		}
		if seen[key] {
			return OrchestratorProfile{}, fmt.Errorf("duplicate key %q", key)
		}
		seen[key] = true

		after = strings.TrimLeft(after, " \t")
		if !strings.HasPrefix(after, "=") {
			return OrchestratorProfile{}, fmt.Errorf("key %q is missing '='", key)
		}
		after = strings.TrimLeft(after[1:], " \t")

		var (
			value string
			err   error
		)
		if key == "schema_version" {
			value, after, err = readInteger(after)
		} else {
			value, after, err = readString(after)
		}
		if err != nil {
			return OrchestratorProfile{}, fmt.Errorf("key %q: %w", key, err)
		}
		if after, err = finishLine(after); err != nil {
			return OrchestratorProfile{}, fmt.Errorf("key %q: %w", key, err)
		}

		if key == "schema_version" {
			profile.SchemaVersion, _ = strconv.Atoi(value)
		} else {
			profile.Instructions = value
		}
		rest = after
	}

	if !seen["schema_version"] {
		return OrchestratorProfile{}, fmt.Errorf("missing key %q", "schema_version")
	}
	if profile.SchemaVersion != SchemaVersion {
		return OrchestratorProfile{}, fmt.Errorf("unsupported schema_version %d", profile.SchemaVersion)
	}
	if !seen["instructions"] {
		return OrchestratorProfile{}, fmt.Errorf("missing key %q", "instructions")
	}
	if strings.TrimSpace(profile.Instructions) == "" {
		return OrchestratorProfile{}, fmt.Errorf("instructions must not be empty")
	}
	return OrchestratorProfile{SchemaVersion: profile.SchemaVersion, Instructions: profile.Instructions}, nil
}

// skipSpaceAndComments drops leading whitespace and whole comment lines.
func skipSpaceAndComments(s string) string {
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if !strings.HasPrefix(s, "#") {
			return s
		}
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			s = s[idx+1:]
		} else {
			return ""
		}
	}
}

// readKey consumes a leading bare key (lowercase letters and underscores) and
// returns it together with the unconsumed remainder.
func readKey(s string) (string, string) {
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= 'a' && c <= 'z') || c == '_' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

// readInteger consumes a run of decimal digits.
func readInteger(s string) (string, string, error) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", s, fmt.Errorf("expected a positive integer")
	}
	return s[:i], s[i:], nil
}

// readString consumes a single- or triple-quoted string value verbatim.
func readString(s string) (string, string, error) {
	switch {
	case strings.HasPrefix(s, `"""`):
		return readDelimited(s[3:], `"""`)
	case strings.HasPrefix(s, "'''"):
		return readDelimited(s[3:], "'''")
	case strings.HasPrefix(s, `"`):
		return readInline(s[1:], '"')
	case strings.HasPrefix(s, "'"):
		return readInline(s[1:], '\'')
	default:
		return "", s, fmt.Errorf("expected a quoted string")
	}
}

// readInline reads until the closing quote on the same line.
func readInline(s string, quote byte) (string, string, error) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' || s[i] == '\r' {
			return "", s, fmt.Errorf("unterminated string")
		}
		if s[i] == quote {
			return s[:i], s[i+1:], nil
		}
	}
	return "", s, fmt.Errorf("unterminated string")
}

// readDelimited reads a multiline string up to its closing delimiter, trimming
// a single newline immediately following the opening delimiter.
func readDelimited(s, delimiter string) (string, string, error) {
	if strings.HasPrefix(s, "\r\n") {
		s = s[2:]
	} else if strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	idx := strings.Index(s, delimiter)
	if idx < 0 {
		return "", s, fmt.Errorf("unterminated multiline string")
	}
	return s[:idx], s[idx+len(delimiter):], nil
}

// finishLine rejects trailing tokens after a value and consumes the line's
// terminating newline (or an end-of-line comment).
func finishLine(s string) (string, error) {
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, "#") {
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			return s[idx+1:], nil
		}
		return "", nil
	}
	switch {
	case s == "":
		return "", nil
	case strings.HasPrefix(s, "\r\n"):
		return s[2:], nil
	case strings.HasPrefix(s, "\n"):
		return s[1:], nil
	default:
		return "", fmt.Errorf("unexpected content")
	}
}
