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
	Type  string   `json:"type"`
	Agent Pane     `json:"agent"`
	Argv  []string `json:"argv"`
}
type AgentListResult struct {
	Type   string `json:"type"`
	Agents []Pane `json:"agents"`
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
