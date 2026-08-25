// Package herdr provides the small Herder CLI surface used by Fledge.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Session is one entry returned by herdr session list --json.
type Session struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Default    bool   `json:"default"`
	SocketPath string `json:"socket_path"`
}

// Client invokes the Herder CLI. The configured streams are connected to an
// interactive Herder process launched by Launch.
type Client struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// New returns a Herder client whose interactive child uses the supplied
// terminal streams.
func New(stdin io.Reader, stdout, stderr io.Writer) *Client {
	return &Client{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
}

// List returns every named Herder session, including stopped sessions.
func (c *Client) List(ctx context.Context) ([]Session, error) {
	cmd := exec.CommandContext(ctx, "herdr", "session", "list", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, commandError("herdr session list --json", err, stderr.String())
	}

	var payload struct {
		Sessions json.RawMessage `json:"sessions"`
	}
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("herdr session list --json: decode output: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("herdr session list --json: decode output: %w", err)
	}
	if len(payload.Sessions) == 0 || string(payload.Sessions) == "null" {
		return nil, fmt.Errorf("herdr session list --json: decode output: missing sessions array")
	}

	var entries []struct {
		Name       *string `json:"name"`
		Running    *bool   `json:"running"`
		Default    *bool   `json:"default"`
		SocketPath *string `json:"socket_path"`
	}
	if err := json.Unmarshal(payload.Sessions, &entries); err != nil {
		return nil, fmt.Errorf("herdr session list --json: decode sessions: %w", err)
	}
	sessions := make([]Session, len(entries))
	for i, entry := range entries {
		if entry.Name == nil || entry.Running == nil || entry.Default == nil || entry.SocketPath == nil {
			return nil, fmt.Errorf("herdr session list --json: decode session %d: missing required field", i)
		}
		if *entry.Name == "" {
			return nil, fmt.Errorf("herdr session list --json: decode session %d: empty name", i)
		}
		sessions[i] = Session{
			Name:       *entry.Name,
			Running:    *entry.Running,
			Default:    *entry.Default,
			SocketPath: *entry.SocketPath,
		}
	}
	return sessions, nil
}

// Launch attaches to name, creating the named session when it does not exist.
// Herder inherits the configured streams and runs from projectRoot.
func (c *Client) Launch(ctx context.Context, projectRoot, name string) error {
	cmd := exec.CommandContext(ctx, "herdr", "--session", name)
	cmd.Dir = projectRoot
	cmd.Stdin = c.stdin
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("herdr --session %s: %w", name, err)
	}
	return nil
}

// Stop stops the named Herder session.
func (c *Client) Stop(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "herdr", "session", "stop", name, "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		output := stderr.String()
		if output == "" {
			output = stdout.String()
		}
		return commandError("herdr session stop "+name+" --json", err, output)
	}
	return nil
}

func commandError(operation string, err error, output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s: %w: %s", operation, err, output)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
