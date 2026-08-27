package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fledge/internal/subprocess"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Error is a structured failure reported by the Herder CLI on stderr.
type Error struct {
	Operation string
	Code      string
	Message   string
}

type contextError struct {
	err   error
	cause error
}

func (e *contextError) Error() string { return e.err.Error() }

func (e *contextError) Unwrap() error { return e.err }

// ContextCause reports the context error observed immediately after a Herdr
// subprocess failed. It is nil when the subprocess itself was not cancelled.
func ContextCause(err error) error {
	var reported interface{ ContextCause() error }
	if errors.As(err, &reported) {
		return reported.ContextCause()
	}
	return nil
}

func (e *contextError) ContextCause() error { return e.cause }

func withContextCause(err, cause error) error {
	if cause == nil {
		return err
	}
	return &contextError{err: err, cause: cause}
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Operation, e.Code, e.Message)
}

// Status reports whether the Herder server for the selected session is running.
type Status struct {
	Running bool
}

// Workspace is one top-level Herder container.
type Workspace struct {
	ID          string `json:"workspace_id"`
	Label       string `json:"label"`
	Number      int    `json:"number"`
	Focused     bool   `json:"focused"`
	ActiveTabID string `json:"active_tab_id"`
}

// Tab is one tab within a workspace.
type Tab struct {
	ID          string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Number      int    `json:"number"`
	Focused     bool   `json:"focused"`
}

// Pane is one terminal cell within a tab.
type Pane struct {
	ID          string `json:"pane_id"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	Focused     bool   `json:"focused"`
	CWD         string `json:"cwd"`
	AgentKind   string `json:"agent"`
	AgentStatus string `json:"agent_status"`
}

// Agent is a coding agent Herder recognizes inside a pane. Name is empty when
// the agent was started outside Herder and carries no assigned name.
type Agent struct {
	Name        string `json:"name"`
	Kind        string `json:"agent"`
	Status      string `json:"agent_status"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	PaneID      string `json:"pane_id"`
}

// WorkspaceCreated is the workspace, root tab, and root pane created together.
type WorkspaceCreated struct {
	Workspace Workspace `json:"workspace"`
	Tab       Tab       `json:"tab"`
	RootPane  Pane      `json:"root_pane"`
}

// TabCreated is the tab and its root pane created together.
type TabCreated struct {
	Tab      Tab  `json:"tab"`
	RootPane Pane `json:"root_pane"`
}

// SplitOptions selects the pane to split and how. Ratio is nil for Herder's
// default split ratio.
type SplitOptions struct {
	PaneID    string
	Direction string
	Ratio     *float64
}

// StartAgentOptions describes an agent launch in an existing shell pane. Args
// are passed to the agent itself and TimeoutMS is omitted when zero.
type StartAgentOptions struct {
	Name      string
	Kind      string
	PaneID    string
	Args      []string
	TimeoutMS int
}

// PromptOptions describes prompt submission. Wait blocks until the agent
// settles, optionally restricted to the Until states, and TimeoutMS is omitted
// when zero.
type PromptOptions struct {
	Target    string
	Text      string
	Wait      bool
	Until     []string
	TimeoutMS int
}

// WithSession returns a copy of the client that addresses the named session. An
// empty name relies on $HERDR_SOCKET_PATH, which Herder injects into its panes.
func (c *Client) WithSession(name string) *Client {
	session := *c
	session.session = name
	return &session
}

// Status reports whether the session's Herder server is running.
func (c *Client) Status(ctx context.Context) (Status, error) {
	stdout, operation, err := c.run(ctx, "status", "--json")
	if err != nil {
		return Status{}, err
	}

	var payload struct {
		Server struct {
			Running bool `json:"running"`
		} `json:"server"`
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	if err := decoder.Decode(&payload); err != nil {
		return Status{}, fmt.Errorf("%s: decode output: %w", operation, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Status{}, fmt.Errorf("%s: decode output: %w", operation, err)
	}
	return Status{Running: payload.Server.Running}, nil
}

// Workspaces returns every workspace in display order.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	var payload struct {
		Workspaces []Workspace `json:"workspaces"`
	}
	if err := c.invoke(ctx, "workspace_list", &payload, "workspace", "list"); err != nil {
		return nil, err
	}
	for _, workspace := range payload.Workspaces {
		if err := validateWorkspace(workspace); err != nil {
			return nil, err
		}
	}
	return payload.Workspaces, nil
}

// CreateWorkspace creates an unfocused workspace with its root tab and pane.
func (c *Client) CreateWorkspace(ctx context.Context, label string) (WorkspaceCreated, error) {
	var created WorkspaceCreated
	if err := c.invoke(ctx, "workspace_created", &created, "workspace", "create", "--label", label, "--no-focus"); err != nil {
		return WorkspaceCreated{}, err
	}
	if err := validateWorkspace(created.Workspace); err != nil {
		return WorkspaceCreated{}, err
	}
	if err := validateTab(created.Tab); err != nil {
		return WorkspaceCreated{}, err
	}
	if err := validatePane(created.RootPane); err != nil {
		return WorkspaceCreated{}, err
	}
	return created, nil
}

