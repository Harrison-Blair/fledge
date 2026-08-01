package agentspawn

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ProfileLaunchOptions contains the harness-independent parts of a managed
// profile launch. Model selection is deliberately left to the spawn layer.
type ProfileLaunchOptions struct {
	Harness          string
	Effort           string
	Instructions     string
	InstructionsFile string
	NativeArgs       []string
}

var supportedProfileEfforts = map[string]struct{}{
	"low": {}, "medium": {}, "high": {}, "xhigh": {}, "max": {},
}

// controlledLongFlags names native options that could replace or undermine a
// setting owned by Fledge. The union is intentional: a managed profile must
// not acquire a second identity merely because a harness adds an alias.
var controlledLongFlags = map[string]string{
	// Model, provider, harness, and profile identity.
	"agent":           "harness/profile identity",
	"agents":          "harness/profile identity",
	"config":          "harness/profile configuration",
	"fallback-model":  "model selection",
	"local-provider":  "model selection",
	"model":           "model selection",
	"models":          "model selection",
	"oss":             "model selection",
	"profile":         "harness/profile identity",
	"provider":        "model selection",
	"setting-sources": "harness/profile configuration",
	"settings":        "harness/profile configuration",

	// Reasoning and profile instructions.
	"append-system-prompt":      "profile instructions",
	"append-system-prompt-file": "profile instructions",
	"developer-instructions":    "profile instructions",
	"effort":                    "reasoning effort",
	"prompt":                    "profile instructions",
	"reasoning-effort":          "reasoning effort",
	"system-prompt":             "profile instructions",
	"system-prompt-file":        "profile instructions",
	"thinking":                  "reasoning effort",
	"variant":                   "reasoning effort",

	// Working directory and placement.
	"add-dir":  "working directory/project root",
	"cd":       "working directory/project root",
	"cwd":      "working directory/project root",
	"dir":      "working directory/project root",
	"project":  "working directory/project root",
	"tmux":     "session placement",
	"worktree": "working directory/project root",

	// Permission and sandbox policy.
	"allow-dangerously-skip-permissions": "permission/sandbox policy",
	"allowed-tools":                      "permission/sandbox policy",
	"allowedTools":                       "permission/sandbox policy",
	"approve":                            "permission/sandbox policy",
	"ask-for-approval":                   "permission/sandbox policy",
	"auto":                               "permission/sandbox policy",
	"dangerously-bypass-approvals-and-sandbox": "permission/sandbox policy",
	"dangerously-bypass-hook-trust":            "permission/sandbox policy",
	"dangerously-skip-permissions":             "permission/sandbox policy",
	"disallowed-tools":                         "permission/sandbox policy",
	"disallowedTools":                          "permission/sandbox policy",
	"exclude-tools":                            "permission/sandbox policy",
	"no-approve":                               "permission/sandbox policy",
	"no-builtin-tools":                         "permission/sandbox policy",
	"no-context-files":                         "profile instructions",
	"no-tools":                                 "permission/sandbox policy",
	"permission-mode":                          "permission/sandbox policy",
	"permission-prompt-tool":                   "permission/sandbox policy",
	"sandbox":                                  "permission/sandbox policy",
	"tools":                                    "permission/sandbox policy",
	"yolo":                                     "permission/sandbox policy",

	// Session identity, continuation, naming, and placement.
	"attach":                             "session/name placement",
	"background":                         "session/name placement",
	"bg":                                 "session/name placement",
	"continue":                           "session/name placement",
	"fork":                               "session/name placement",
	"fork-session":                       "session/name placement",
	"from-pr":                            "session/name placement",
	"name":                               "session/name placement",
	"no-session":                         "session/name placement",
	"no-session-persistence":             "session/name placement",
	"remote":                             "remote session placement",
	"remote-auth-token-env":              "remote session authentication",
	"remote-control":                     "session/name placement",
	"remote-control-session-name-prefix": "session/name placement",
	"resume":                             "session/name placement",
	"session":                            "session/name placement",
	"session-dir":                        "session/name placement",
	"session-id":                         "session/name placement",
	"title":                              "session/name placement",
}

type shortControlledFlag struct {
	flag    string
	setting string
}

