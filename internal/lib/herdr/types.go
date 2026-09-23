package herdr

// Pane is the placement and occupant subset shared by PaneInfo and AgentInfo.
type Pane struct {
	PaneID      string  `json:"pane_id"`
	WorkspaceID string  `json:"workspace_id"`
	TabID       string  `json:"tab_id"`
	Name        *string `json:"name"`
	Agent       *string `json:"agent"`
	AgentStatus string  `json:"agent_status"`
	Cwd         *string `json:"cwd"`
}

// Capabilities is the ServerCapabilities subset carried by a pong. Boolean flags
// default to false when the pong omits the capabilities object or a member.
type Capabilities struct {
	LiveHandoff                bool    `json:"live_handoff"`
	DetachedServerDaemon       bool    `json:"detached_server_daemon"`
	HealthCheck                bool    `json:"health_check"`
	SurfaceInterest            bool    `json:"surface_interest"`
	EndpointProtocolGeneration *uint32 `json:"endpoint_protocol_generation"`
}

// PongResult is the ping liveness and capability probe reply.
type PongResult struct {
	Type         string        `json:"type"`
	Version      string        `json:"version"`
	Protocol     uint32        `json:"protocol"`
	Capabilities *Capabilities `json:"capabilities"`
}

// IntegrationInfo describes one integration target's command and install state.
type IntegrationInfo struct {
	Target    string `json:"target"`
	Label     string `json:"label"`
	Command   string `json:"command"`
	Available bool   `json:"available"`
	State     string `json:"state"`
}

// IntegrationListResult lists every known integration target.
type IntegrationListResult struct {
	Type         string            `json:"type"`
	Integrations []IntegrationInfo `json:"integrations"`
}
type Workspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}
type Tab struct {
	ID          string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}
type Layout struct {
	TabID         string `json:"tab_id"`
	WorkspaceID   string `json:"workspace_id"`
	FocusedPaneID string `json:"focused_pane_id"`
}
type Snapshot struct {
	Workspaces []Workspace `json:"workspaces"`
	Tabs       []Tab       `json:"tabs"`
	Panes      []Pane      `json:"panes"`
	Layouts    []Layout    `json:"layouts"`
	Agents     []Pane      `json:"agents"`
}
type Worktree struct {
	Path            string  `json:"path"`
	Branch          *string `json:"branch"`
	OpenWorkspaceID *string `json:"open_workspace_id"`
}
type SnapshotResult struct {
	Type     string    `json:"type"`
	Snapshot *Snapshot `json:"snapshot"`
}
type PaneResult struct {
	Type string `json:"type"`
	Pane Pane   `json:"pane"`
}
type AgentResult struct {
	Type  string       `json:"type"`
	Agent AgentDetails `json:"agent"`
	Argv  []string     `json:"argv"`
}
type AgentListResult struct {
	Type   string         `json:"type"`
	Agents []AgentDetails `json:"agents"`
}
type TabResult struct {
	Type string `json:"type"`
	Tab  Tab    `json:"tab"`
}
type CreatedResult struct {
	Type        string    `json:"type"`
	Workspace   Workspace `json:"workspace"`
	Tab         Tab       `json:"tab"`
	RootPane    Pane      `json:"root_pane"`
	Worktree    Worktree  `json:"worktree"`
	AlreadyOpen *bool     `json:"already_open"`
}
type WorktreeListResult struct {
	Type   string `json:"type"`
	Source struct {
		RepoRoot string `json:"repo_root"`
	} `json:"source"`
	Worktrees []Worktree `json:"worktrees"`
}

// AgentDetails is the full AgentInfo carried by every agent_* result.
type AgentDetails struct {
	Pane
	TerminalID            string        `json:"terminal_id"`
	ForegroundCwd         *string       `json:"foreground_cwd"`
	InteractiveReady      *bool         `json:"interactive_ready"`
	LaunchPending         *bool         `json:"launch_pending"`
	Focused               *bool         `json:"focused"`
	Revision              *uint64       `json:"revision"`
	Title                 *string       `json:"title"`
	TerminalTitle         *string       `json:"terminal_title"`
	TerminalTitleStripped *string       `json:"terminal_title_stripped"`
	AgentSession          *AgentSession `json:"agent_session"`
}
type AgentSession struct {
	Source *string `json:"source"`
	Agent  *string `json:"agent"`
	Kind   *string `json:"kind"`
	Value  *string `json:"value"`
}

// PaneRead is a terminal snapshot carried by pane_read results. Revision and
// Truncated are pointers so a missing required field is detectable.
type PaneRead struct {
	PaneID      string  `json:"pane_id"`
	WorkspaceID string  `json:"workspace_id"`
	TabID       string  `json:"tab_id"`
	Source      string  `json:"source"`
	Format      string  `json:"format"`
	Text        string  `json:"text"`
	Revision    *uint64 `json:"revision"`
	Truncated   *bool   `json:"truncated"`
}
type PaneReadResult struct {
	Type string   `json:"type"`
	Read PaneRead `json:"read"`
}
