package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Harrison-Blair/fledge/internal/fsutil"
)

const (
	generatedProfilesDir          = "generated"
	generatedOrchestratorFilename = "orchestrator.md"
)

// DefaultOrchestratorInstructions are written for newly initialized projects.
const DefaultOrchestratorInstructions = `You are the coordinator for this Fledge project.

Spawn workers with explicit, noninteractive commands of this form:
fledge agent spawn --name <name> --harness <claude|codex|pi|opencode> --prompt <prompt>

Coordinate exclusively through Fledge's injected session messaging commands:
fledge agent message send <recipient> <text>
fledge agent message reply <message-id> <text>

Messages are delivered directly to live agents. Use reply with the incoming message ID to preserve correlation. Treat an injected completion message as the worker's completion signal, then stop that worker with fledge agent stop <name>. Never poll the Fledge inbox or use direct Herdr commands to inspect, prompt, or collect agent output. Do not start nested Fledge or Herdr sessions.`

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
	return ensureGeneratedOrchestratorPrompt(root, instructions, nil)
}

// ensureGeneratedOrchestratorPrompt takes afterValidate so a test can occupy the
// window between validating the path and opening it, which is where a planted
// symlink would race the check. Production passes nil.
func ensureGeneratedOrchestratorPrompt(root, instructions string, afterValidate func()) (string, error) {
	path := filepath.Join(root, stateDirectory, profilesDir, generatedProfilesDir, generatedOrchestratorFilename)
	if err := rejectGeneratedPromptSymlink(path); err != nil {
		return "", err
	}
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, []byte(instructions)) {
		if err := secureGeneratedPrompt(path); err != nil {
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
	if afterValidate != nil {
		afterValidate()
	}
	// The open is the guard: a symlink planted since the check above is refused
	// here rather than followed. Truncation happens through the validated handle
	// instead of with O_TRUNC, because a Windows open would follow such a symlink
	// and truncate its target before fsutil could reject it.
	file, err := fsutil.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return "", fmt.Errorf("create generated orchestrator prompt %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("protect generated orchestrator prompt %q: %w", path, err)
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("truncate generated orchestrator prompt %q: %w", path, err)
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

// secureGeneratedPrompt narrows the mode of an unchanged prompt through a
// validated handle, so a symlink planted since it was inspected is not
// chmodded through.
func secureGeneratedPrompt(path string) error {
	file, err := fsutil.OpenRegular(path, os.O_RDONLY, 0o600)
	if err != nil {
		return err
	}
	return errors.Join(file.Chmod(0o600), file.Close())
}

func rejectGeneratedPromptSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect generated orchestrator prompt %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("generated orchestrator prompt %q must not be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generated orchestrator prompt %q must be a regular file", path)
	}
	return nil
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

func parseProfile(input string) (OrchestratorProfile, error) {
	parser := profileParser{input: input}
	values := make(map[string]string, 2)

	for {
		parser.skipSpaceAndComments()
		if parser.done() {
			break
		}
		key, err := parser.readKey()
		if err != nil {
			return OrchestratorProfile{}, err
		}
		if key != "schema_version" && key != "instructions" {
			return OrchestratorProfile{}, fmt.Errorf("unknown key %q", key)
		}
		if _, exists := values[key]; exists {
			return OrchestratorProfile{}, fmt.Errorf("duplicate key %q", key)
		}
		parser.skipHorizontalSpace()
		if !parser.consume("=") {
			return OrchestratorProfile{}, fmt.Errorf("key %q is missing '='", key)
		}
		parser.skipHorizontalSpace()

		var value string
		if key == "schema_version" {
			value, err = parser.readInteger()
		} else {
			value, err = parser.readString()
		}
		if err != nil {
			return OrchestratorProfile{}, fmt.Errorf("key %q: %w", key, err)
		}
		if err := parser.finishAssignment(); err != nil {
			return OrchestratorProfile{}, fmt.Errorf("key %q: %w", key, err)
		}
		values[key] = value
	}

	versionText, ok := values["schema_version"]
	if !ok {
		return OrchestratorProfile{}, fmt.Errorf("missing key %q", "schema_version")
	}
	version, err := strconv.Atoi(versionText)
	if err != nil {
		return OrchestratorProfile{}, fmt.Errorf("invalid schema_version %q", versionText)
	}
	if version != SchemaVersion {
		return OrchestratorProfile{}, fmt.Errorf("unsupported schema_version %d", version)
	}
	instructions, ok := values["instructions"]
	if !ok {
		return OrchestratorProfile{}, fmt.Errorf("missing key %q", "instructions")
	}
	if strings.TrimSpace(instructions) == "" {
		return OrchestratorProfile{}, fmt.Errorf("instructions must not be empty")
	}

	return OrchestratorProfile{SchemaVersion: version, Instructions: instructions}, nil
}

type profileParser struct {
	input string
	pos   int
}

func (p *profileParser) done() bool {
	return p.pos >= len(p.input)
}

func (p *profileParser) consume(value string) bool {
	if !strings.HasPrefix(p.input[p.pos:], value) {
		return false
	}
	p.pos += len(value)
	return true
}

func (p *profileParser) skipHorizontalSpace() {
	for !p.done() && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *profileParser) skipSpaceAndComments() {
	for {
		for !p.done() && unicode.IsSpace(rune(p.input[p.pos])) {
			p.pos++
		}
		if p.done() || p.input[p.pos] != '#' {
			return
		}
		p.skipComment()
	}
}

func (p *profileParser) skipComment() {
	for !p.done() && p.input[p.pos] != '\n' {
		p.pos++
	}
}

func (p *profileParser) readKey() (string, error) {
	start := p.pos
	for !p.done() {
		char := p.input[p.pos]
		if (char >= 'a' && char <= 'z') || char == '_' {
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return "", fmt.Errorf("expected a bare key at byte %d", p.pos)
	}
	return p.input[start:p.pos], nil
}

func (p *profileParser) readInteger() (string, error) {
	start := p.pos
	for !p.done() && p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		p.pos++
	}
	if start == p.pos {
		return "", fmt.Errorf("expected a positive integer")
	}
	return p.input[start:p.pos], nil
}

func (p *profileParser) readString() (string, error) {
	switch {
	case p.consume("\"\"\""):
		return p.readMultilineString("\"\"\"", true)
	case p.consume("'''"):
		return p.readMultilineString("'''", false)
	case p.consume("\""):
		return p.readSingleLineString('"', true)
	case p.consume("'"):
		return p.readSingleLineString('\'', false)
	default:
		return "", fmt.Errorf("expected a quoted string")
	}
}

func (p *profileParser) readSingleLineString(quote byte, escaped bool) (string, error) {
	start := p.pos
	for !p.done() {
		char := p.input[p.pos]
		if char == '\n' || char == '\r' {
			return "", fmt.Errorf("unterminated string")
		}
		if char == quote {
			raw := p.input[start:p.pos]
			p.pos++
			if escaped {
				return unescapeBasicString(raw)
			}
			return validateLiteralString(raw)
		}
		if escaped && char == '\\' {
			p.pos++
			if p.done() {
				return "", fmt.Errorf("unterminated escape")
			}
		}
		p.pos++
	}
	return "", fmt.Errorf("unterminated string")
}

func (p *profileParser) readMultilineString(delimiter string, escaped bool) (string, error) {
	if p.consume("\r\n") || p.consume("\n") {
		// TOML trims the newline immediately following an opening delimiter.
	}
	start := p.pos
	for !p.done() {
		if strings.HasPrefix(p.input[p.pos:], delimiter) {
			runLength := 0
			for p.pos+runLength < len(p.input) && p.input[p.pos+runLength] == delimiter[0] {
				runLength++
			}
			if runLength > 5 {
				return "", fmt.Errorf("too many consecutive quote characters")
			}
			raw := p.input[start:p.pos] + strings.Repeat(delimiter[:1], runLength-len(delimiter))
			p.pos += runLength
			if escaped {
				return unescapeBasicString(raw)
			}
			return validateLiteralString(raw)
		}
		p.pos++
	}
	return "", fmt.Errorf("unterminated multiline string")
}

func validateLiteralString(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("invalid UTF-8 in string")
	}
	for _, value := range raw {
		if (value < 0x20 && value != '\t' && value != '\n' && value != '\r') || value == 0x7f {
			return "", fmt.Errorf("invalid control character in string")
		}
	}
	return raw, nil
}

func unescapeBasicString(raw string) (string, error) {
	var value strings.Builder
	for len(raw) > 0 {
		if raw[0] == '\r' {
			if len(raw) > 1 && raw[1] == '\n' {
				raw = raw[2:]
			} else {
				raw = raw[1:]
			}
			value.WriteByte('\n')
			continue
		}
		if raw[0] != '\\' {
			r, size := utf8.DecodeRuneInString(raw)
			if r == utf8.RuneError && size == 1 {
				return "", fmt.Errorf("invalid UTF-8 in string")
			}
			if (r < 0x20 && r != '\t' && r != '\n') || r == 0x7f {
				return "", fmt.Errorf("invalid control character in string")
			}
			value.WriteString(raw[:size])
			raw = raw[size:]
			continue
		}
		if len(raw) > 1 && (raw[1] == '\n' || raw[1] == '\r') {
			raw = raw[1:]
			if len(raw) > 1 && raw[0] == '\r' && raw[1] == '\n' {
				raw = raw[2:]
			} else {
				raw = raw[1:]
			}
			for len(raw) > 0 && strings.ContainsRune(" \t\r\n", rune(raw[0])) {
				raw = raw[1:]
			}
			continue
		}
		if len(raw) < 2 || !strings.ContainsRune(`btnfr"\uU`, rune(raw[1])) {
			return "", fmt.Errorf("invalid string escape")
		}

		r, _, tail, err := strconv.UnquoteChar(raw, '"')
		if err != nil {
			return "", fmt.Errorf("invalid string escape: %w", err)
		}
		value.WriteRune(r)
		raw = tail
	}
	return value.String(), nil
}

func (p *profileParser) finishAssignment() error {
	p.skipHorizontalSpace()
	if !p.done() && p.input[p.pos] == '#' {
		p.skipComment()
	}
	if p.done() {
		return nil
	}
	if p.consume("\r\n") || p.consume("\n") {
		return nil
	}
	return fmt.Errorf("unexpected content at byte %d", p.pos)
}
