package fledge

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/messaging"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
)

type StartResult struct {
	ProjectRoot   string `json:"project_root"`
	Session       string `json:"session"`
	SessionSource string `json:"session_source"`
	Socket        string `json:"socket"`
	Started       bool   `json:"started"`
	Version       string `json:"herdr_version"`
	Protocol      int    `json:"protocol"`
}

type StatusResult struct {
	ProjectRoot         string         `json:"project_root"`
	Session             string         `json:"session"`
	SessionSource       string         `json:"session_source"`
	Socket              string         `json:"socket,omitempty"`
	HerdrVersion        string         `json:"herdr_version"`
	HerdrProtocol       int            `json:"herdr_protocol"`
	ServerVersion       string         `json:"server_version,omitempty"`
	ServerProtocol      int            `json:"server_protocol,omitempty"`
	ProtocolCompatible  bool           `json:"protocol_compatible"`
	ServerState         string         `json:"server_state"`
	AgentStates         map[string]int `json:"agent_states"`
	UserPendingMessages int            `json:"user_pending_messages"`
}

func (s *Service) Start(ctx context.Context, timeout time.Duration) (StartResult, error) {
	installed, err := s.inspect(ctx)
	if err != nil {
		return StartResult{}, err
	}
	session, client, pong, err := s.session(ctx, installed, false)
	if err != nil {
		return StartResult{}, err
	}
	if client != nil {
		if err := s.saveSocket(session.SocketPath); err != nil {
			return StartResult{}, Wrap("state_persist_failed", fmt.Sprintf("server is running but its socket could not be persisted: %v", err), err)
		}
		return s.startResult(session.SocketPath, false, pong), nil
	}
	if session.Name != "" && session.Running {
		return StartResult{}, NewError("session_not_running",
			fmt.Sprintf("Fledge session %q is reported running but is not reachable; stop it with Herdr before retrying `fledge start`", s.Project.Session))
	}
	if err := s.prepareFreshStart(ctx, session); err != nil {
		return StartResult{}, err
	}
	runID, err := s.beginMessageRun(ctx, installed)
	if err != nil {
		return StartResult{}, err
	}
	startedSuccessfully := false
	defer func() {
		if !startedSuccessfully {
			s.rollbackNewServer()
			_ = s.closeMessageRun(runID, "startup_failed")
		}
	}()
	startCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	exited, err := s.Binary.StartServer(startCtx, s.Project.Session, s.Project.Root)
	if err != nil {
		return StartResult{}, Wrap("server_start_failed", fmt.Sprintf("start Herdr server: %v", err), err)
	}
	result, err := s.waitForServerReady(startCtx, installed, exited, timeout)
	if err == nil {
		startedSuccessfully = true
	}
	return result, err
}

func (s *Service) rollbackNewServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	session, found, err := s.Binary.FindSession(ctx, s.Project.Session)
	if err != nil || !found {
		return
	}
	if session.Running && session.SocketPath != "" {
		_ = (&herdr.Client{Socket: session.SocketPath}).Call(ctx, "server.stop", nil, nil)
	}
	session, found, err = s.Binary.FindSession(ctx, s.Project.Session)
	if err == nil && found && !session.Running {
		_ = s.Binary.DeleteSession(ctx, s.Project.Session)
	}
}

func (s *Service) waitForServerReady(
	ctx context.Context,
	installed herdr.BinaryInfo,
	exited <-chan error,
	timeout time.Duration,
) (StartResult, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		select {
		case processErr := <-exited:
			if processErr == nil {
				processErr = errors.New("server process exited")
			}
			return StartResult{}, Wrap("server_exited", fmt.Sprintf("Herdr server exited before it became ready: %v", processErr), processErr)
		case <-ticker.C:
			result, ready, err := s.inspectStartedServer(ctx, installed)
			if err != nil {
				if ready {
					return StartResult{}, err
				}
				lastErr = err
				continue
			}
			if ready {
				return result, nil
			}
		case <-ctx.Done():
			message := fmt.Sprintf("Herdr session %q was not ready within %s", s.Project.Session, timeout)
			if lastErr != nil {
				message += ": " + lastErr.Error()
			}
			return StartResult{}, Wrap("server_start_timeout", message, ctx.Err())
		}
	}
}

