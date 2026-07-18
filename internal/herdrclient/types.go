// Herdr socket API types — target Herdr v0.7.4, socket protocol v15.
//
// PROVENANCE: hand-authored from docs/reference/integration-surfaces.md
// (research snapshot 2026-07-17). NOT yet regenerated from a live binary:
// the Stage 0 build environment had no Herdr install and no network route to
// its distribution points (docs/DECISIONS.md). Regenerate/verify with:
//
//	scripts/gen-herdr-types.sh
//
// which dumps `herdr api schema --json`, checks every method named below
// against the dump, and records the clear_agent_authority/release_agent
// schema-coverage finding. Until that has run, treat exact field names as
// unverified; unknown fields pass through generically per Herdr's own
// compatibility guidance.
package herdrclient

import (
	"context"
	"encoding/json"
)

// Method names (dot notation), from the documented v0.7.4 surface. Only the
// methods Fledge needs are typed first-class; the rest of the surface can be
// reached via Client.Call directly.
const (
	MethodPing            = "ping"
	MethodSessionSnapshot = "session.snapshot"

	MethodEventsSubscribe = "events.subscribe"
	MethodEventsWait      = "events.wait"

	MethodPaneSplit               = "pane.split"
	MethodPaneList                = "pane.list"
	MethodPaneGet                 = "pane.get"
	MethodPaneRead                = "pane.read"
	MethodPaneSendText            = "pane.send_text"
	MethodPaneSendKeys            = "pane.send_keys"
	MethodPaneSendInput           = "pane.send_input"
	MethodPaneReportAgent         = "pane.report_agent"
	MethodPaneReportAgentSession  = "pane.report_agent_session"
	MethodPaneReportMetadata      = "pane.report_metadata"
	MethodPaneClearAgentAuthority = "pane.clear_agent_authority"
	MethodPaneReleaseAgent        = "pane.release_agent"
	MethodPaneWaitForOutput       = "pane.wait_for_output"
	MethodPaneClose               = "pane.close"

	MethodAgentStart   = "agent.start"
	MethodAgentList    = "agent.list"
	MethodAgentGet     = "agent.get"
	MethodAgentRead    = "agent.read"
	MethodAgentExplain = "agent.explain"
	MethodAgentSend    = "agent.send"

	MethodWorktreeCreate = "worktree.create"
)

// Event types Fledge consumes as input signals (never as truth).
const (
	EventPaneCreated            = "pane.created"
	EventPaneUpdated            = "pane.updated"
	EventPaneClosed             = "pane.closed"
	EventPaneExited             = "pane.exited"
	EventPaneAgentDetected      = "pane.agent_detected"
	EventPaneAgentStatusChanged = "pane.agent_status_changed"
	EventPaneOutputMatched      = "pane.output_matched"
	EventLayoutUpdated          = "layout.updated"
)

// Snapshot is the session.snapshot bootstrap: version/protocol metadata,
// focus, and resource records. Records are kept raw; consumers decode what
// they need and ignore the rest.
type Snapshot struct {
	Version         string            `json:"version"`
	ProtocolVersion int               `json:"protocol_version"`
	FocusedPane     string            `json:"focused_pane_id"`
	Workspaces      []json.RawMessage `json:"workspaces"`
	Tabs            []json.RawMessage `json:"tabs"`
	Panes           []json.RawMessage `json:"panes"`
	Agents          []json.RawMessage `json:"agents"`
}

// AgentInfo is the subset of an agent/pane record the harnesses read.
type AgentInfo struct {
	PaneID  string `json:"pane_id"`
	Name    string `json:"name"`
	Agent   string `json:"agent"`
	State   string `json:"state"`
	Source  string `json:"source"`
	Session string `json:"session_id"`
}

// AgentExplain is the per-pane authority explanation. The pivotal field for
// EXP1 is ScreenDetectionSkipReason: non-empty means a lifecycle authority
// has made screen rules non-authoritative for the pane.
type AgentExplain struct {
	PaneID                    string `json:"pane_id"`
	State                     string `json:"state"`
	Authority                 string `json:"authority"`
	Source                    string `json:"source"`
	ScreenDetectionSkipReason string `json:"screen_detection_skip_reason"`
}

// ReportAgentParams — pane.report_agent seizes lifecycle authority for the
// pane when Source is custom:* (suppressing screen-manifest detection).
// Seq must be monotonic per source: reports with seq <= the last accepted
// are accepted by the API but ignored by pane state.
type ReportAgentParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source"`
	Agent  string `json:"agent,omitempty"`
	State  string `json:"state"`
	Seq    int    `json:"seq,omitempty"`
}

// ReportMetadataParams — display-only; does NOT take lifecycle authority.
type ReportMetadataParams struct {
	PaneID string            `json:"pane_id"`
	Source string            `json:"source"`
	Seq    int               `json:"seq,omitempty"`
	Tokens map[string]string `json:"tokens,omitempty"`
}

// ClearAgentAuthorityParams — clears hook authority; with Source empty it
// clears any authority, and fallback (screen) detection resumes.
type ClearAgentAuthorityParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source,omitempty"`
}