// RenameWorkspace changes a workspace's display label.
func (c *Client) RenameWorkspace(ctx context.Context, id, label string) error {
	return c.invoke(ctx, "workspace_info", nil, "workspace", "rename", id, label)
}

// Tabs returns the tabs of one workspace, or of every workspace when
// workspaceID is empty.
func (c *Client) Tabs(ctx context.Context, workspaceID string) ([]Tab, error) {
	args := []string{"tab", "list"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}

	var payload struct {
		Tabs []Tab `json:"tabs"`
	}
	if err := c.invoke(ctx, "tab_list", &payload, args...); err != nil {
		return nil, err
	}
	for _, tab := range payload.Tabs {
		if err := validateTab(tab); err != nil {
			return nil, err
		}
	}
	return payload.Tabs, nil
}

// CreateTab creates an unfocused tab and its root pane in a workspace.
func (c *Client) CreateTab(ctx context.Context, workspaceID, label string) (TabCreated, error) {
	var created TabCreated
	if err := c.invoke(ctx, "tab_created", &created, "tab", "create", "--workspace", workspaceID, "--label", label, "--no-focus"); err != nil {
		return TabCreated{}, err
	}
	if err := validateTab(created.Tab); err != nil {
		return TabCreated{}, err
	}
	if err := validatePane(created.RootPane); err != nil {
		return TabCreated{}, err
	}
	return created, nil
}

// RenameTab changes a tab's label.
func (c *Client) RenameTab(ctx context.Context, id, label string) error {
	return c.invoke(ctx, "tab_info", nil, "tab", "rename", id, label)
}

// Panes returns the panes of one workspace, or of every workspace when
// workspaceID is empty.
func (c *Client) Panes(ctx context.Context, workspaceID string) ([]Pane, error) {
	args := []string{"pane", "list"}
	if workspaceID != "" {
		args = append(args, "--workspace", workspaceID)
	}

	var payload struct {
		Panes []Pane `json:"panes"`
	}
	if err := c.invoke(ctx, "pane_list", &payload, args...); err != nil {
		return nil, err
	}
	for _, pane := range payload.Panes {
		if err := validatePane(pane); err != nil {
			return nil, err
		}
	}
	return payload.Panes, nil
}

// CurrentPane returns the live pane corresponding to the invoking terminal.
func (c *Client) CurrentPane(ctx context.Context) (Pane, error) {
	var payload struct {
		Pane Pane `json:"pane"`
	}
	if err := c.invoke(ctx, "pane_current", &payload, "pane", "current", "--current"); err != nil {
		return Pane{}, err
	}
	if err := validatePane(payload.Pane); err != nil {
		return Pane{}, err
	}
	return payload.Pane, nil
}

// SplitPane splits a pane and returns the new, unfocused sibling pane.
func (c *Client) SplitPane(ctx context.Context, options SplitOptions) (Pane, error) {
	args := []string{"pane", "split", "--pane", options.PaneID, "--direction", options.Direction}
	if options.Ratio != nil {
		args = append(args, "--ratio", strconv.FormatFloat(*options.Ratio, 'f', -1, 64))
	}
	args = append(args, "--no-focus")

	var payload struct {
		Pane Pane `json:"pane"`
	}
	if err := c.invoke(ctx, "pane_info", &payload, args...); err != nil {
		return Pane{}, err
	}
	if err := validatePane(payload.Pane); err != nil {
		return Pane{}, err
	}
	return payload.Pane, nil
}

// ClosePane closes a pane and its terminal.
func (c *Client) ClosePane(ctx context.Context, id string) error {
	return c.invoke(ctx, "ok", nil, "pane", "close", id)
}

// StartAgent starts an agent in a pane already sitting at a shell prompt.
func (c *Client) StartAgent(ctx context.Context, options StartAgentOptions) (Agent, error) {
	args := []string{"agent", "start", options.Name, "--kind", options.Kind, "--pane", options.PaneID}
	if options.TimeoutMS != 0 {
		args = append(args, "--timeout", strconv.Itoa(options.TimeoutMS))
	}
	if len(options.Args) > 0 {
		args = append(args, "--")
		args = append(args, options.Args...)
	}

	var payload struct {
		Agent Agent `json:"agent"`
	}
	if err := c.invoke(ctx, "agent_started", &payload, args...); err != nil {
		return Agent{}, err
	}
	if err := validateAgent(payload.Agent); err != nil {
		return Agent{}, err
	}
	return payload.Agent, nil
}