var controlledShortFlags = map[string][]shortControlledFlag{
	"claude": {
		{flag: "-c", setting: "session/name placement"},
		{flag: "-n", setting: "session/name placement"},
		{flag: "-r", setting: "session/name placement"},
		{flag: "-w", setting: "working directory/project root"},
	},
	"codex": {
		{flag: "-C", setting: "working directory/project root"},
		{flag: "-a", setting: "permission/sandbox policy"},
		{flag: "-c", setting: "harness/profile configuration"},
		{flag: "-m", setting: "model selection"},
		{flag: "-p", setting: "harness/profile identity"},
		{flag: "-s", setting: "permission/sandbox policy"},
	},
	"opencode": {
		{flag: "-c", setting: "session/name placement"},
		{flag: "-m", setting: "model selection"},
		{flag: "-p", setting: "profile instructions"},
		{flag: "-s", setting: "session/name placement"},
	},
	"pi": {
		{flag: "-nbt", setting: "permission/sandbox policy"},
		{flag: "-na", setting: "permission/sandbox policy"},
		{flag: "-nc", setting: "profile instructions"},
		{flag: "-nt", setting: "permission/sandbox policy"},
		{flag: "-xt", setting: "permission/sandbox policy"},
		{flag: "-a", setting: "permission/sandbox policy"},
		{flag: "-c", setting: "session/name placement"},
		{flag: "-n", setting: "session/name placement"},
		{flag: "-r", setting: "session/name placement"},
		{flag: "-t", setting: "permission/sandbox policy"},
	},
}

// Native options whose separated values can be distinguished safely from a
// positional prompt or subcommand. Unknown options remain usable with the
// unambiguous --flag=value form.
var nativeLongValueFlags = map[string]map[string]struct{}{
	"claude": stringSet(
		"debug-file", "input-format", "json-schema", "max-budget-usd",
		"output-format", "plugin-dir",
		"plugin-url",
	),
	"codex": stringSet(),
	"opencode": stringSet(
		"hostname", "log-level", "mdns-domain", "port", "replay-limit",
	),
	"pi": stringSet(
		"api-key", "export", "extension", "mode", "prompt-template",
		"skill", "theme",
	),
}

var nativeLongVariadicValueFlags = map[string]map[string]struct{}{
	"claude":   stringSet("betas", "file", "mcp-config"),
	"codex":    stringSet("image"),
	"opencode": stringSet("cors"),
}

var nativeShortValueFlags = map[string]map[string]struct{}{
	"pi": {"-e": {}},
}

var nativeShortVariadicValueFlags = map[string]map[string]struct{}{
	"codex": {"-i": {}},
}

var nativeShortOptionalValueFlags = map[string]map[string]struct{}{
	"claude": {"-d": {}},
}

var nativeShortBooleanFlags = map[string]map[string]struct{}{
	"pi": {"-ne": {}, "-np": {}, "-ns": {}},
}

// ValidateProfileLaunch checks whether a managed profile can be represented by
// a harness. It deliberately does not require launch-time artifacts such as
// Pi's prepared instruction file, so callers can use it for compatibility
// filtering before a harness is selected.
func ValidateProfileLaunch(opts ProfileLaunchOptions) error {
	if _, ok := controlledShortFlags[opts.Harness]; !ok {
		return fmt.Errorf(
			"unsupported profile launch harness %q; supported harnesses are claude, codex, opencode, and pi",
			opts.Harness,
		)
	}
	if opts.Effort != "" {
		if _, ok := supportedProfileEfforts[opts.Effort]; !ok {
			return fmt.Errorf(
				"unsupported profile effort %q for harness %q; supported efforts are low, medium, high, xhigh, and max",
				opts.Effort, opts.Harness,
			)
		}
	}
	if !utf8.ValidString(opts.Instructions) {
		return fmt.Errorf("profile instructions for harness %q are not valid UTF-8", opts.Harness)
	}
	if strings.IndexByte(opts.Instructions, 0) >= 0 {
		return fmt.Errorf("profile instructions for harness %q contain a NUL byte", opts.Harness)
	}
	// OpenCode exposes --variant only on its non-interactive run command, while
	// the TUI's --prompt is an initial user message rather than managed context.
	if opts.Harness == "opencode" && (opts.Effort != "" || opts.Instructions != "") {
		setting, verb := "effort and instructions", "are"
		if opts.Effort == "" {
			setting = "instructions"
			verb = "are"
		} else if opts.Instructions == "" {
			setting = "effort"
			verb = "is"
		}
		return fmt.Errorf(
			"managed %s for harness %q %s unsupported: the interactive OpenCode TUI has no reliable native transport",
			setting, opts.Harness, verb,
		)
	}
	if err := validateNativeArgs(opts.Harness, opts.NativeArgs); err != nil {
		return err
	}
	return nil
}