func (s *Service) beginMessageRun(ctx context.Context, installed herdr.BinaryInfo) (string, error) {
	if err := project.EnsureLogsIgnored(s.Project.Root); err != nil {
		return "", Wrap("message_log_unavailable", err.Error(), err)
	}
	header := messaging.RunHeader{
		Fledge: buildinfo.Current(), Herdr: installed.Version, Protocol: installed.Protocol,
		ProjectRoot: s.Project.Root, Session: s.Project.Session, Git: inspectGit(ctx, s.Project.Root),
		StartedAt: time.Now().UTC(),
	}
	runID, err := s.messageStore().StartRun(header)
	if err != nil {
		return "", messageStoreError(err)
	}
	if err := s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		st.ActiveRunID = runID
		return nil
	}); err != nil {
		_ = s.closeMessageRun(runID, "state_persist_failed")
		return "", Wrap("state_persist_failed", fmt.Sprintf("persist active message run: %v", err), err)
	}
	return runID, nil
}

func inspectGit(ctx context.Context, root string) messaging.GitInfo {
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = root
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	head, err := run("rev-parse", "HEAD")
	if err != nil {
		return messaging.GitInfo{Error: "unknown"}
	}
	branch, branchErr := run("branch", "--show-current")
	status, dirtyErr := run("status", "--porcelain")
	dirty := status != ""
	info := messaging.GitInfo{Head: head, Branch: branch}
	if branchErr == nil && dirtyErr == nil {
		info.Dirty = &dirty
	} else {
		info.Error = "partial"
	}
	return info
}

func (s *Service) inspectStartedServer(ctx context.Context, installed herdr.BinaryInfo) (StartResult, bool, error) {
	candidate, found, err := s.Binary.FindSession(ctx, s.Project.Session)
	if err != nil || !found || candidate.SocketPath == "" {
		return StartResult{}, false, err
	}
	client := &herdr.Client{Socket: candidate.SocketPath}
	ready, err := client.Ping(ctx)
	if err != nil {
		return StartResult{}, false, err
	}
	if ready.Protocol != installed.Protocol {
		return StartResult{}, true, NewError("protocol_mismatch",
			fmt.Sprintf("new Herdr server uses protocol %d, installed schema uses %d", ready.Protocol, installed.Protocol))
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return StartResult{}, true, Wrap("session_discovery_failed",
			fmt.Sprintf("new Herdr server is ready but its workspace snapshot failed: %v", err), err)
	}
	if snapshot.Protocol != installed.Protocol {
		return StartResult{}, true, NewError("session_discovery_failed",
			fmt.Sprintf("new Herdr server returned snapshot protocol %d, expected %d", snapshot.Protocol, installed.Protocol))
	}
	workspace, matched, err := matchingWorkspace(snapshot, s.Project.Root)
	if err != nil {
		return StartResult{}, true, Wrap("session_discovery_failed",
			fmt.Sprintf("inspect new Herdr workspace metadata: %v", err), err)
	}
	if matched {
		s.WorkspaceID = workspace.WorkspaceID
	}
	if err := s.saveSocket(candidate.SocketPath); err != nil {
		return StartResult{}, true, Wrap("state_persist_failed",
			fmt.Sprintf("server started but its socket could not be persisted: %v", err), err)
	}
	return s.startResult(candidate.SocketPath, true, ready), true, nil
}

func (s *Service) startResult(socket string, started bool, pong herdr.Pong) StartResult {
	return StartResult{
		ProjectRoot: s.Project.Root, Session: s.Project.Session, SessionSource: s.Project.SessionSource,
		Socket: socket, Started: started, Version: pong.Version, Protocol: pong.Protocol,
	}
}

