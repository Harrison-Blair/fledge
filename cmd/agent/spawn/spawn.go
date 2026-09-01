// Package spawn adapts agent spawning to Cobra.
package spawn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	internalagent "fledge/internal/agent"
	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/picker"
	"fledge/internal/session"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type spawnOperation func(context.Context, internalagent.SpawnOptions) (internalagent.SpawnResult, error)
type terminalDetector func(int) bool
type resolverFactory func(io.Reader, io.Writer) picker.Resolver

const modelTimeout = 5 * time.Second

// initialPromptStatus is the redacted delivery outcome attached to a partial
// spawn result. It carries only the fixed status string, the whitelisted
// SafeCode, and a secret-free retry command; it never renders the prompt text,
// its path, the raw cause, or any Herder operation, message, or code.
type initialPromptStatus struct {
	Status    string   `json:"status"`
	Code      string   `json:"code"`
	RetryArgv []string `json:"retry_argv"`
}

// partialSpawnResult flattens the seven-field SpawnResult and appends the
// initial-prompt delivery status. The embedded result is the sole authoritative
// state reported when an agent starts but its prompt is unconfirmed.
type partialSpawnResult struct {
	internalagent.SpawnResult
	InitialPrompt initialPromptStatus `json:"initial_prompt"`
}

// New constructs the agent spawn command.
func New() *cobra.Command {
	return newCommand(spawn, term.IsTerminal, func(input io.Reader, output io.Writer) picker.Resolver {
		return picker.Resolver{
			Input:  input,
			Output: output,
			Models: func(ctx context.Context, harness catalog.Harness) []string {
				return catalog.Models(ctx, harness, modelTimeout)
			},
		}
	})
}

func spawn(ctx context.Context, options internalagent.SpawnOptions) (internalagent.SpawnResult, error) {
	base := herdr.New(nil, nil, nil)
	caller, client, err := internalagent.Connect(ctx, ".", os.Getenv, base.List, func(name string) session.PaneResolver { return base.WithSession(name) })
	if err != nil {
		return internalagent.SpawnResult{}, err
	}
	return internalagent.Spawn(ctx, client, caller, options)
}

