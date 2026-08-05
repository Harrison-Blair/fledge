// Package herdr adapts the Herdr CLI to the operations Fledge needs.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Session is a named Herdr session returned by the Herdr CLI.
type Session struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	SocketPath string `json:"socket_path"`
}

// Snapshot is the live layout and agent state for one Herdr session.
type Snapshot struct {
	FocusedWorkspaceID string      `json:"focused_workspace_id"`
	FocusedTabID       string      `json:"focused_tab_id"`
	FocusedPaneID      string      `json:"focused_pane_id"`
	Workspaces         []Workspace `json:"workspaces"`
	Tabs               []Tab       `json:"tabs"`
	Panes              []Pane      `json:"panes"`
	Agents             []Agent     `json:"agents"`
}

type Workspace struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

type Tab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

type Pane struct {
	PaneID      string  `json:"pane_id"`
	TabID       string  `json:"tab_id"`
	WorkspaceID string  `json:"workspace_id"`
	Label       *string `json:"label"`
	Agent       *string `json:"agent"`
	AgentStatus string  `json:"agent_status"`
}

type Agent struct {
	Name         *string       `json:"name"`
	Agent        *string       `json:"agent"`
	PaneID       string        `json:"pane_id"`
	TabID        string        `json:"tab_id"`
	WorkspaceID  string        `json:"workspace_id"`
	AgentStatus  string        `json:"agent_status"`
	AgentSession *AgentSession `json:"agent_session"`
	Revision     int           `json:"revision"`
}

// AgentSession is Herdr's exact correlation between a pane's agent and the
// harness's own native session. Kind is "id" or "path"; Value is the harness
// session identifier Fledge uses to locate the agent's transcript.
type AgentSession struct {
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Value  string `json:"value"`
}

// Client invokes a Herdr executable.
type Client struct {
	binary string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time
}

const initialLayoutSettleTime = 100 * time.Millisecond

// NewClient creates a Herdr CLI client.
func NewClient(binary string, stdin io.Reader, stdout, stderr io.Writer) *Client {
	return &Client{
		binary: binary,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		now:    time.Now,
	}
}

// Check verifies that the Herdr executable can be found.
func (c *Client) Check() error {
	if _, err := exec.LookPath(c.binary); err != nil {
		return fmt.Errorf("find %q on PATH: %w", c.binary, err)
	}

	return nil
}

// Attach launches or attaches to name with its initial working directory set
// to dir.
func (c *Client) Attach(ctx context.Context, name, dir string) error {
	command := exec.CommandContext(ctx, c.binary, "--session", name)
	command.Dir = dir
	command.Stdin = c.stdin
	command.Stdout = c.stdout
	command.Stderr = c.stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("attach to Herdr session %q: %w", name, err)
	}

	return nil
}

// StartServer starts a named Herdr server without attaching a UI. The server
// owns its own lifetime after the command has successfully started.
func (c *Client) StartServer(name, dir string, environment map[string]string) error {
	command := exec.Command(c.binary, "--session", name, "server")
	command.Dir = dir
	command.Env = replaceEnvironment(os.Environ(), environment)

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open null device: %w", err)
	}
	defer devNull.Close()
	command.Stdin = devNull
	command.Stdout = devNull
	command.Stderr = devNull

	if err := command.Start(); err != nil {
		return fmt.Errorf("start Herdr server %q: %w", name, err)
	}
	if err := command.Process.Release(); err != nil {
		killErr := command.Process.Kill()
		return errors.Join(fmt.Errorf("release Herdr server %q: %w", name, err), killErr)
	}
	return nil
}

// WaitReady polls the named server until its initial layout can be read or the
// server remains layout-empty long enough to require explicit initialization.
func (c *Client) WaitReady(ctx context.Context, name string, timeout time.Duration) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	var emptySince time.Time
	for {
		if snapshot, err := c.Snapshot(ctx, name); err == nil {
			if len(snapshot.Tabs) > 0 && len(snapshot.Panes) > 0 {
				return snapshot, nil
			}
			if emptySince.IsZero() {
				emptySince = c.now()
			} else if c.now().Sub(emptySince) >= initialLayoutSettleTime {
				return snapshot, nil
			}
			lastErr = errors.New("snapshot has no initial tab and pane")
		} else {
			// A failed poll says nothing about the layout, so the settle window
			// must restart from the next empty snapshot.
			emptySince = time.Time{}
			if ctx.Err() == nil {
				lastErr = err
			}
		}

		select {
		case <-ctx.Done():
			return Snapshot{}, fmt.Errorf("wait for Herdr session %q readiness: %w", name, errors.Join(ctx.Err(), lastErr))
		case <-ticker.C:
		}
	}
}