// BuildProfileArgs translates managed profile controls into harness-native
// arguments and appends validated native passthrough arguments in their
// original order.
func BuildProfileArgs(opts ProfileLaunchOptions) ([]string, error) {
	if err := ValidateProfileLaunch(opts); err != nil {
		return nil, err
	}
	if opts.Harness == "pi" && opts.Instructions != "" {
		if opts.InstructionsFile == "" {
			return nil, fmt.Errorf(
				"profile instructions for harness %q require a prepared instruction file before launch",
				opts.Harness,
			)
		}
		if !filepath.IsAbs(opts.InstructionsFile) {
			return nil, fmt.Errorf(
				"prepared instruction file for harness %q must be an absolute path",
				opts.Harness,
			)
		}
	}

	args := make([]string, 0, len(opts.NativeArgs)+6)
	switch opts.Harness {
	case "claude":
		args = appendProfileValue(args, "--effort", opts.Effort)
		args = appendProfileValue(args, "--append-system-prompt", opts.Instructions)
	case "codex":
		if opts.Effort != "" {
			args = append(args, "--config", codexStringConfig("model_reasoning_effort", opts.Effort))
		}
		if opts.Instructions != "" {
			args = append(args, "--config", codexStringConfig("developer_instructions", opts.Instructions))
		}
	case "opencode":
	case "pi":
		args = appendProfileValue(args, "--thinking", opts.Effort)
		if opts.Instructions != "" {
			args = append(args, "--append-system-prompt", opts.InstructionsFile)
		}
	}

	return append(args, opts.NativeArgs...), nil
}

func appendProfileValue(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}

func codexStringConfig(key, value string) string {
	encoded, _ := json.Marshal(value) // Encoding a valid UTF-8 string cannot fail.
	return key + "=" + string(encoded)
}

func validateNativeArgs(harness string, args []string) error {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if err := validateNativeArgText(harness, index, arg); err != nil {
			return err
		}
		if arg == "--" {
			return malformedNativeArg(harness, index, arg, "option terminators can smuggle positional launch arguments")
		}
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue, ok := splitLongFlag(arg)
			if !ok {
				return malformedNativeArg(harness, index, arg, "expected --flag or --flag=value")
			}
			canonicalName := canonicalLongFlag(harness, name)
			if setting, reserved := controlledLongFlags[canonicalName]; reserved {
				return controlledNativeArg(harness, index, arg, setting)
			}
			if hasValue && value == "" {
				return malformedNativeArg(harness, index, arg, "inline option value must not be empty")
			}
			if _, variadic := nativeLongVariadicValueFlags[harness][canonicalName]; variadic && !hasValue {
				lastValue, err := validateVariadicValues(harness, args, index)
				if err != nil {
					return err
				}
				index = lastValue
				continue
			}
			if _, needsValue := nativeLongValueFlags[harness][canonicalName]; needsValue && !hasValue {
				if err := validateSeparatedValue(harness, args, index); err != nil {
					return err
				}
				index++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if arg == "-" {
				return malformedNativeArg(harness, index, arg, "a lone dash is a positional argument")
			}
			if _, safe := nativeShortBooleanFlags[harness][arg]; safe {
				continue
			}
			if nativeShortOptionalValueArg(harness, arg) {
				continue
			}
			if flag, variadic := nativeShortVariadicValueArg(harness, arg); variadic {
				if arg == flag {
					lastValue, err := validateVariadicValues(harness, args, index)
					if err != nil {
						return err
					}
					index = lastValue
				} else if shortAttachedValue(flag, arg) == "" {
					return malformedNativeArg(harness, index, arg, "inline option value must not be empty")
				}
				continue
			}
			if flag, needsValue := nativeShortValueArg(harness, arg); needsValue {
				if arg == flag {
					if err := validateSeparatedValue(harness, args, index); err != nil {
						return err
					}
					index++
				} else if shortAttachedValue(flag, arg) == "" {
					return malformedNativeArg(harness, index, arg, "inline option value must not be empty")
				}
				continue
			}
			if setting, reserved := controlledShortArg(harness, arg); reserved {
				return controlledNativeArg(harness, index, arg, setting)
			}
			continue
		}
		return malformedNativeArg(harness, index, arg, "positional prompts, projects, and subcommands are controlled by Fledge")
	}
	return nil
}

