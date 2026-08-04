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
