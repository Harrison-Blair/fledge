// Package herdrwire speaks the Herdr socket API directly for pane control.
// It complements internal/herdr, which shells out to the herdr CLI for session
// lifecycle: sessions are resolved and started through the CLI, everything a
// running pane needs is done here over the session's unix socket.
//
// Verified live against herdr 0.7.4 / protocol 16. The transport is
// newline-delimited JSON, one request per connection: the server reads a single
// line, writes a single line, and closes. Every call therefore dials fresh.
package herdrwire

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// dialTimeout bounds connecting to the session socket; callTimeout bounds the
// write-and-read that follows. Both are fixed rather than caller-supplied: a
// wedged Herdr socket must fail the operation that touched it instead of
// blocking the caller forever. callTimeout is a var so tests can lower it.
const dialTimeout = 5 * time.Second

var callTimeout = 10 * time.Second

// nextID numbers requests. Protocol 16 only requires an id that is non-empty
// and unique within a connection's lifetime; a process-wide counter is enough.
var nextID atomic.Uint64

// request is the protocol-16 envelope. Params is mandatory on every method, so
// it is never omitempty — a method taking no arguments sends {}.
type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

// response is the single reply line. Exactly one of Result and Error is set.
type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *wireError      `json:"error"`
}

// wireError is an error the server reported, as opposed to a transport or
// decode failure. AgentAlive distinguishes the two by testing for this type.
type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	method  string
}

func (e *wireError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.method, e.Message, e.Code)
}