func validateNativeArgText(harness string, index int, arg string) error {
	if arg == "" {
		return malformedNativeArg(harness, index, arg, "argument must not be empty")
	}
	if !utf8.ValidString(arg) {
		return malformedNativeArg(harness, index, arg, "argument is not valid UTF-8")
	}
	if strings.IndexByte(arg, 0) >= 0 {
		return malformedNativeArg(harness, index, arg, "argument contains a NUL byte")
	}
	return nil
}

func validateSeparatedValue(harness string, args []string, flagIndex int) error {
	if flagIndex+1 >= len(args) {
		return malformedNativeArg(harness, flagIndex, args[flagIndex], "option requires a value")
	}
	value := args[flagIndex+1]
	if err := validateNativeArgText(harness, flagIndex+1, value); err != nil {
		return err
	}
	if strings.HasPrefix(value, "-") {
		return malformedNativeArg(
			harness, flagIndex, args[flagIndex],
			fmt.Sprintf("option requires a value before %q", value),
		)
	}
	return nil
}

func validateVariadicValues(harness string, args []string, flagIndex int) (int, error) {
	lastValue := flagIndex
	for index := flagIndex + 1; index < len(args); index++ {
		value := args[index]
		if err := validateNativeArgText(harness, index, value); err != nil {
			return 0, err
		}
		if strings.HasPrefix(value, "-") {
			break
		}
		lastValue = index
	}
	if lastValue == flagIndex {
		return 0, malformedNativeArg(harness, flagIndex, args[flagIndex], "option requires at least one value")
	}
	return lastValue, nil
}

func splitLongFlag(arg string) (name, value string, hasValue, ok bool) {
	body := strings.TrimPrefix(arg, "--")
	if body == "" {
		return "", "", false, false
	}
	name, value, hasValue = strings.Cut(body, "=")
	if name == "" {
		return "", "", false, false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' {
			return "", "", false, false
		}
	}
	return name, value, hasValue, true
}

func canonicalLongFlag(harness, name string) string {
	if harness != "opencode" {
		return name
	}
	var canonical strings.Builder
	canonical.Grow(len(name))
	for _, char := range name {
		if char >= 'A' && char <= 'Z' {
			canonical.WriteByte('-')
			canonical.WriteRune(char + ('a' - 'A'))
			continue
		}
		canonical.WriteRune(char)
	}
	return canonical.String()
}

func controlledShortArg(harness, arg string) (string, bool) {
	for _, candidate := range controlledShortFlags[harness] {
		if strings.HasPrefix(arg, candidate.flag) {
			return candidate.setting, true
		}
	}
	// Harness parsers commonly expand boolean short-option bundles. Inspecting
	// every rune prevents forms such as -vmVALUE from hiding a reserved -m.
	for _, char := range arg[1:] {
		for _, candidate := range controlledShortFlags[harness] {
			if len(candidate.flag) == 2 && char == rune(candidate.flag[1]) {
				return candidate.setting, true
			}
		}
	}
	return "", false
}

func nativeShortValueArg(harness, arg string) (string, bool) {
	for flag := range nativeShortValueFlags[harness] {
		if arg == flag || strings.HasPrefix(arg, flag+"=") || strings.HasPrefix(arg, flag) {
			return flag, true
		}
	}
	return "", false
}

func nativeShortVariadicValueArg(harness, arg string) (string, bool) {
	for flag := range nativeShortVariadicValueFlags[harness] {
		if strings.HasPrefix(arg, flag) {
			return flag, true
		}
	}
	return "", false
}

func shortAttachedValue(flag, arg string) string {
	value := strings.TrimPrefix(arg, flag)
	return strings.TrimPrefix(value, "=")
}

func nativeShortOptionalValueArg(harness, arg string) bool {
	for flag := range nativeShortOptionalValueFlags[harness] {
		if strings.HasPrefix(arg, flag) {
			return true
		}
	}
	return false
}

func controlledNativeArg(harness string, index int, arg, setting string) error {
	return fmt.Errorf(
		"native argument %d %q for harness %q conflicts with Fledge-controlled %s",
		index+1, arg, harness, setting,
	)
}

func malformedNativeArg(harness string, index int, arg, detail string) error {
	return fmt.Errorf("native argument %d %q for harness %q is malformed: %s", index+1, arg, harness, detail)
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