func newCommand(spawn spawnOperation, isTerminal terminalDetector, resolver resolverFactory) *cobra.Command {
	var options internalagent.SpawnOptions
	var request picker.LaunchRequest
	var ratio float64
	var promptText, promptFile string

	command := &cobra.Command{
		Use:   "spawn <name> [--harness HARNESS] [--prompt TEXT | --prompt-file PATH] [-- harness arguments]",
		Short: "Start an agent in a new Herder pane",
		Args: func(cmd *cobra.Command, args []string) error {
			named := len(args)
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				named = dash
			}
			if named != 1 {
				return fmt.Errorf("accepts 1 name argument, received %d", named)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			options.Name = args[0]
			request.Args = nil
			if dash := cmd.ArgsLenAtDash(); dash != -1 {
				request.Args = args[dash:]
			}
			if cmd.Flags().Changed("ratio") {
				options.Ratio = &ratio
			}

			// Load and fully validate the initial prompt before terminal
			// detection, resolver construction, resolver prompts, or any launch
			// side effect, so invalid input can never mutate or invoke
			// resolution.
			prompt, err := loadInitialPrompt(
				cmd.Flags().Changed("prompt"), promptText,
				cmd.Flags().Changed("prompt-file"), promptFile,
			)
			if err != nil {
				return err
			}
			options.InitialPrompt = prompt

			input := cmd.InOrStdin()
			output := cmd.OutOrStdout()
			request.PromptProfile = true
			request.Interactive = streamIsTerminal(input, isTerminal) && streamIsTerminal(output, isTerminal)
			choice, err := resolver(input, output).Resolve(cmd.Context(), request)
			if err != nil {
				return err
			}
			options.Harness = string(choice.Harness)
			options.Model = choice.Model
			options.Profile = choice.Profile
			options.Args = choice.Args

			result, err := spawn(cmd.Context(), options)
			if err != nil {
				return reportSpawnError(cmd.OutOrStdout(), result, err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}

	flags := command.Flags()
	flags.StringVar(&request.Harness, "harness", "", "agent harness to start")
	flags.StringVar(&request.Model, "model", "", "model passed to the harness")
	flags.StringVar(&request.Profile, "profile", "", "Fledge-managed agent profile to load")
	flags.BoolVar(&request.NoProfile, "no-profile", false, "start without an agent profile")
	flags.StringVar(&options.Workspace, "workspace", "", `place the agent in "new" or an existing workspace ID`)
	flags.StringVar(&options.Tab, "tab", "", "split a pane of this tab ID")
	flags.StringVar(&options.Pane, "pane", "", "split this pane ID")
	flags.StringVar(&options.Split, "split", "", "direction for --tab or --pane placement: right or down (default right)")
	flags.Float64Var(&ratio, "ratio", 0, "fraction of the split pane given to the agent")
	flags.StringVar(&options.Label, "label", "", "workspace or tab label (defaults to the agent name)")
	flags.StringVar(&promptText, "prompt", "", "send TEXT to the agent once after it starts, without waiting; max 100 KiB UTF-8; not confidential, so not for secrets")
	flags.StringVar(&promptFile, "prompt-file", "", "read the initial prompt from PATH (stdin via - is unsupported); sent once after the agent starts, without waiting; max 100 KiB UTF-8; not confidential, so not for secrets")
	command.MarkFlagsMutuallyExclusive("profile", "no-profile")
	command.MarkFlagsMutuallyExclusive("prompt", "prompt-file")

	return command
}

// loadInitialPrompt resolves the optional initial prompt. Omitting both flags
// yields a nil pointer, preserving InitialPrompt=nil; an explicit --prompt
// (empty included) or --prompt-file is fully validated through the internal
// validator so invalid input fails fast before any resolution. The two flags
// are mutually exclusive at the Cobra layer, so at most one is changed here.
func loadInitialPrompt(promptChanged bool, promptText string, fileChanged bool, promptFile string) (*string, error) {
	switch {
	case promptChanged:
		if err := internalagent.ValidateInitialPrompt(promptText); err != nil {
			return nil, err
		}
		return &promptText, nil
	case fileChanged:
		text, err := readPromptFile(promptFile)
		if err != nil {
			return nil, err
		}
		if err := internalagent.ValidateInitialPrompt(text); err != nil {
			return nil, err
		}
		return &text, nil
	default:
		return nil, nil
	}
}

// readPromptFile reads a prompt file exactly once and returns its bytes verbatim
// as a string. The exact path "-" is rejected because stdin delivery is
// unsupported; "./-" and other paths are read literally. IO errors carry the
// path for context but never any file content.
func readPromptFile(path string) (string, error) {
	if path == "-" {
		return "", errors.New(`prompt-file "-" is not supported; reading the prompt from stdin is unavailable`)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt-file %q: %w", path, err)
	}
	return string(data), nil
}

// maxMarkerDepth bounds the trusted-marker unwrap traversal so a self- or
// mutually-unwrapping cause cannot recurse forever.
const maxMarkerDepth = 100

// maxMarkerWork caps the total nodes the trusted-marker traversal may visit
// across the whole walk. Depth alone leaves a branching Unwrap() []error cycle
// free to revisit ~2^maxMarkerDepth nodes; this budget keeps total work linear.
// The genuine Spawn chain is a handful of fmt/errors.Join wrappers, well under
// this bound, while an adversarial graph burns out in microseconds.
const maxMarkerWork = 256

// markerStatus reports how the traversal of one subtree ended: it located an
// actual *internalagent.InitialPromptError value, it searched the subtree
// exhaustively and proved none is present, or a depth or work bound cut it short
// before it could tell.
type markerStatus int

const (
	markerAbsent markerStatus = iota
	markerFound
	markerTruncated
)

// firstInitialPromptError returns the first value whose dynamic type is
// *internalagent.InitialPromptError reachable from err through the standard
// Unwrap() error and Unwrap() []error chains, in ordinary depth-first
// left-to-right order. It is the CLI trust boundary: recognition uses direct
// type assertions only, so no custom As(any) bool hook is ever consulted. A
// hostile error therefore cannot forge the marker, and a typed-nil marker is
// reported as found-but-nil for the caller to reject rather than trusted.
//
// depth bounds recursion against unwrap cycles, and budget is a shared count of
// remaining node visits: every call spends exactly one unit (nil and
// depth-refused nodes included), so a branching cycle or wide child slice does
// at most maxMarkerWork constant-time visits. Exhausting either bound returns
// markerTruncated immediately — a cut-short branch could hide the true first
// marker, so traversal must never continue to a later sibling past it. Only a
// subtree proven empty returns markerAbsent and lets the walk move on.
func firstInitialPromptError(err error, depth int, budget *int) (*internalagent.InitialPromptError, markerStatus) {
	if *budget <= 0 {
		return nil, markerTruncated
	}
	*budget--
	if err == nil {
		return nil, markerAbsent
	}
	if depth > maxMarkerDepth {
		return nil, markerTruncated
	}
	if here, ok := err.(*internalagent.InitialPromptError); ok {
		return here, markerFound
	}
	switch node := err.(type) {
	case interface{ Unwrap() error }:
		return firstInitialPromptError(node.Unwrap(), depth+1, budget)
	case interface{ Unwrap() []error }:
		for _, child := range node.Unwrap() {
			if *budget <= 0 {
				return nil, markerTruncated
			}
			if here, childStatus := firstInitialPromptError(child, depth+1, budget); childStatus != markerAbsent {
				return here, childStatus
			}
		}
	}
	return nil, markerAbsent
}

// reportSpawnError renders a spawn failure. An error chain carrying a trusted
// *internalagent.InitialPromptError means the agent is live but its prompt was
// unconfirmed: emit exactly one redacted partial-result line built from the
// returned result, then return the original typed chain so Cobra exits nonzero
// through its safe error path. Recognition goes through firstInitialPromptError,
// never errors.As, so a forged or blind As hook, a typed-nil marker, or an
// adversarially truncated branch falls through to the ordinary error path and
// emits no result line. If the partial line cannot be written, join the write
// error to the original and make no second encoding attempt.
func reportSpawnError(out io.Writer, result internalagent.SpawnResult, err error) error {
	budget := maxMarkerWork
	promptErr, status := firstInitialPromptError(err, 0, &budget)
	if status != markerFound || promptErr == nil {
		return err
	}
	partial := partialSpawnResult{
		SpawnResult: result,
		InitialPrompt: initialPromptStatus{
			Status:    "delivery_unconfirmed",
			Code:      promptErr.SafeCode(),
			RetryArgv: []string{"fledge", "agent", "message", result.Name, "--", "<prompt>"},
		},
	}
	if encErr := json.NewEncoder(out).Encode(partial); encErr != nil {
		return errors.Join(err, fmt.Errorf("write partial spawn result: %w", encErr))
	}
	return err
}

func streamIsTerminal(stream any, isTerminal terminalDetector) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && isTerminal(int(file.Fd()))
}