// Call dials socket, sends one request line, and decodes the one response line
// into result, which may be nil to discard it. A nil params is sent as {}.
func Call(socket, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	req := request{
		ID:     strconv.FormatUint(nextID.Add(1), 10),
		Method: method,
		Params: params,
	}

	conn, err := net.DialTimeout("unix", socket, dialTimeout)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(callTimeout)); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}

	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if resp.Error != nil {
		resp.Error.method = method
		return resp.Error
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// StartedAgent is what agent.start reports about the new pane.
type StartedAgent struct {
	PaneID     string
	TerminalID string
}

// AgentStart launches argv in a new named pane. A non-empty split ("right" or
// "down") splits the focused pane to make room for the new one instead of
// taking over a tab; empty means let herdr place it.
func AgentStart(socket, name, cwd string, argv []string, env map[string]string, split string) (StartedAgent, error) {
	return agentStart(socket, name, cwd, argv, env, split, "", "", false)
}

// AgentStartInWorkspace launches an unfocused agent in a specific existing
// workspace and tab. Dedicated-definition placement uses this after creating
// and labelling the workspace, so it never depends on the operator's focus.
func AgentStartInWorkspace(socket, name, cwd string, argv []string, env map[string]string, workspaceID, tabID string) (StartedAgent, error) {
	return agentStart(socket, name, cwd, argv, env, "", workspaceID, tabID, true)
}

func agentStart(socket, name, cwd string, argv []string, env map[string]string, split, workspaceID, tabID string, explicitFocus bool) (StartedAgent, error) {
	params := struct {
		Name        string            `json:"name"`
		Argv        []string          `json:"argv"`
		Cwd         string            `json:"cwd,omitempty"`
		Env         map[string]string `json:"env,omitempty"`
		Split       string            `json:"split,omitempty"`
		WorkspaceID string            `json:"workspace_id,omitempty"`
		TabID       string            `json:"tab_id,omitempty"`
		Focus       *bool             `json:"focus,omitempty"`
	}{Name: name, Argv: argv, Cwd: cwd, Env: env, Split: split, WorkspaceID: workspaceID, TabID: tabID}
	if explicitFocus {
		focus := false
		params.Focus = &focus
	}

	var result struct {
		Agent struct {
			PaneID     string `json:"pane_id"`
			TerminalID string `json:"terminal_id"`
		} `json:"agent"`
	}
	if err := Call(socket, "agent.start", params, &result); err != nil {
		return StartedAgent{}, err
	}
	return StartedAgent{PaneID: result.Agent.PaneID, TerminalID: result.Agent.TerminalID}, nil
}

// CreatedWorkspace identifies the objects Herdr creates together for a new
// workspace. Callers need all three IDs to label the initial tab, address its
// normal root shell, and roll the whole workspace back if later setup fails.
type CreatedWorkspace struct {
	WorkspaceID string
	TabID       string
	RootPaneID  string
}

// Workspace identifies one live Herdr workspace for placement resolution.
type Workspace struct {
	WorkspaceID string
	Label       string
}

// WorkspaceList returns every live workspace in the session.
func WorkspaceList(socket string) ([]Workspace, error) {
	var result struct {
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"workspaces"`
	}
	if err := Call(socket, "workspace.list", nil, &result); err != nil {
		return nil, err
	}
	out := make([]Workspace, 0, len(result.Workspaces))
	for _, workspace := range result.Workspaces {
		out = append(out, Workspace{
			WorkspaceID: workspace.WorkspaceID,
			Label:       workspace.Label,
		})
	}
	return out, nil
}

// WorkspaceCreate creates a workspace rooted at cwd, labelled label, with the
// requested focus behavior. A fresh session has
// no workspace until a client attaches, and the one herdr then manufactures has
// no reliable cwd (observed landing in $HOME on 0.7.4: its "follow the source
// pane" default has nothing to follow) — so fledge creates the first workspace
// itself before anyone attaches.
//
// The initial tab already exists when this returns and its id comes back in the
// same reply (verified on 0.7.4 / protocol 16), so labelling that tab needs no
// tab.list lookup.
func WorkspaceCreate(socket, cwd, label string, focus bool) (CreatedWorkspace, error) {
	params := struct {
		Cwd   string `json:"cwd"`
		Focus bool   `json:"focus"`
		Label string `json:"label,omitempty"`
	}{Cwd: cwd, Focus: focus, Label: label}

	var result struct {
		Workspace struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspace"`
		Tab struct {
			TabID string `json:"tab_id"`
		} `json:"tab"`
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
	}
	if err := Call(socket, "workspace.create", params, &result); err != nil {
		return CreatedWorkspace{}, err
	}
	return CreatedWorkspace{
		WorkspaceID: result.Workspace.WorkspaceID,
		TabID:       result.Tab.TabID,
		RootPaneID:  result.RootPane.PaneID,
	}, nil
}

// WorkspaceClose closes one workspace and all panes it owns.
func WorkspaceClose(socket, workspaceID string) error {
	params := struct {
		WorkspaceID string `json:"workspace_id"`
	}{WorkspaceID: workspaceID}
	return Call(socket, "workspace.close", params, nil)
}

// Tab identifies one live Herdr tab for placement resolution.
type Tab struct {
	TabID       string
	WorkspaceID string
	Label       string
}

// CreatedTab identifies a newly created tab and its initial shell pane.
type CreatedTab struct {
	TabID      string
	RootPaneID string
}

// TabList returns the tabs in workspaceID.
func TabList(socket, workspaceID string) ([]Tab, error) {
	params := struct {
		WorkspaceID string `json:"workspace_id"`
	}{WorkspaceID: workspaceID}
	var result struct {
		Tabs []struct {
			TabID       string `json:"tab_id"`
			WorkspaceID string `json:"workspace_id"`
			Label       string `json:"label"`
		} `json:"tabs"`
	}
	if err := Call(socket, "tab.list", params, &result); err != nil {
		return nil, err
	}
	out := make([]Tab, 0, len(result.Tabs))
	for _, tab := range result.Tabs {
		out = append(out, Tab{
			TabID:       tab.TabID,
			WorkspaceID: tab.WorkspaceID,
			Label:       tab.Label,
		})
	}
	return out, nil
}

// TabCreate creates an unfocused tab in workspaceID. Herdr creates an initial
// shell pane with every tab; callers may close it after placing a durable pane.
func TabCreate(socket, workspaceID, cwd, label string) (CreatedTab, error) {
	params := struct {
		WorkspaceID string `json:"workspace_id"`
		Cwd         string `json:"cwd"`
		Label       string `json:"label"`
		Focus       bool   `json:"focus"`
	}{WorkspaceID: workspaceID, Cwd: cwd, Label: label, Focus: false}
	var result struct {
		Tab struct {
			TabID string `json:"tab_id"`
		} `json:"tab"`
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
	}
	if err := Call(socket, "tab.create", params, &result); err != nil {
		return CreatedTab{}, err
	}
	return CreatedTab{TabID: result.Tab.TabID, RootPaneID: result.RootPane.PaneID}, nil
}

// TabClose closes one tab and every pane it owns.
func TabClose(socket, tabID string) error {
	params := struct {
		TabID string `json:"tab_id"`
	}{TabID: tabID}
	return Call(socket, "tab.close", params, nil)
}

// TabRename labels a tab.
func TabRename(socket, tabID, label string) error {
	params := struct {
		TabID string `json:"tab_id"`
		Label string `json:"label"`
	}{TabID: tabID, Label: label}
	return Call(socket, "tab.rename", params, nil)
}

// ProcessInfo returns the pane's shell pid, the long-lived pane process. Herdr
// reports a null shell_pid for a pane that has no process yet; that is not an
// error and returns 0.
func ProcessInfo(socket, paneID string) (shellPID int, err error) {
	var result struct {
		ProcessInfo struct {
			ShellPID *int `json:"shell_pid"`
		} `json:"process_info"`
	}
	if err := Call(socket, "pane.process_info", paneParams(paneID), &result); err != nil {
		return 0, err
	}
	if result.ProcessInfo.ShellPID == nil {
		return 0, nil
	}
	return *result.ProcessInfo.ShellPID, nil
}

// SendInput types text into a pane. pressEnter appends keys:["enter"], which is
// what actually submits in a TUI — a bare \r in text does not (EXP2).
func SendInput(socket, paneID, text string, pressEnter bool) error {
	params := struct {
		PaneID string   `json:"pane_id"`
		Text   string   `json:"text"`
		Keys   []string `json:"keys,omitempty"`
	}{PaneID: paneID, Text: text}
	if pressEnter {
		params.Keys = []string{"enter"}
	}
	return Call(socket, "pane.send_input", params, nil)
}

// PaneCurrent returns the focused pane's id — the pane an agent.start split
// will divide.
func PaneCurrent(socket string) (paneID string, err error) {
	var result struct {
		Pane struct {
			PaneID string `json:"pane_id"`
		} `json:"pane"`
	}
	if err := Call(socket, "pane.current", nil, &result); err != nil {
		return "", err
	}
	return result.Pane.PaneID, nil
}

// PaneSwap exchanges the positions of two panes. Verified on 0.7.4: the panes
// trade slots and focus stays with the *slot*, so a caller that wants a
// specific pane focused afterwards must say so with PaneFocus.
func PaneSwap(socket, sourcePaneID, targetPaneID string) error {
	params := struct {
		SourcePaneID string `json:"source_pane_id"`
		TargetPaneID string `json:"target_pane_id"`
	}{SourcePaneID: sourcePaneID, TargetPaneID: targetPaneID}
	return Call(socket, "pane.swap", params, nil)
}

// PaneFocus focuses a pane.
func PaneFocus(socket, paneID string) error {
	return Call(socket, "pane.focus", paneParams(paneID), nil)
}

// PaneClose closes a pane.
func PaneClose(socket, paneID string) error {
	return Call(socket, "pane.close", paneParams(paneID), nil)
}

// ReleaseAgent drops a custom agent identity from a pane on clean exit.
func ReleaseAgent(socket, paneID, source, agent string) error {
	params := struct {
		PaneID string `json:"pane_id"`
		Source string `json:"source"`
		Agent  string `json:"agent"`
	}{PaneID: paneID, Source: source, Agent: agent}
	return Call(socket, "pane.release_agent", params, nil)
}

// ReportMetadata sets display-only pane metadata. It never seizes agent
// authority: native screen detection still wins (EXP1), so this is titling
// only.
func ReportMetadata(socket, paneID, source, title string) error {
	params := struct {
		PaneID string `json:"pane_id"`
		Source string `json:"source"`
		Title  string `json:"title"`
	}{PaneID: paneID, Source: source, Title: title}
	return Call(socket, "pane.report_metadata", params, nil)
}

// WindowTitleSet sets the terminal window title of the session's foreground
// client, and reports whether it landed. Herdr applies a title to an attached
// client only: with nobody attached it answers no_foreground_client and
// changes nothing, which is the normal state between a session starting and
// the operator attaching to it. Callers that need the title to stick must
// therefore retry rather than set it once.
func WindowTitleSet(socket, title string) (changed bool, err error) {
	params := struct {
		Title string `json:"title"`
	}{Title: title}

	var result struct {
		Changed bool `json:"changed"`
	}
	if err := Call(socket, "client.window_title.set", params, &result); err != nil {
		return false, err
	}
	return result.Changed, nil
}

// AgentAlive reports whether Herdr still knows the pane. Any error the server
// reports is read as "not alive" rather than matched against a specific code:
// protocol 16's error codes for an unknown or closed pane are not pinned down,
// and agent.get on a live pane succeeds. Transport failures still propagate,
// since those say nothing about the pane.
func AgentAlive(socket, paneID string) (bool, error) {
	params := struct {
		Target string `json:"target"`
	}{Target: paneID}
	err := Call(socket, "agent.get", params, nil)
	if err == nil {
		return true, nil
	}
	var werr *wireError
	if errors.As(err, &werr) {
		return false, nil
	}
	return false, err
}

// AgentStatus returns Herdr's native screen-detected status for a pane-hosted
// agent. A newly created pane reports "unknown" until the integration has
// initialized its TUI; sending input before that transition can race startup
// and leave the text echoed above (rather than submitted inside) the TUI.
func AgentStatus(socket, paneID string) (string, error) {
	params := struct {
		Target string `json:"target"`
	}{Target: paneID}
	var result struct {
		Agent struct {
			Status string `json:"agent_status"`
		} `json:"agent"`
	}
	if err := Call(socket, "agent.get", params, &result); err != nil {
		return "", err
	}
	return result.Agent.Status, nil
}

// paneParams is the {"pane_id": ...} body shared by the pane-scoped methods.
func paneParams(paneID string) any {
	return struct {
		PaneID string `json:"pane_id"`
	}{PaneID: paneID}
}
