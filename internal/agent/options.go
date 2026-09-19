package agent

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// SpawnOptions are the user-supplied launch and placement settings.
type SpawnOptions struct {
	Provided                                                                                                      []string
	Name, Harness, Model, Workspace, WorkspaceID, Tab, TabID, Pane, Worktree, Branch, Base, Cwd, Label, Direction string
	Prompt, File                                                                                                  string
	Env, Args                                                                                                     []string
	Focus, DirectionSet, NoWait, PromptSet, FileSet                                                               bool
	Ratio                                                                                                         *float64
	Timeout                                                                                                       time.Duration
}

// Validate performs local checks and returns the native argument vector.
func (o SpawnOptions) Validate() ([]string, error) {
	values := map[string]string{"name": o.Name, "harness": o.Harness, "model": o.Model, "workspace": o.Workspace, "workspace-id": o.WorkspaceID, "tab": o.Tab, "tab-id": o.TabID, "pane": o.Pane, "worktree": o.Worktree, "branch": o.Branch, "base": o.Base, "cwd": o.Cwd, "label": o.Label}
	for _, flag := range o.Provided {
		if value, ok := values[flag]; ok && value == "" {
			return nil, invalid("--%s cannot be empty", flag)
		}
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`).MatchString(o.Name) {
		return nil, invalid("--name must match [a-z][a-z0-9_-]{0,31}")
	}
	if err := validateKind(o.Harness); err != nil {
		return nil, err
	}
	if o.Timeout.Milliseconds() <= 3000 || o.Timeout.Milliseconds() > 300000 {
		return nil, invalid("--timeout must convert to 3001 through 300000 milliseconds")
	}
	if o.Workspace != "" && o.WorkspaceID != "" || o.Tab != "" && o.TabID != "" {
		return nil, invalid("name and ID selectors are mutually exclusive")
	}
	if o.Pane != "" && (o.Workspace != "" || o.WorkspaceID != "" || o.Tab != "" || o.TabID != "" || o.Worktree != "" || o.Cwd != "" || len(o.Env) > 0 || o.DirectionSet || o.Ratio != nil) {
		return nil, invalid("--pane cannot be combined with placement, cwd, env, or split settings")
	}
	if o.Worktree != "" && (len(o.Env) > 0 || o.TabID != "") {
		return nil, invalid("--worktree cannot be combined with --env or --tab-id")
	}
	if o.NoWait && (o.PromptSet || o.FileSet) {
		return nil, invalid("--no-wait cannot be combined with --prompt or --file")
	}
	if o.Worktree != "new" && (o.Branch != "" || o.Base != "") {
		return nil, invalid("--branch and --base require --worktree new")
	}
	if o.Direction != "right" && o.Direction != "down" {
		return nil, invalid("--direction must be right or down")
	}
	if o.Ratio != nil && (math.IsNaN(*o.Ratio) || math.IsInf(*o.Ratio, 0) || *o.Ratio <= 0 || *o.Ratio >= 1) {
		return nil, invalid("--ratio must be greater than zero and less than one")
	}
	for _, e := range o.Env {
		key, _, ok := strings.Cut(e, "=")
		if !ok || key == "" || strings.ContainsRune(e, 0) || !utf8.ValidString(e) {
			return nil, invalid("--env requires a nonempty KEY=VALUE without NUL")
		}
	}
	for _, s := range append([]string{o.Workspace, o.WorkspaceID, o.Tab, o.TabID, o.Pane, o.Model, o.Cwd, o.Worktree, o.Branch, o.Base, o.Label}, o.Args...) {
		if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
			return nil, invalid("arguments must be valid UTF-8 without NUL")
		}
	}
	return modelArguments(o.Harness, o.Model, o.Args)
}
func modelArguments(kind, model string, args []string) ([]string, error) {
	result := append([]string{}, args...)
	if model == "" {
		return result, nil
	}
	if slices.Contains([]string{"kiro", "amp", "muse", "mastracode", "qodercli"}, kind) {
		return nil, invalid("--model is not verified for %s; pass native arguments explicitly", kind)
	}
	short := slices.Contains(strings.Fields("codex gemini cline opencode kimi droid grok hermes kilo qwen letta maki"), kind)
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--model" || strings.HasPrefix(arg, "--model=") || short && strings.HasPrefix(arg, "-m") {
			return nil, invalid("native model option conflicts with --model")
		}
		if kind == "codex" {
			config := ""
			switch {
			case arg == "-c" || arg == "--config":
				if i+1 < len(args) {
					config = args[i+1]
				}
			case strings.HasPrefix(arg, "--config="):
				config = strings.TrimPrefix(arg, "--config=")
			case strings.HasPrefix(arg, "-c"):
				config = strings.TrimPrefix(strings.TrimPrefix(arg, "-c"), "=")
			}
			key, _, ok := strings.Cut(config, "=")
			key = strings.Trim(strings.TrimSpace(key), `"'`)
			if ok && key == "model" {
				return nil, invalid("native Codex model configuration conflicts with --model")
			}
		}
	}
	prefix := []string{"--model", model}
	if kind == "hermes" {
		prefix = append([]string{"chat"}, prefix...)
		if len(result) > 0 && result[0] == "chat" {
			result = result[1:]
		}
	}
	return append(prefix, result...), nil
}

// kinds lists the documented Herdr harness kinds accepted by --harness.
var kinds = strings.Fields("pi claude codex gemini cursor devin agy cline omp mastracode opencode copilot kimi kiro droid amp grok hermes kilo qodercli qwen letta maki muse")

func validateKind(kind string) error {
	if !slices.Contains(kinds, kind) {
		return invalid("--harness must be a documented Herdr harness kind")
	}
	return nil
}
func invalid(format string, args ...any) error {
	return &InputError{Message: fmt.Sprintf(format, args...)}
}

// InputError identifies invalid CLI input (exit status 2).
type InputError struct{ Message string }

func (e *InputError) Error() string { return e.Message }
