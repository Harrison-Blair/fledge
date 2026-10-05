package spawn

import "strings"

// permissionArguments supplies bypass defaults unless the caller chose permissions.
func permissionArguments(kind string, args []string) []string {
	var defaults []string
	switch kind {
	case "claude":
		defaults = []string{"--permission-mode", "bypassPermissions"}
	case "codex":
		defaults = []string{"--yolo"}
	default:
		return args
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		flag, _, attached := strings.Cut(arg, "=")
		// Model and profile values are operands, not permission options.
		if flag == "--model" || flag == "-m" || kind == "codex" && (flag == "--profile" || flag == "-p") {
			if !attached {
				i++
			}
			continue
		}
		if kind == "claude" {
			if flag == "--permission-mode" || flag == "--dangerously-skip-permissions" {
				return args
			}
			continue
		}
		switch flag {
		case "--yolo", "--dangerously-bypass-approvals-and-sandbox", "--sandbox", "--ask-for-approval", "--approve-for-me", "--full-auto":
			return args
		}
		if strings.HasPrefix(arg, "-s") && !strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-a") && !strings.HasPrefix(arg, "--") {
			return args
		}
		config := ""
		switch {
		case arg == "-c" || arg == "--config":
			i++
			if i < len(args) {
				config = args[i]
			}
		case strings.HasPrefix(arg, "--config="):
			config = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-c"):
			config = strings.TrimPrefix(strings.TrimPrefix(arg, "-c"), "=")
		}
		key, _, ok := strings.Cut(config, "=")
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		if ok && (key == "approval_policy" || key == "sandbox_mode" || key == "default_permissions" || key == "permissions" || strings.HasPrefix(key, "permissions.")) {
			return args
		}
	}
	return append(defaults, args...)
}