// CreateWorkspace creates a workspace and its initial tab and pane.
func (c *Client) CreateWorkspace(ctx context.Context, session, dir, label string) (Workspace, Tab, Pane, error) {
	var response struct {
		Result struct {
			Type      string    `json:"type"`
			Workspace Workspace `json:"workspace"`
			Tab       Tab       `json:"tab"`
			RootPane  Pane      `json:"root_pane"`
		} `json:"result"`
	}
	args := []string{"workspace", "create", "--cwd", dir, "--label", label, "--no-focus"}
	if err := c.runSessionJSON(ctx, session, &response, args...); err != nil {
		return Workspace{}, Tab{}, Pane{}, fmt.Errorf("create Herdr workspace %q: %w", label, err)
	}
	if response.Result.Workspace.WorkspaceID == "" || response.Result.Tab.TabID == "" || response.Result.RootPane.PaneID == "" {
		return Workspace{}, Tab{}, Pane{}, fmt.Errorf("create Herdr workspace %q: response is missing workspace_id, tab_id, or root pane_id", label)
	}
	return response.Result.Workspace, response.Result.Tab, response.Result.RootPane, nil
}

// Snapshot returns the live state of a named session.
func (c *Client) Snapshot(ctx context.Context, session string) (Snapshot, error) {
	var response struct {
		Result struct {
			Type     string   `json:"type"`
			Snapshot Snapshot `json:"snapshot"`
		} `json:"result"`
	}
	if err := c.runSessionJSON(ctx, session, &response, "api", "snapshot"); err != nil {
		return Snapshot{}, fmt.Errorf("read Herdr session %q snapshot: %w", session, err)
	}
	if response.Result.Type != "session_snapshot" {
		return Snapshot{}, fmt.Errorf("read Herdr session %q snapshot: unexpected response type %q", session, response.Result.Type)
	}
	return response.Result.Snapshot, nil
}

func (c *Client) RenameTab(ctx context.Context, session, tabID, label string) error {
	if err := c.runSessionJSON(ctx, session, nil, "tab", "rename", tabID, label); err != nil {
		return fmt.Errorf("rename Herdr tab %q: %w", tabID, err)
	}
	return nil
}

func (c *Client) RenamePane(ctx context.Context, session, paneID, label string) error {
	if err := c.runSessionJSON(ctx, session, nil, "pane", "rename", paneID, label); err != nil {
		return fmt.Errorf("rename Herdr pane %q: %w", paneID, err)
	}
	return nil
}

func (c *Client) SplitPane(ctx context.Context, session, paneID, dir string, environment map[string]string) (Pane, error) {
	var response struct {
		Result struct {
			Type string `json:"type"`
			Pane Pane   `json:"pane"`
		} `json:"result"`
	}
	args := []string{"pane", "split", paneID, "--direction", "right", "--ratio", "0.5", "--cwd", dir}
	args = appendEnvironmentArgs(args, environment)
	args = append(args, "--no-focus")
	if err := c.runSessionJSON(ctx, session, &response, args...); err != nil {
		return Pane{}, fmt.Errorf("split Herdr pane %q: %w", paneID, err)
	}
	if response.Result.Pane.PaneID == "" {
		return Pane{}, fmt.Errorf("split Herdr pane %q: response is missing pane_id", paneID)
	}
	return response.Result.Pane, nil
}

