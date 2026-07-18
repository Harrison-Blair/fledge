// Herdr socket API types — target Herdr v0.7.4, socket protocol v16.
//
// PROVENANCE: reconciled against the live binary's `herdr api schema --json`
// (committed as herdr-schema.json) on 2026-07-17, superseding the original
// hand-authored v15 snapshot. Key protocol-16 facts baked in here and recorded
// in docs/DECISIONS.md (ADR-015):
//   - Every request carries a mandatory `params` (see client.go).
//   - Results are wrapped by kind: session.snapshot -> {snapshot:{...}},
//     agent.start/agent.get -> {agent:{...}}, pane.read -> {read:{...}}.
//   - agent.start/agent.get params key the pane as `target`, and agent.start
//     takes `argv` (not `command`).
//   - The screen-detection signal is the boolean `screen_detection_skipped`
//     on the agent record (there is no `screen_detection_skip_reason`).
//
// Regenerate/verify on every Herdr upgrade with scripts/gen-herdr-types.sh.
// Unknown fields pass through generically per Herdr's compatibility guidance.
package herdrclient

import (
	"context"
	"encoding/json"
)

// Method names (dot notation), verified present in the v16 schema dump. Only
// the methods Fledge needs are typed first-class; the rest of the surface can
// be reached via Client.Call directly.
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

// snapshotResult wraps session.snapshot's {type, snapshot:{...}} envelope.
type snapshotResult struct {
	Snapshot Snapshot `json:"snapshot"`
}

// Snapshot is the session.snapshot bootstrap: version/protocol metadata, focus,
// and resource records. Records are kept raw; consumers decode what they need
// and ignore the rest.
type Snapshot struct {
	Version         string            `json:"version"`
	ProtocolVersion int               `json:"protocol"`
	FocusedPane     string            `json:"focused_pane_id"`
	Workspaces      []json.RawMessage `json:"workspaces"`
	Tabs            []json.RawMessage `json:"tabs"`
	Panes           []json.RawMessage `json:"panes"`
	Agents          []json.RawMessage `json:"agents"`
}

// AgentInfo is the typed agent/pane record (result type agent_info), returned
// by agent.get and embedded under `agent` in agent.start and session.snapshot.
// The pivotal EXP1 field is ScreenDetectionSkipped: true means a lifecycle
// authority has made screen rules non-authoritative for the pane.
type AgentInfo struct {
	PaneID                 string `json:"pane_id"`
	Name                   string `json:"name"`
	Agent                  string `json:"agent"`
	AgentStatus            string `json:"agent_status"`
	ScreenDetectionSkipped bool   `json:"screen_detection_skipped"`
	DisplayAgent           string `json:"display_agent"`
	Revision               int    `json:"revision"`
}

// agentResult wraps the {type, agent:{...}} envelope of agent.get/agent.start.
type agentResult struct {
	Agent AgentInfo `json:"agent"`
}

// AgentExplain is agent.explain's result. In v16 the `explain` payload is an
// open, server-defined object (untyped in the schema), so it is captured raw;
// harnesses record it verbatim and the operator interprets it alongside the
// typed AgentInfo.ScreenDetectionSkipped signal.
type AgentExplain struct {
	Explain json.RawMessage `json:"explain"`
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

// ReadParams — pane.read. Source: visible | recent | recent_unwrapped |
// detection (the exact bottom-buffer snapshot screen detection uses). Note the
// underscore: protocol 16 rejects the reference doc's hyphenated form.
type ReadParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source,omitempty"`
	Lines  int    `json:"lines,omitempty"`
}

// readResult wraps pane.read's {type, read:{...}} envelope.
type readResult struct {
	Read ReadResult `json:"read"`
}

// ReadResult holds pane text output (v16 returns text plus a truncation flag;
// there is no per-line array).
type ReadResult struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// AgentStartParams — agent.start: run a command in a new pane as a named,
// waitable agent target. The command vector is `argv` (protocol 16).
type AgentStartParams struct {
	Name  string   `json:"name"`
	Cwd   string   `json:"cwd,omitempty"`
	Split string   `json:"split,omitempty"`
	Argv  []string `json:"argv"`
}

// AgentStartResult — the spawned pane/agent identifiers, lifted from the
// nested `agent` record.
type AgentStartResult struct {
	PaneID string
	Name   string
}

// SubscribeParams — events.subscribe; the connection stays open and event
// lines follow on the same socket. Subscriptions is the v16 array of match
// objects; passed through raw since Fledge does not yet type the match surface.
type SubscribeParams struct {
	Subscriptions []json.RawMessage `json:"subscriptions"`
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
	var w snapshotResult
	if err := resp.Decode(&w); err != nil {
		return nil, err
	}
	return &w.Snapshot, nil
}

// EventsSubscribe opens the streaming connection and registers the given raw
// subscription match objects; consume events via Events(). (Unexercised by the
// Stage 0 experiments; streaming semantics re-verify at Stage 1.)
func (c *Client) EventsSubscribe(ctx context.Context, subscriptions ...json.RawMessage) error {
	return c.subscribe(ctx, MethodEventsSubscribe, SubscribeParams{Subscriptions: subscriptions})
}

func (c *Client) PaneRead(ctx context.Context, p ReadParams) (*ReadResult, *Response, error) {
	resp, err := c.Call(ctx, MethodPaneRead, p)
	if err != nil {
		return nil, resp, err
	}
	var w readResult
	if err := resp.Decode(&w); err != nil {
		return nil, resp, err
	}
	return &w.Read, resp, nil
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
	var w agentResult
	if err := resp.Decode(&w); err != nil {
		return nil, resp, err
	}
	return &AgentStartResult{PaneID: w.Agent.PaneID, Name: w.Agent.Name}, resp, nil
}

// AgentExplainPane returns the (open) authority explanation plus the raw
// response; the raw line is what experiment reports record verbatim. The pane
// is addressed as `target` in protocol 16.
func (c *Client) AgentExplainPane(ctx context.Context, paneID string) (*AgentExplain, *Response, error) {
	resp, err := c.Call(ctx, MethodAgentExplain, map[string]string{"target": paneID})
	if err != nil {
		return nil, resp, err
	}
	var e AgentExplain
	if err := resp.Decode(&e); err != nil {
		return nil, resp, err
	}
	return &e, resp, nil
}

// AgentGet returns the typed agent record for a pane (addressed as `target`).
// AgentInfo.ScreenDetectionSkipped is the pivotal EXP1 signal.
func (c *Client) AgentGet(ctx context.Context, paneID string) (*AgentInfo, *Response, error) {
	resp, err := c.Call(ctx, MethodAgentGet, map[string]string{"target": paneID})
	if err != nil {
		return nil, resp, err
	}
	var w agentResult
	if err := resp.Decode(&w); err != nil {
		return nil, resp, err
	}
	return &w.Agent, resp, nil
}
