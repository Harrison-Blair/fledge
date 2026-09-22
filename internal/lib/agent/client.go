package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// API is the socket boundary; tests supply a deterministic request script.
type API interface {
	Call(context.Context, string, any, any) error
}

// Client performs validated Herdr agent requests for one operation.
type Client struct {
	API             API
	CallerPane, Cwd string
	initErr         error
}

// FromEnvironment creates an operation-local client without contacting Herdr.
func FromEnvironment(timeout time.Duration) Client {
	c := Client{CallerPane: os.Getenv("HERDR_PANE_ID")}
	if os.Getenv("HERDR_ENV") != "1" || os.Getenv("HERDR_SOCKET_PATH") == "" {
		c.initErr = fmt.Errorf("run inside Herdr with HERDR_ENV=1 and HERDR_SOCKET_PATH set")
	}
	c.Cwd, _ = os.Getwd()
	c.API = herdr.Client{Socket: os.Getenv("HERDR_SOCKET_PATH"), Timeout: timeout + 15*time.Second}
	return c
}

// Call sends one request, locating any transport failure at method's phase.
func (c Client) Call(ctx context.Context, method string, params any, result any) error {
	if c.initErr != nil {
		return c.initErr
	}
	err := c.API.Call(ctx, method, params, result)
	if err != nil {
		return &phaseError{phase: method, cause: err}
	}
	return nil
}

// Get fetches one agent by target, rejecting incomplete agent.get results.
func (c Client) Get(ctx context.Context, target string) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	err := c.Call(ctx, "agent.get", map[string]any{"target": target}, &r)
	if err == nil && (r.Type != "agent_info" || !ValidAgentInfo(r.Agent)) {
		err = Protocol("incomplete agent.get result")
	}
	return r.Agent, err
}

// Prompt submits text to target, rejecting incomplete agent.prompt results.
func (c Client) Prompt(ctx context.Context, target, text string) (herdr.AgentDetails, error) {
	var r herdr.AgentResult
	err := c.Call(ctx, "agent.prompt", map[string]any{"target": target, "text": text}, &r)
	if err == nil && (r.Type != "agent_prompted" || !ValidAgent(r.Agent.Pane)) {
		err = Protocol("incomplete agent.prompt result")
	}
	if err != nil {
		return herdr.AgentDetails{}, err
	}
	return r.Agent, nil
}

// ResolveTarget collapses the exactly-one-of --name/--pane choice into one agent target.
func ResolveTarget(name, pane string) (string, error) {
	if (name == "") == (pane == "") {
		return "", Invalid("exactly one of --name or --pane is required")
	}
	if name != "" {
		return name, nil
	}
	return pane, nil
}

// Protocol reports a malformed Herdr result; the request may have taken effect.
func Protocol(message string) error {
	return &herdr.Error{Code: "protocol_error", Message: message, Uncertain: true}
}
func ValidPane(p herdr.Pane) bool { return p.PaneID != "" && p.WorkspaceID != "" && p.TabID != "" }
func ValidAgent(p herdr.Pane) bool {
	if !ValidPane(p) {
		return false
	}
	switch p.AgentStatus {
	case "idle", "working", "blocked", "done", "unknown":
		return true
	}
	return false
}
func ValidAgentInfo(a herdr.AgentDetails) bool {
	if !ValidAgent(a.Pane) || a.TerminalID == "" || a.Focused == nil || a.Revision == nil {
		return false
	}
	if s := a.AgentSession; s != nil {
		return s.Source != nil && s.Agent != nil && s.Kind != nil && s.Value != nil &&
			(*s.Kind == "id" || *s.Kind == "path")
	}
	return true
}

// AtPhase attributes err to an API phase, as Call does for transport failures.
func AtPhase(phase string, err error) error { return &phaseError{phase: phase, cause: err} }

type phaseError struct {
	phase string
	cause error
}

func (e *phaseError) Error() string { return e.cause.Error() }
func (e *phaseError) Unwrap() error { return e.cause }