func (c *Client) CreateTab(ctx context.Context, session, workspaceID, dir, label string, environment map[string]string) (Tab, Pane, error) {
	var response struct {
		Result struct {
			Type     string `json:"type"`
			Tab      Tab    `json:"tab"`
			RootPane Pane   `json:"root_pane"`
		} `json:"result"`
	}
	args := []string{"tab", "create", "--workspace", workspaceID, "--cwd", dir, "--label", label}
	args = appendEnvironmentArgs(args, environment)
	args = append(args, "--no-focus")
	if err := c.runSessionJSON(ctx, session, &response, args...); err != nil {
		return Tab{}, Pane{}, fmt.Errorf("create Herdr tab %q: %w", label, err)
	}
	if response.Result.Tab.TabID == "" || response.Result.RootPane.PaneID == "" {
		return Tab{}, Pane{}, fmt.Errorf("create Herdr tab %q: response is missing tab_id or root pane_id", label)
	}
	return response.Result.Tab, response.Result.RootPane, nil
}

func (c *Client) CloseTab(ctx context.Context, session, tabID string) error {
	if err := c.runSessionJSON(ctx, session, nil, "tab", "close", tabID); err != nil {
		return fmt.Errorf("close Herdr tab %q: %w", tabID, err)
	}
	return nil
}

func (c *Client) ClosePane(ctx context.Context, session, paneID string) error {
	if err := c.runSessionJSON(ctx, session, nil, "pane", "close", paneID); err != nil {
		return fmt.Errorf("close Herdr pane %q: %w", paneID, err)
	}
	return nil
}

func (c *Client) FocusAgent(ctx context.Context, session, target string) error {
	if err := c.runSessionJSON(ctx, session, nil, "agent", "focus", target); err != nil {
		return fmt.Errorf("focus Herdr agent %q: %w", target, err)
	}
	return nil
}

func (c *Client) StartAgent(ctx context.Context, session, name, kind, paneID string, timeout time.Duration, nativeArgs []string) error {
	args := []string{"agent", "start", name, "--kind", kind, "--pane", paneID, "--timeout", strconv.FormatInt(timeout.Milliseconds(), 10)}
	if len(nativeArgs) > 0 {
		args = append(args, "--")
		args = append(args, nativeArgs...)
	}
	if err := c.runSessionJSON(ctx, session, nil, args...); err != nil {
		return fmt.Errorf("start Herdr agent %q: %w", name, err)
	}
	return nil
}

func (c *Client) PromptAgent(ctx context.Context, session, target, prompt string) error {
	if err := c.runSessionJSON(ctx, session, nil, "agent", "prompt", target, prompt); err != nil {
		return fmt.Errorf("prompt Herdr agent %q: %w", target, err)
	}
	return nil
}

// List returns all Herdr named sessions.
func (c *Client) List(ctx context.Context) ([]Session, error) {
	var response struct {
		Sessions []Session `json:"sessions"`
	}

	if err := c.runJSON(ctx, &response, "session", "list", "--json"); err != nil {
		return nil, fmt.Errorf("list Herdr sessions: %w", err)
	}

	return response.Sessions, nil
}

// Stop stops a running named Herdr session.
func (c *Client) Stop(ctx context.Context, name string) error {
	if err := c.runJSON(ctx, nil, "session", "stop", name, "--json"); err != nil {
		return fmt.Errorf("stop Herdr session %q: %w", name, err)
	}

	return nil
}

// Delete deletes a stopped named Herdr session.
func (c *Client) Delete(ctx context.Context, name string) error {
	if err := c.runJSON(ctx, nil, "session", "delete", name, "--json"); err != nil {
		return fmt.Errorf("delete Herdr session %q: %w", name, err)
	}

	return nil
}

func (c *Client) runJSON(ctx context.Context, destination any, args ...string) error {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	command := exec.CommandContext(ctx, c.binary, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, message)
	}

	if destination == nil {
		return nil
	}
	if stdout.Len() == 0 {
		return errors.New("decode JSON response: empty response")
	}

	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}

	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("decode JSON response: unexpected trailing data")
	}

	return nil
}

func (c *Client) runSessionJSON(ctx context.Context, session string, destination any, args ...string) error {
	sessionArgs := make([]string, 0, len(args)+2)
	sessionArgs = append(sessionArgs, "--session", session)
	sessionArgs = append(sessionArgs, args...)
	return c.runJSON(ctx, destination, sessionArgs...)
}

func appendEnvironmentArgs(args []string, environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+environment[key])
	}
	return args
}

func replaceEnvironment(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := overrides[key]; !replaced {
			result = append(result, entry)
		}
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}