// prepareFreshStart removes any stopped deterministic session and all
// disposable mappings. Only the coordinated-stop generation survives between
// server lifecycles.
func (s *Service) prepareFreshStart(ctx context.Context, session herdr.SessionInfo) error {
	if session.Name != "" {
		if err := s.Binary.DeleteSession(ctx, s.Project.Session); err != nil {
			if _, found, findErr := s.Binary.FindSession(ctx, s.Project.Session); findErr != nil || found {
				return Wrap("session_delete_failed",
					fmt.Sprintf("cannot delete stopped Herdr session %q before startup: %v; delete it with `herdr session delete %s --json` and retry",
						s.Project.Session, err, s.Project.Session), err)
			}
		}
	}
	if err := s.closeActiveMessageRun("abnormal prior run closed before fresh start"); err != nil {
		var serviceErr *Error
		if errors.As(err, &serviceErr) && strings.HasPrefix(serviceErr.Code, "message_") {
			return err
		}
		return Wrap("state_persist_failed",
			fmt.Sprintf("cannot inspect stale Fledge mappings before startup: %v", err), err)
	}
	if err := s.clearDisposableState(); err != nil {
		message := fmt.Sprintf("cannot clear stale Fledge mappings before startup: %v; check access to the Fledge state directory", err)
		if session.Name != "" {
			message = fmt.Sprintf("stopped Herdr session %q was deleted, but stale Fledge mappings could not be cleared: %v; check access to the Fledge state directory",
				s.Project.Session, err)
		}
		return Wrap("state_persist_failed", message, err)
	}
	s.WorkspaceID = ""
	return nil
}

func (s *Service) saveSocket(socket string) error {
	return s.Store.WithLocked(s.Project.Session, s.Project.Root, func(st *state.Session) error {
		st.Socket = socket
		if s.WorkspaceID != "" {
			st.WorkspaceID = s.WorkspaceID
		}
		return nil
	})
}

// StopGeneration returns the persisted count of successful coordinated stops.
func (s *Service) StopGeneration() (uint64, error) {
	st, err := s.Store.Read(s.Project.Session, s.Project.Root)
	if err != nil {
		return 0, err
	}
	return st.StopGeneration, nil
}

// WaitForStopGeneration reports whether a successful coordinated stop was
// persisted after generation. It performs a final inspection at the timeout
// boundary so a stop racing the last ticker interval is not missed.
func (s *Service) WaitForStopGeneration(ctx context.Context, generation uint64, timeout time.Duration) (bool, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		current, err := s.StopGeneration()
		if err != nil {
			return false, err
		}
		if current > generation {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-ticker.C:
		case <-timer.C:
			current, err := s.StopGeneration()
			if err != nil {
				return false, err
			}
			return current > generation, nil
		}
	}
}

func (s *Service) Status(ctx context.Context) (StatusResult, error) {
	installed, err := s.inspect(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	out := StatusResult{
		ProjectRoot: s.Project.Root, Session: s.Project.Session, SessionSource: s.Project.SessionSource,
		HerdrVersion: installed.Version, HerdrProtocol: installed.Protocol,
		ServerState: "stopped", ProtocolCompatible: true, AgentStates: emptyCounts(),
	}
	if st, found, readErr := s.Store.ReadExisting(s.Project.Session, s.Project.Root); readErr != nil {
		return StatusResult{}, Wrap("state_unavailable", readErr.Error(), readErr)
	} else if found {
		pending, countErr := s.pendingMessageCounts(st.ActiveRunID)
		if countErr != nil {
			return StatusResult{}, countErr
		}
		out.UserPendingMessages = pending[userMailbox]
	}
	session, client, pong, err := s.session(ctx, installed, false)
	if err != nil {
		return StatusResult{}, err
	}
	out.Socket = session.SocketPath
	if client == nil {
		return out, nil
	}
	out.ServerState, out.ServerVersion, out.ServerProtocol = "running", pong.Version, pong.Protocol
	out.ProtocolCompatible = pong.Protocol == installed.Protocol
	agents, err := s.listWithClient(ctx, client)
	if err != nil {
		return StatusResult{}, err
	}
	for _, agent := range agents {
		out.AgentStates[agent.State]++
	}
	return out, nil
}

func emptyCounts() map[string]int {
	counts := make(map[string]int, len(ReportedStates))
	for _, name := range ReportedStates {
		counts[name] = 0
	}
	return counts
}
