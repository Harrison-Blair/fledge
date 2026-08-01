package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Harrison-Blair/fledge/internal/processenv"
)

const MinimumProtocol = 17

var RequiredMethods = []string{
	"ping", "server.stop", "session.snapshot", "workspace.create", "workspace.focus",
	"tab.create", "tab.rename", "pane.focus", "pane.rename", "pane.split", "pane.process_info", "pane.send_input", "pane.close",
	"agent.start", "agent.list", "agent.get", "agent.read", "agent.send_keys",
	"agent.prompt", "agent.wait",
}

type BinaryInfo struct {
	Path     string   `json:"path"`
	Version  string   `json:"version"`
	Protocol int      `json:"protocol"`
	Methods  []string `json:"-"`
}

type SessionInfo struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Default    bool   `json:"default"`
	SessionDir string `json:"session_dir"`
	SocketPath string `json:"socket_path"`
}

type Binary struct{ Path string }

func (b Binary) path() string {
	if b.Path == "" {
		return "herdr"
	}
	return b.Path
}

// ResolvedPath is the executable this Binary invokes, including the default
// used when Path is unset.
func (b Binary) ResolvedPath() string { return b.path() }

func (b Binary) Inspect(ctx context.Context) (BinaryInfo, error) {
	path := b.path()
	versionOut, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return BinaryInfo{}, fmt.Errorf("run %s --version: %w", path, err)
	}
	schemaOut, err := exec.CommandContext(ctx, path, "api", "schema", "--json").Output()
	if err != nil {
		return BinaryInfo{}, fmt.Errorf("run %s api schema --json: %w", path, err)
	}
	var schema any
	if err := json.Unmarshal(schemaOut, &schema); err != nil {
		return BinaryInfo{}, fmt.Errorf("decode Herdr API schema: %w", err)
	}
	root, ok := schema.(map[string]any)
	if !ok {
		return BinaryInfo{}, fmt.Errorf("Herdr API schema is not an object")
	}
	protocol, _ := root["protocol"].(float64)
	methodSet := map[string]bool{}
	collectMethods(root, methodSet)
	for _, required := range RequiredMethods {
		if !methodSet[required] {
			return BinaryInfo{}, fmt.Errorf("Herdr API is missing required method %s; install Herdr 0.7.5 or newer", required)
		}
	}
	if int(protocol) < MinimumProtocol {
		return BinaryInfo{}, fmt.Errorf("Herdr protocol %d is unsupported; install Herdr 0.7.5 or newer (protocol %d+)", int(protocol), MinimumProtocol)
	}
	methods := make([]string, 0, len(methodSet))
	for method := range methodSet {
		methods = append(methods, method)
	}
	return BinaryInfo{Path: path, Version: strings.TrimSpace(string(versionOut)), Protocol: int(protocol), Methods: methods}, nil
}

func collectMethods(value any, methods map[string]bool) {
	switch v := value.(type) {
	case map[string]any:
		if raw, ok := v["method"].(map[string]any); ok {
			if method, ok := raw["const"].(string); ok {
				methods[method] = true
			}
		}
		for _, child := range v {
			collectMethods(child, methods)
		}
	case []any:
		for _, child := range v {
			collectMethods(child, methods)
		}
	}
}

func (b Binary) Sessions(ctx context.Context) ([]SessionInfo, error) {
	path := b.path()
	out, err := exec.CommandContext(ctx, path, "session", "list", "--json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list Herdr sessions: %w: %s", err, bytes.TrimSpace(out))
	}
	var payload struct {
		Sessions []SessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("decode Herdr session list: %w", err)
	}
	return payload.Sessions, nil
}

func (b Binary) FindSession(ctx context.Context, name string) (SessionInfo, bool, error) {
	sessions, err := b.Sessions(ctx)
	if err != nil {
		return SessionInfo{}, false, err
	}
	for _, session := range sessions {
		if session.Name == name {
			return session, true, nil
		}
	}
	return SessionInfo{}, false, nil
}

// DeleteSession permanently removes a stopped named Herdr session and its
// persisted restart state.
func (b Binary) DeleteSession(ctx context.Context, name string) error {
	path := b.path()
	out, err := exec.CommandContext(ctx, path, "session", "delete", name, "--json").CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete Herdr session %q: %w: %s", name, err, bytes.TrimSpace(out))
	}
	return nil
}

// StartServer starts Herdr in its own process group and reports early process
// failure on exited. The caller owns readiness polling.
func (b Binary) StartServer(ctx context.Context, session, cwd string) (<-chan error, error) {
	path := b.path()
	// Do not bind the long-lived server to the caller's context. The server is
	// deliberately detached and must survive after `fledge start` returns.
	cmd := exec.Command(path, "--session", session, "server")
	cmd.Dir = cwd
	cmd.Env = processenv.WithoutNoColor(os.Environ())
	cmd.Stdin = nil
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = devNull, devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		devNull.Close()
		return nil, err
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		devNull.Close()
	}()
	return exited, nil
}

func (b Binary) Attach(ctx context.Context, session, target string, takeover bool) error {
	path := b.path()
	args := []string{"--session", session, "agent", "attach", target}
	if takeover {
		args = append(args, "--takeover")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = processenv.WithoutNoColor(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (b Binary) AttachSession(ctx context.Context, session, cwd string) error {
	path := b.path()
	cmd := exec.CommandContext(ctx, path, "session", "attach", session)
	cmd.Dir = cwd
	cmd.Env = processenv.WithoutNoColor(os.Environ())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func Milliseconds(d time.Duration) *int64 {
	if d <= 0 {
		return nil
	}
	v := d.Milliseconds()
	return &v
}
