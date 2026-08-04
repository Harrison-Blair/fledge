package harness

import (
	"fmt"
	"strings"
)

// ValidateNativeArgs rejects model-selection flags in native passthrough
// arguments. Fledge owns model selection so it cannot safely permit a second,
// harness-native value.
func ValidateNativeArgs(args []string) error {
	for index, arg := range args {
		if arg == "--model" || strings.HasPrefix(arg, "--model=") ||
			(arg != "--" && strings.HasPrefix(arg, "-m") && !strings.HasPrefix(arg, "--")) {
			return fmt.Errorf(
				"native argument %d %q selects a model; use Fledge's --model option instead",
				index+1,
				arg,
			)
		}
	}
	return nil
}

// BuildArgs maps Fledge's resolved model to the selected harness's native
// --model option, followed by native passthrough arguments in their original
// order. A model need not appear in a discovered Catalog: explicit custom IDs
// are valid. Claude Code additionally defaults to bypassing permission
// prompts unless the native arguments already choose a permission behavior.
// The returned slice never aliases nativeArgs.
func BuildArgs(selected Harness, model string, nativeArgs []string) ([]string, error) {
	if err := ValidateNativeArgs(nativeArgs); err != nil {
		return nil, err
	}
	if !supportedID(selected.ID) {
		return nil, fmt.Errorf("unsupported harness %q", selected.ID)
	}

	injectBypass := selected.ID == "claude" && !hasPermissionFlag(nativeArgs)

	capacity := len(nativeArgs)
	if model != "" {
		capacity += 2
	}
	if injectBypass {
		capacity += 2
	}
	args := make([]string, 0, capacity)
	if model != "" {
		args = append(args, "--model", model)
	}
	if injectBypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	}
	return append(args, nativeArgs...), nil
}

// AppendOrchestratorInstructions adds Fledge's durable coordinator policy to
// the end of the harness arguments so Fledge's value takes precedence over
// conflicting native passthrough arguments.
func AppendOrchestratorInstructions(selected Harness, args []string, instructions string) ([]string, error) {
	result := append([]string(nil), args...)
	switch selected.ID {
	case "claude", "pi":
		return append(result, "--append-system-prompt", instructions), nil
	case "codex":
		return append(result, "-c", "developer_instructions="+tomlBasicString(instructions)), nil
	case "opencode":
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported harness %q", selected.ID)
	}
}

func tomlBasicString(value string) string {
	var encoded strings.Builder
	encoded.WriteByte('"')
	for _, char := range value {
		switch char {
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
		case '"':
			encoded.WriteString(`\"`)
		case '\\':
			encoded.WriteString(`\\`)
		default:
			if char < 0x20 || char == 0x7f {
				fmt.Fprintf(&encoded, `\u%04X`, char)
			} else {
				encoded.WriteRune(char)
			}
		}
	}
	encoded.WriteByte('"')
	return encoded.String()
}

func hasPermissionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--permission-mode" || strings.HasPrefix(arg, "--permission-mode=") ||
			arg == "--dangerously-skip-permissions" {
			return true
		}
	}
	return false
}

func supportedID(id string) bool {
	for _, candidate := range supported {
		if candidate.ID == id {
			return true
		}
	}
	return false
}