// ReleaseAgentParams — clean-exit signal: drops the pane's agent identity
// immediately instead of waiting for fallback.
type ReleaseAgentParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source"`
	Agent  string `json:"agent"`
}

// SendInputParams — text plus encoded keypresses in one request. This is the
// "text + real Enter" path required for interactive Claude panes (the Ink
// TUI does not treat programmatic \r as submit).
type SendInputParams struct {
	PaneID string   `json:"pane_id"`
	Text   string   `json:"text,omitempty"`
	Keys   []string `json:"keys,omitempty"`
}

// ReadParams — pane.read. Source: visible | recent | recent-unwrapped |
// detection (the exact bottom-buffer snapshot screen detection uses).
type ReadParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source,omitempty"`
	Lines  int    `json:"lines,omitempty"`
}

// ReadResult holds pane text output.
type ReadResult struct {
	Text  string   `json:"text"`
	Lines []string `json:"lines"`
}

// AgentStartParams — agent.start: run a command in a new pane as a named,
// waitable agent target.
type AgentStartParams struct {
	Name    string   `json:"name"`
	Cwd     string   `json:"cwd,omitempty"`
	Split   string   `json:"split,omitempty"`
	Command []string `json:"command"`
}

// AgentStartResult — the spawned pane/agent identifiers.
type AgentStartResult struct {
	PaneID string `json:"pane_id"`
	Name   string `json:"name"`
}

// SubscribeParams — events.subscribe; the connection stays open and event
// lines follow on the same socket.
type SubscribeParams struct {
	Topics []string `json:"topics,omitempty"`
}

// Typed convenience wrappers over Client.Call.

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, MethodPing, nil)
	return err
}

func (c *Client) SessionSnapshot(ctx context.Context) (*Snapshot, error) {
	resp, err := c.Call(ctx, MethodSessionSnapshot, nil)
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := resp.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// EventsSubscribe registers for events; consume them via Events().
func (c *Client) EventsSubscribe(ctx context.Context, topics ...string) error {
	_, err := c.Call(ctx, MethodEventsSubscribe, SubscribeParams{Topics: topics})
	return err
}

func (c *Client) PaneRead(ctx context.Context, p ReadParams) (*ReadResult, *Response, error) {
	resp, err := c.Call(ctx, MethodPaneRead, p)
	if err != nil {
		return nil, resp, err
	}
	var r ReadResult
	if err := resp.Decode(&r); err != nil {
		return nil, resp, err
	}
	return &r, resp, nil
}

func (c *Client) PaneSendInput(ctx context.Context, p SendInputParams) error {
	_, err := c.Call(ctx, MethodPaneSendInput, p)
	return err
}

func (c *Client) PaneSendKeys(ctx context.Context, paneID string, keys ...string) error {
	_, err := c.Call(ctx, MethodPaneSendKeys, SendInputParams{PaneID: paneID, Keys: keys})
	return err
}

func (c *Client) PaneReportAgent(ctx context.Context, p ReportAgentParams) error {
	_, err := c.Call(ctx, MethodPaneReportAgent, p)
	return err
}

func (c *Client) PaneReportMetadata(ctx context.Context, p ReportMetadataParams) error {
	_, err := c.Call(ctx, MethodPaneReportMetadata, p)
	return err
}

func (c *Client) PaneClearAgentAuthority(ctx context.Context, p ClearAgentAuthorityParams) error {
	_, err := c.Call(ctx, MethodPaneClearAgentAuthority, p)
	return err
}

func (c *Client) PaneReleaseAgent(ctx context.Context, p ReleaseAgentParams) error {
	_, err := c.Call(ctx, MethodPaneReleaseAgent, p)
	return err
}

func (c *Client) PaneClose(ctx context.Context, paneID string) error {
	_, err := c.Call(ctx, MethodPaneClose, map[string]string{"pane_id": paneID})
	return err
}

func (c *Client) AgentStart(ctx context.Context, p AgentStartParams) (*AgentStartResult, *Response, error) {
	resp, err := c.Call(ctx, MethodAgentStart, p)
	if err != nil {
		return nil, resp, err
	}
	var r AgentStartResult
	if err := resp.Decode(&r); err != nil {
		return nil, resp, err
	}
	return &r, resp, nil
}

// AgentExplainPane returns the authority explanation plus the raw response
// (the raw line is what experiment reports record verbatim).
func (c *Client) AgentExplainPane(ctx context.Context, paneID string) (*AgentExplain, *Response, error) {
	resp, err := c.Call(ctx, MethodAgentExplain, map[string]string{"pane_id": paneID})
	if err != nil {
		return nil, resp, err
	}
	var e AgentExplain
	if err := resp.Decode(&e); err != nil {
		return nil, resp, err
	}
	return &e, resp, nil
}

func (c *Client) AgentGet(ctx context.Context, paneID string) (*AgentInfo, *Response, error) {
	resp, err := c.Call(ctx, MethodAgentGet, map[string]string{"pane_id": paneID})
	if err != nil {
		return nil, resp, err
	}
	var a AgentInfo
	if err := resp.Decode(&a); err != nil {
		return nil, resp, err
	}
	return &a, resp, nil
}
