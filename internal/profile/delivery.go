package profile

import (
	"fmt"
	"strings"
)

const (
	piHarness     = "pi"
	claudeHarness = "claude"
	codexHarness  = "codex"
)

// UnsupportedHarnessError reports that a harness has no native profile
// delivery adapter.
type UnsupportedHarnessError struct {
	Harness string
}

func (e *UnsupportedHarnessError) Error() string {
	return fmt.Sprintf("harness %q does not support native profile delivery", e.Harness)
}

// InstructionArgumentConflictError reports a caller-supplied harness argument
// that competes with Fledge's profile instruction channel.
type InstructionArgumentConflictError struct {
	Harness  string
	Argument string
}

func (e *InstructionArgumentConflictError) Error() string {
	return fmt.Sprintf("harness %q argument %q conflicts with profile instruction delivery", e.Harness, e.Argument)
}

// LaunchArgs returns a copy of args with the profile delivered through the
// harness's native instruction channel. instructionFile is the runtime-owned
// snapshot path used by the file-backed Pi and Claude adapters; Codex receives
// the snapshot text as a TOML string. Caller-supplied instruction arguments
// are rejected instead of being combined with the profile.
func LaunchArgs(configured Profile, harness, instructionFile string, args []string) ([]string, error) {
	if conflict := instructionConflict(harness, args); conflict != "" {
		return nil, &InstructionArgumentConflictError{Harness: harness, Argument: conflict}
	}

	var delivery []string
	switch harness {
	case piHarness:
		if instructionFile == "" {
			return nil, fmt.Errorf("deliver profile %q to harness %q: instruction file path is empty", configured.Name, harness)
		}
		delivery = []string{"--append-system-prompt", instructionFile}
	case claudeHarness:
		if instructionFile == "" {
			return nil, fmt.Errorf("deliver profile %q to harness %q: instruction file path is empty", configured.Name, harness)
		}
		delivery = []string{"--append-system-prompt-file", instructionFile}
	case codexHarness:
		delivery = []string{"-c", "developer_instructions=" + tomlString(configured.Instructions)}
	default:
		return nil, &UnsupportedHarnessError{Harness: harness}
	}

	result := make([]string, 0, len(delivery)+len(args))
	result = append(result, delivery...)
	result = append(result, args...)
	return result, nil
}

func instructionConflict(harness string, args []string) string {
	switch harness {
	case piHarness:
		return conflictingFlag(args, "--system-prompt", "--append-system-prompt")
	case claudeHarness:
		return conflictingFlag(args,
			"--system-prompt",
			"--system-prompt-file",
			"--append-system-prompt",
			"--append-system-prompt-file",
		)
	case codexHarness:
		return conflictingCodexConfig(args)
	default:
		return ""
	}
}

func conflictingFlag(args []string, reserved ...string) string {
	for _, arg := range args {
		for _, flag := range reserved {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return arg
			}
		}
	}
	return ""
}

func conflictingCodexConfig(args []string) string {
	for i, arg := range args {
		var setting string
		switch {
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				setting = args[i+1]
			}
		case strings.HasPrefix(arg, "-c="):
			setting = strings.TrimPrefix(arg, "-c=")
		case strings.HasPrefix(arg, "--config="):
			setting = strings.TrimPrefix(arg, "--config=")
		}
		key, _, _ := strings.Cut(setting, "=")
		if strings.TrimSpace(key) == "developer_instructions" {
			return arg
		}
	}
	return ""
}

// tomlString encodes text as a TOML basic string without relying on Go's \x
// escapes, which TOML does not accept.
func tomlString(text string) string {
	var encoded strings.Builder
	encoded.Grow(len(text) + 2)
	encoded.WriteByte('"')
	for _, r := range text {
		switch r {
		case '"':
			encoded.WriteString(`\"`)
		case '\\':
			encoded.WriteString(`\\`)
		case '\b':
			encoded.WriteString(`\b`)
		case '\t':
			encoded.WriteString(`\t`)
		case '\n':
			encoded.WriteString(`\n`)
		case '\f':
			encoded.WriteString(`\f`)
		case '\r':
			encoded.WriteString(`\r`)
		default:
			if r <= 0x1f || r == 0x7f {
				fmt.Fprintf(&encoded, `\u%04X`, r)
			} else {
				encoded.WriteRune(r)
			}
		}
	}
	encoded.WriteByte('"')
	return encoded.String()
}