// PromptAgent submits prompt text to an agent and returns the raw result
// object, which carries the agent state observed after any requested wait.
func (c *Client) PromptAgent(ctx context.Context, options PromptOptions) (json.RawMessage, error) {
	args := []string{"agent", "prompt", options.Target, options.Text}
	if options.Wait {
		args = append(args, "--wait")
	}
	for _, until := range options.Until {
		args = append(args, "--until", until)
	}
	if options.TimeoutMS != 0 {
		args = append(args, "--timeout", strconv.Itoa(options.TimeoutMS))
	}

	var result json.RawMessage
	if err := c.invoke(ctx, "agent_prompted", &result, args...); err != nil {
		return nil, err
	}
	return result, nil
}

// Agents returns every live agent across all workspaces.
func (c *Client) Agents(ctx context.Context) ([]Agent, error) {
	var payload struct {
		Agents []Agent `json:"agents"`
	}
	if err := c.invoke(ctx, "agent_list", &payload, "agent", "list"); err != nil {
		return nil, err
	}
	for _, agent := range payload.Agents {
		if err := validateAgent(agent); err != nil {
			return nil, err
		}
	}
	return payload.Agents, nil
}

// GetAgent returns one agent addressed by name or by hosting pane ID.
func (c *Client) GetAgent(ctx context.Context, target string) (Agent, error) {
	var payload struct {
		Agent Agent `json:"agent"`
	}
	if err := c.invoke(ctx, "agent_info", &payload, "agent", "get", target); err != nil {
		return Agent{}, err
	}
	if err := validateAgent(payload.Agent); err != nil {
		return Agent{}, err
	}
	return payload.Agent, nil
}

// Invoke runs an arbitrary Herder subcommand and returns its raw result object,
// for the many methods this package does not wrap.
func (c *Client) Invoke(ctx context.Context, args ...string) (json.RawMessage, error) {
	var result json.RawMessage
	if err := c.invoke(ctx, "", &result, args...); err != nil {
		return nil, err
	}
	return result, nil
}

// invoke runs a Herder subcommand that prints the socket result envelope,
// decoding the result into out. A non-empty expectType is checked against the
// result's discriminant, and out may be nil when only the error matters.
func (c *Client) invoke(ctx context.Context, expectType string, out any, args ...string) error {
	stdout, operation, err := c.run(ctx, args...)
	if err != nil {
		return err
	}

	var payload struct {
		Result json.RawMessage `json:"result"`
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("%s: decode output: %w", operation, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%s: decode output: %w", operation, err)
	}
	if len(payload.Result) == 0 || string(payload.Result) == "null" {
		return fmt.Errorf("%s: decode output: missing result object", operation)
	}

	if expectType != "" {
		var discriminant struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload.Result, &discriminant); err != nil {
			return fmt.Errorf("%s: decode result: %w", operation, err)
		}
		if discriminant.Type != expectType {
			return fmt.Errorf("%s: result type %q, want %q", operation, discriminant.Type, expectType)
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload.Result, out); err != nil {
		return fmt.Errorf("%s: decode result: %w", operation, err)
	}
	return nil
}

// run executes one Herder subcommand against the selected session and returns
// its stdout alongside the operation description used in errors.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, string, error) {
	argv := args
	if c.session != "" {
		argv = append([]string{"--session", c.session}, args...)
	}
	operation := "herdr " + strings.Join(argv, " ")

	executable := "herdr"
	if c.session == "" && os.Getenv("HERDR_ENV") == "1" {
		if injected := os.Getenv("HERDR_BIN_PATH"); injected != "" {
			executable = injected
		}
	}
	cmd := subprocess.CommandContext(ctx, executable, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		cause := ctx.Err()
		if reported := decodeError(operation, stderr.Bytes()); reported != nil {
			return nil, operation, withContextCause(reported, cause)
		}
		return nil, operation, withContextCause(commandError(operation, err, stderr.String()), cause)
	}
	return stdout.Bytes(), operation, nil
}

// decodeError reads Herder's JSON error envelope, returning nil when stderr
// carries something else.
func decodeError(operation string, stderr []byte) *Error {
	var payload struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stderr), &payload); err != nil || payload.Error == nil {
		return nil
	}
	return &Error{Operation: operation, Code: payload.Error.Code, Message: payload.Error.Message}
}

func validateWorkspace(workspace Workspace) error {
	if workspace.ID == "" {
		return errors.New("herdr workspace: missing workspace_id")
	}
	return nil
}

func validateTab(tab Tab) error {
	if tab.ID == "" || tab.WorkspaceID == "" {
		return errors.New("herdr tab: missing tab_id or workspace_id")
	}
	return nil
}

func validatePane(pane Pane) error {
	if pane.ID == "" || pane.WorkspaceID == "" || pane.TabID == "" {
		return errors.New("herdr pane: missing pane_id, workspace_id, or tab_id")
	}
	return nil
}

func validateAgent(agent Agent) error {
	if agent.PaneID == "" || agent.WorkspaceID == "" || agent.TabID == "" {
		return errors.New("herdr agent: missing pane_id, workspace_id, or tab_id")
	}
	return nil
}
