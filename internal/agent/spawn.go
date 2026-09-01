package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"fledge/internal/catalog"
	"fledge/internal/herdr"
	"fledge/internal/profile"
)

const defaultSplitDirection = "right"

const cleanupTimeout = 5 * time.Second

// maxInitialPromptBytes bounds an initial prompt at 100 KiB, measured in UTF-8
// bytes rather than runes.
const maxInitialPromptBytes = 100 * 1024

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// SpawnOptions describes an agent launch. At most one of Workspace, Tab, and
// Pane selects where the agent lands; Workspace is "new" or a workspace ID.
// Ratio applies to the split placements only, Label defaults to Name, Profile
// is an immutable managed snapshot, and Args follow model and profile delivery.
// InitialPrompt is absent when nil; a non-nil value is fully validated and
// delivered once after the agent starts.
type SpawnOptions struct {
	Name          string
	Harness       string
	Model         string
	Profile       *profile.Profile
	Workspace     string
	Tab           string
	Pane          string
	Split         string
	Ratio         *float64
	Label         string
	Args          []string
	InitialPrompt *string
}

// SpawnResult describes the agent that was started and where it landed.
type SpawnResult struct {
	Name        string `json:"name"`
	Harness     string `json:"harness"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

// InitialPromptError reports that an agent started successfully but its initial
// prompt was not acknowledged. The agent, its pane, and any profile artifact
// are intentionally left in place; only Cause records why delivery failed, and
// it is never rendered into the error string.
type InitialPromptError struct {
	Cause error
}

// Error is a fixed, redacted message. It never renders the prompt text, the
// cause, or any Herder operation, message, or code.
func (e *InitialPromptError) Error() string {
	return "agent started but initial prompt delivery was not confirmed"
}

// Unwrap exposes the cause so errors.Is and errors.As traverse the chain while
// Error stays constant.
func (e *InitialPromptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// maxUnwrapDepth bounds SafeCode's unwrap traversal so a pathological self- or
// mutually-unwrapping cause cannot spin forever.
const maxUnwrapDepth = 100

// maxUnwrapWork caps the total nodes SafeCode's traversal may visit across the
// whole walk. Depth alone leaves a branching Unwrap() []error cycle free to
// revisit ~2^maxUnwrapDepth nodes; this budget makes total work linear. 256 is
// generous for real causes — fmt wrappers, errors.Join fanout, and Herder
// chains stay under a dozen nodes, and even a maximally deep legitimate chain
// of maxUnwrapDepth wrappers fits — while an adversarial graph burns out in
// microseconds.
const maxUnwrapWork = 256

// SafeCode classifies the delivery failure using only the whitelisted
// structured Herder codes. Any nil, wait-only, timeout, context, transport, or
// otherwise unrecognized cause reports "unknown"; the Herder operation,
// message, and raw cause are never exposed.
//
// Recognition deliberately avoids errors.As: its As(any) hook lets a hostile
// cause forge a whitelisted code or hand back a typed-nil pointer that would
// panic on dereference. Instead firstStructuredError walks the standard unwrap
// tree with direct type assertions, and only an actual non-nil *herdr.Error
// whose Code is whitelisted is ever surfaced. The walk is bounded by both
// maxUnwrapDepth and maxUnwrapWork; a walk cut short by either bound reports
// "unknown", because the unexplored region could hide the true first
// structured error.
func (e *InitialPromptError) SafeCode() string {
	if e == nil {
		return "unknown"
	}
	budget := maxUnwrapWork
	structured, status := firstStructuredError(e.Cause, 0, &budget)
	if status != walkFound || structured == nil {
		return "unknown"
	}
	switch structured.Code {
	case "agent_blocked", "agent_pane_not_found":
		return structured.Code
	}
	return "unknown"
}

// walkStatus reports how the traversal of one subtree ended: it located an
// actual *herdr.Error value, it searched the subtree exhaustively and proved
// none is present, or the depth or work bound cut it short before it could
// tell.
type walkStatus int

const (
	walkAbsent walkStatus = iota
	walkFound
	walkTruncated
)

// firstStructuredError returns the first value whose dynamic type is
// *herdr.Error reached from err through the standard Unwrap() error and
// Unwrap() []error chains, in ordinary depth-first left-to-right order. It
// uses direct type assertions so no custom As hook is consulted; on walkFound
// the returned pointer may itself be nil when that value is a typed nil.
//
// depth bounds recursion against unwrap cycles, and budget is a shared count
// of remaining node visits: every call spends exactly one unit (nil and
// depth-refused nodes included), so a branching cycle or a wide child slice
// does at most maxUnwrapWork constant-time visits. The child slice is ranged
// in place, never copied.
//
// Exhausting either bound returns walkTruncated immediately — a cut-short
// branch could hide the true first structured error, so traversal must never
// continue to a later sibling past it. Only a subtree proven empty returns
// walkAbsent and lets the walk move on.
func firstStructuredError(err error, depth int, budget *int) (structured *herdr.Error, status walkStatus) {
	if *budget <= 0 {
		return nil, walkTruncated
	}
	*budget--
	if err == nil {
		return nil, walkAbsent
	}
	if depth > maxUnwrapDepth {
		return nil, walkTruncated
	}
	if here, ok := err.(*herdr.Error); ok {
		return here, walkFound
	}
	switch node := err.(type) {
	case interface{ Unwrap() error }:
		return firstStructuredError(node.Unwrap(), depth+1, budget)
	case interface{ Unwrap() []error }:
		for _, child := range node.Unwrap() {
			if *budget <= 0 {
				return nil, walkTruncated
			}
			if here, childStatus := firstStructuredError(child, depth+1, budget); childStatus != walkAbsent {
				return here, childStatus
			}
		}
	}
	return nil, walkAbsent
}

// ValidateInitialPrompt reports why text cannot serve as an agent's initial
// prompt. Checks run in a fixed order so the reason is deterministic: empty,
// then oversize, then invalid UTF-8, then an embedded NUL. Valid content is
// neither trimmed nor normalized.
func ValidateInitialPrompt(text string) error {
	if text == "" {
		return errors.New("initial prompt must not be empty")
	}
	if len(text) > maxInitialPromptBytes {
		return fmt.Errorf("initial prompt must not exceed %d bytes", maxInitialPromptBytes)
	}
	if !utf8.ValidString(text) {
		return errors.New("initial prompt must be valid UTF-8")
	}
	if strings.ContainsRune(text, 0) {
		return errors.New("initial prompt must not contain a NUL byte")
	}
	return nil
}

// Spawn creates a pane for the agent and starts the harness inside it. A failed
// launch closes the pane and removes any file-backed profile artifact.
func Spawn(ctx context.Context, h Herder, caller Caller, opts SpawnOptions) (result SpawnResult, err error) {
	label, err := validateSpawn(opts)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent: %w", err)
	}
	args, cleanupArtifact, err := spawnArgs(caller, opts)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, err)
	}
	retainArtifact := false
	if cleanupArtifact != nil {
		defer func() {
			if retainArtifact {
				return
			}
			if cleanupErr := cleanupArtifact(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}()
	}

	pane, err := placePane(ctx, h, caller, opts, label)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, err)
	}

	if _, startErr := h.StartAgent(ctx, herdr.StartAgentOptions{
		Name:   opts.Name,
		Kind:   opts.Harness,
		PaneID: pane.ID,
		Args:   args,
	}); startErr != nil {
		callerErr := ctx.Err()
		cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		cleanupErr := h.ClosePane(cleanupCtx, pane.ID)
		cancelCleanup()

		failures := []error{fmt.Errorf("start in pane %q: %w", pane.ID, startErr)}
		if callerErr != nil && !errors.Is(startErr, callerErr) {
			failures = append(failures, callerErr)
		}
		if cleanupErr != nil {
			failures = append(failures, fmt.Errorf("close pane %q: %w", pane.ID, cleanupErr))
		}
		return SpawnResult{}, fmt.Errorf("spawn agent %q: %w", opts.Name, errors.Join(failures...))
	}
	retainArtifact = true

	result = SpawnResult{
		Name:        opts.Name,
		Harness:     opts.Harness,
		Model:       opts.Model,
		WorkspaceID: pane.WorkspaceID,
		TabID:       pane.TabID,
		PaneID:      pane.ID,
	}
	if opts.Profile != nil {
		result.Profile = opts.Profile.Name
	}

	if opts.InitialPrompt != nil {
		// The agent is live and its pane and profile artifact are committed.
		// Deliver the initial prompt exactly once, fire-and-forget: a delivery
		// failure leaves the running agent in place and is reported as a
		// partial, unconfirmed launch rather than triggering any teardown,
		// retry, poll, or wait. The opaque acknowledgement is ignored.
		if _, promptErr := Message(ctx, h, MessageOptions{Target: opts.Name, Text: *opts.InitialPrompt}); promptErr != nil {
			return result, fmt.Errorf("spawn agent %q: %w", opts.Name, &InitialPromptError{Cause: promptErr})
		}
	}
	return result, nil
}

func spawnArgs(caller Caller, opts SpawnOptions) ([]string, func() error, error) {
	args := append([]string(nil), opts.Args...)
	var cleanup func() error
	if opts.Profile != nil {
		// Validate the selected harness and every reserved instruction argument
		// before writing an artifact or creating a pane.
		if _, err := profile.LaunchArgs(*opts.Profile, opts.Harness, "/fledge/profile/instructions.md", args); err != nil {
			return nil, nil, fmt.Errorf("prepare profile %q: %w", opts.Profile.Name, err)
		}

		instructionPath := ""
		if opts.Harness == string(catalog.Pi) || opts.Harness == string(catalog.Claude) {
			var err error
			instructionPath, cleanup, err = createProfileArtifact(caller.RecordPath, opts.Name, opts.Profile.Instructions)
			if err != nil {
				return nil, nil, fmt.Errorf("materialize profile %q: %w", opts.Profile.Name, err)
			}
		}
		var err error
		args, err = profile.LaunchArgs(*opts.Profile, opts.Harness, instructionPath, args)
		if err != nil {
			if cleanup != nil {
				err = errors.Join(err, cleanup())
			}
			return nil, nil, fmt.Errorf("prepare profile %q: %w", opts.Profile.Name, err)
		}
	}
	if opts.Model != "" {
		args = append([]string{"--model", opts.Model}, args...)
	}
	return args, cleanup, nil
}

// validateSpawn checks the options in isolation and returns the pane label.
func validateSpawn(opts SpawnOptions) (string, error) {
	if !namePattern.MatchString(opts.Name) {
		return "", fmt.Errorf("name %q must match %s", opts.Name, namePattern)
	}
	if _, err := catalog.ParseHarness(opts.Harness); err != nil {
		return "", err
	}
	if opts.InitialPrompt != nil {
		if err := ValidateInitialPrompt(*opts.InitialPrompt); err != nil {
			return "", err
		}
	}

	placements := 0
	for _, placement := range []string{opts.Workspace, opts.Tab, opts.Pane} {
		if placement != "" {
			placements++
		}
	}
	if placements > 1 {
		return "", fmt.Errorf("at most one of workspace, tab, and pane may be set")
	}
	if opts.Split != "" && opts.Split != defaultSplitDirection && opts.Split != "down" {
		return "", fmt.Errorf("split %q must be right or down", opts.Split)
	}
	if opts.Split != "" && opts.Tab == "" && opts.Pane == "" {
		return "", fmt.Errorf("split applies to tab and pane placement only")
	}
	if opts.Ratio != nil && opts.Tab == "" && opts.Pane == "" {
		return "", fmt.Errorf("ratio applies to tab and pane placement only")
	}

	if opts.Label != "" {
		return opts.Label, nil
	}
	return opts.Name, nil
}

// placePane creates or splits the pane the agent will run in.
func placePane(ctx context.Context, h Herder, caller Caller, opts SpawnOptions, label string) (herdr.Pane, error) {
	switch {
	case opts.Pane != "":
		return h.SplitPane(ctx, splitOptions(opts, opts.Pane))
	case opts.Tab != "":
		host, err := tabHostPane(ctx, h, opts.Tab)
		if err != nil {
			return herdr.Pane{}, err
		}
		return h.SplitPane(ctx, splitOptions(opts, host.ID))
	case opts.Workspace == "new":
		created, err := h.CreateWorkspace(ctx, label)
		if err != nil {
			return herdr.Pane{}, err
		}
		return created.RootPane, nil
	case opts.Workspace != "":
		return rootPaneOfNewTab(ctx, h, opts.Workspace, label)
	default:
		workspace := caller.WorkspaceID
		if workspace == "" {
			focused, err := focusedWorkspace(ctx, h)
			if err != nil {
				return herdr.Pane{}, err
			}
			workspace = focused
		}
		return rootPaneOfNewTab(ctx, h, workspace, label)
	}
}

func splitOptions(opts SpawnOptions, paneID string) herdr.SplitOptions {
	direction := opts.Split
	if direction == "" {
		direction = defaultSplitDirection
	}
	return herdr.SplitOptions{PaneID: paneID, Direction: direction, Ratio: opts.Ratio}
}

func rootPaneOfNewTab(ctx context.Context, h Herder, workspaceID, label string) (herdr.Pane, error) {
	created, err := h.CreateTab(ctx, workspaceID, label)
	if err != nil {
		return herdr.Pane{}, err
	}
	return created.RootPane, nil
}

// tabHostPane returns the pane to split for a tab, preferring its focused pane.
func tabHostPane(ctx context.Context, h Herder, tabID string) (herdr.Pane, error) {
	panes, err := h.Panes(ctx, "")
	if err != nil {
		return herdr.Pane{}, err
	}

	var first *herdr.Pane
	for i, pane := range panes {
		if pane.TabID != tabID {
			continue
		}
		if pane.Focused {
			return pane, nil
		}
		if first == nil {
			first = &panes[i]
		}
	}
	if first == nil {
		return herdr.Pane{}, fmt.Errorf("tab %q has no panes", tabID)
	}
	return *first, nil
}

func focusedWorkspace(ctx context.Context, h Herder) (string, error) {
	workspaces, err := h.Workspaces(ctx)
	if err != nil {
		return "", err
	}
	for _, workspace := range workspaces {
		if workspace.Focused {
			return workspace.ID, nil
		}
	}
	return "", fmt.Errorf("no focused workspace")
}
