package herdr

import (
	"context"
	"encoding/json"
)

type Pong struct {
	Type     string `json:"type"`
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

type AgentInfo struct {
	Agent            *string           `json:"agent"`
	AgentStatus      string            `json:"agent_status"`
	CWD              *string           `json:"cwd"`
	DisplayAgent     *string           `json:"display_agent"`
	InteractiveReady bool              `json:"interactive_ready"`
	LaunchPending    bool              `json:"launch_pending"`
	Name             *string           `json:"name"`
	PaneID           string            `json:"pane_id"`
	TabID            string            `json:"tab_id"`
	WorkspaceID      string            `json:"workspace_id"`
	StateLabels      map[string]string `json:"state_labels"`
}

type PaneInfo struct {
	Agent       *string `json:"agent"`
	AgentStatus string  `json:"agent_status"`
	CWD         *string `json:"cwd"`
	Label       *string `json:"label"`
	PaneID      string  `json:"pane_id"`
	TabID       string  `json:"tab_id"`
	WorkspaceID string  `json:"workspace_id"`
}

type TabInfo struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

type WorkspaceInfo struct {
	WorkspaceID string                 `json:"workspace_id"`
	Number      int                    `json:"number"`
	Label       string                 `json:"label"`
	Focused     bool                   `json:"focused"`
	Worktree    *WorkspaceWorktreeInfo `json:"worktree"`
}

type WorkspaceWorktreeInfo struct {
	RepoRoot     string `json:"repo_root"`
	CheckoutPath string `json:"checkout_path"`
}

type Snapshot struct {
	Version            string          `json:"version"`
	Protocol           int             `json:"protocol"`
	FocusedWorkspaceID *string         `json:"focused_workspace_id"`
	Workspaces         []WorkspaceInfo `json:"workspaces"`
	Tabs               []TabInfo       `json:"tabs"`
	Panes              []PaneInfo      `json:"panes"`
	Agents             []AgentInfo     `json:"agents"`
}

type Process struct {
	PID   int      `json:"pid"`
	Name  string   `json:"name"`
	Argv  []string `json:"argv"`
	Argv0 *string  `json:"argv0"`
}

type ProcessInfo struct {
	PaneID              string    `json:"pane_id"`
	ShellPID            *int      `json:"shell_pid"`
	ForegroundProcesses []Process `json:"foreground_processes"`
}

type Result struct {
	Type        string          `json:"type"`
	Version     string          `json:"version"`
	Protocol    int             `json:"protocol"`
	Snapshot    Snapshot        `json:"snapshot"`
	Agent       AgentInfo       `json:"agent"`
	Agents      []AgentInfo     `json:"agents"`
	Workspace   WorkspaceInfo   `json:"workspace"`
	Tab         TabInfo         `json:"tab"`
	RootPane    PaneInfo        `json:"root_pane"`
	Pane        PaneInfo        `json:"pane"`
	ProcessInfo ProcessInfo     `json:"process_info"`
	PaneID      string          `json:"pane_id"`
	Text        string          `json:"text"`
	Read        json.RawMessage `json:"read"`
}

func (c *Client) Ping(ctx context.Context) (Pong, error) {
	var r Pong
	err := c.Call(ctx, "ping", nil, &r)
	return r, err
}

func (c *Client) Snapshot(ctx context.Context) (Snapshot, error) {
	var r Result
	err := c.Call(ctx, "session.snapshot", nil, &r)
	return r.Snapshot, err
}
