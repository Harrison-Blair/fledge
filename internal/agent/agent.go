package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"fledge/internal/herdr"
	"fledge/internal/project"
	"fledge/internal/session"
	"fledge/internal/session/record"
)

// Herder is the Herder CLI surface needed to manage agents.
type Herder interface {
	Workspaces(context.Context) ([]herdr.Workspace, error)
	CreateWorkspace(context.Context, string) (herdr.WorkspaceCreated, error)
	CreateTab(context.Context, string, string) (herdr.TabCreated, error)
	Panes(context.Context, string) ([]herdr.Pane, error)
	SplitPane(context.Context, herdr.SplitOptions) (herdr.Pane, error)
	ClosePane(context.Context, string) error
	StartAgent(context.Context, herdr.StartAgentOptions) (herdr.Agent, error)
	PromptAgent(context.Context, herdr.PromptOptions) (json.RawMessage, error)
	Agents(context.Context) ([]herdr.Agent, error)
	GetAgent(context.Context, string) (herdr.Agent, error)
}

// Caller is the validated Herder context the command was invoked from.
type Caller struct {
	Session     string
	RecordPath  string
	WorkspaceID string
	PaneID      string
}

// Connect resolves the caller's Herder context and returns a client scoped to
// the session that context belongs to.
func Connect(ctx context.Context, path string, getenv func(string) string, list func(context.Context) ([]herdr.Session, error), scoped func(string) session.PaneResolver) (Caller, *herdr.Client, error) {
	name, err := session.RunningSession(ctx, path, list)
	if err != nil {
		return Caller{}, nil, err
	}
	root, err := project.Find(path)
	if err != nil {
		return Caller{}, nil, fmt.Errorf("connect to Fledge session: %w", err)
	}
	records, err := record.Load(root)
	if err != nil {
		return Caller{}, nil, fmt.Errorf("connect to Fledge session: %w", err)
	}
	var recordPath string
	for _, rec := range records {
		if rec.HerdrSessionName == name {
			recordPath = rec.Path
			break
		}
	}
	if recordPath == "" {
		return Caller{}, nil, fmt.Errorf("connect to Fledge session: record for %q disappeared", name)
	}
	client := herdr.New(nil, nil, nil).WithSession(name)
	caller := Caller{Session: name, RecordPath: recordPath}
	if getenv("HERDR_ENV") == "1" {
		_, pane, err := session.ValidateAmbientPane(ctx, getenv, []string{name}, scoped)
		if err != nil {
			return Caller{}, nil, fmt.Errorf("connect to Fledge session: %w", err)
		}
		caller.WorkspaceID = pane.WorkspaceID
		caller.PaneID = pane.ID
	}
	return caller, client, nil
}

// MessageOptions describes prompt submission to one agent.
type MessageOptions struct {
	Target    string
	Text      string
	Wait      bool
	Until     []string
	TimeoutMS int
}

// Message submits prompt text to an agent and returns Herder's raw result.
func Message(ctx context.Context, h Herder, opts MessageOptions) (json.RawMessage, error) {
	return h.PromptAgent(ctx, herdr.PromptOptions{
		Target:    opts.Target,
		Text:      opts.Text,
		Wait:      opts.Wait,
		Until:     opts.Until,
		TimeoutMS: opts.TimeoutMS,
	})
}

// List returns every live agent in the session.
func List(ctx context.Context, h Herder) ([]herdr.Agent, error) {
	return h.Agents(ctx)
}

// Stop closes the pane hosting the agent addressed by name or pane ID.
func Stop(ctx context.Context, h Herder, target string) (string, error) {
	found, err := h.GetAgent(ctx, target)
	if err != nil {
		return "", fmt.Errorf("stop agent %q: %w", target, err)
	}
	if err := h.ClosePane(ctx, found.PaneID); err != nil {
		return "", fmt.Errorf("stop agent %q: close pane %q: %w", target, found.PaneID, err)
	}
	return found.PaneID, nil
}
