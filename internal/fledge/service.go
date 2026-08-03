package fledge

import (
	"context"
	"fmt"
	"time"

	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

const (
	legacyWorkspaceLabelPrefix = "fledge:"
	agentLabelPrefix           = "fledge-agent:"
	orchestratorLabel          = "orchestrator"
)

type Service struct {
	Project              project.Info
	Binary               herdr.Binary
	Store                *state.Store
	Installed            *herdr.BinaryInfo
	WorkspaceID          string
	LaunchStopCleanup    func(StopCleanupRequest) error
	ExecAgent            func(path string, argv, env []string) error
	LaunchDeliveryHelper func(name, activationID string, timeout time.Duration) error
	// FledgeExecutable overrides the binary the dedicated-tab bootstrap
	// re-invokes in the prepared pane; empty means the running executable.
	FledgeExecutable string
	MessageStore     *messaging.Store
	CallerPaneID     string
}

func (s *Service) messageStore() *messaging.Store {
	if s.MessageStore != nil {
		return s.MessageStore
	}
	return messaging.NewStore(s.Project.Root)
}

// messages builds the durable-messaging collaborator from the fields the
// caller configured. It is rebuilt per operation because CallerPaneID and the
// store fields stay writable after the Service is constructed.
func (s *Service) messages() *messenger {
	return &messenger{
		project:      s.Project,
		store:        s.Store,
		log:          s.messageStore(),
		callerPaneID: s.CallerPaneID,
		connect:      s.messagingClient,
	}
}

func (s *Service) messagingClient(ctx context.Context) (*herdr.Client, error) {
	_, _, client, err := s.running(ctx)
	return client, err
}

func (s *Service) inspect(ctx context.Context) (herdr.BinaryInfo, error) {
	if s.Installed != nil {
		return *s.Installed, nil
	}
	info, err := s.Binary.Inspect(ctx)
	if err != nil {
		return herdr.BinaryInfo{}, Wrap("herdr_incompatible", err.Error(), err)
	}
	return info, nil
}

func (s *Service) session(ctx context.Context, installed herdr.BinaryInfo, required bool) (herdr.SessionInfo, *herdr.Client, herdr.Pong, error) {
	session, found, err := s.Binary.FindSession(ctx, s.Project.Session)
	if err != nil {
		return herdr.SessionInfo{}, nil, herdr.Pong{}, Wrap("herdr_discovery_failed", err.Error(), err)
	}
	if !found || !session.Running || session.SocketPath == "" {
		if required {
			return herdr.SessionInfo{}, nil, herdr.Pong{}, NewError("session_not_running",
				fmt.Sprintf("Fledge session %q is not running; run `fledge start` first", s.Project.Session))
		}
		return session, nil, herdr.Pong{}, nil
	}
	client := &herdr.Client{Socket: session.SocketPath}
	pong, err := client.Ping(ctx)
	if err != nil {
		if required {
			return session, nil, herdr.Pong{}, Wrap("session_not_running",
				fmt.Sprintf("Fledge session %q is not reachable; run `fledge start`", s.Project.Session), err)
		}
		return session, nil, herdr.Pong{}, nil
	}
	if pong.Protocol != installed.Protocol {
		return session, client, pong, &Error{
			Code: "protocol_mismatch",
			Message: fmt.Sprintf("running Herdr server uses protocol %d but %s provides protocol %d; stop and restart the session after updating Herdr",
				pong.Protocol, installed.Path, installed.Protocol),
			Details: map[string]int{"server_protocol": pong.Protocol, "installed_protocol": installed.Protocol},
		}
	}
	return session, client, pong, nil
}

func (s *Service) running(ctx context.Context) (herdr.BinaryInfo, herdr.SessionInfo, *herdr.Client, error) {
	installed, err := s.inspect(ctx)
	if err != nil {
		return installed, herdr.SessionInfo{}, nil, err
	}
	session, client, _, err := s.session(ctx, installed, true)
	return installed, session, client, err
}
