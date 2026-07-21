// Package agentcfg holds named agent configurations and the static table
// routing model ids to integrations. Routing never guesses: a model the table
// does not know is an error, not a default.
package agentcfg

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

// FileName is the agent config file, relative to the .fledge directory.
const FileName = "agents.json"

// Config describes one launchable agent.
type Config struct {
	Integration    string            `json:"integration"`
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	PermissionMode string            `json:"permission_mode,omitempty"`
	Argv           []string          `json:"argv,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
}

// Load reads the agent configs under root. A missing file is not an error: it
// yields an empty map, since configs are optional until an operator writes one.
func Load(root string) (map[string]Config, error) {
	data, err := os.ReadFile(filepath.Join(root, scaffold.DirName, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	configs := map[string]Config{}
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", FileName, err)
	}
	return configs, nil
}

// Route maps a model id to the integration that can launch it, and for pi the
// provider to launch it under. Matching is by prefix over a fixed table.
func Route(model string) (integration, provider string, err error) {
	switch {
	case strings.HasPrefix(model, "claude"):
		return "claude", "", nil
	// openai-codex, not openai: pi reaches OpenAI through a ChatGPT
	// subscription (OAuth), where the plain openai provider wants an API key.
	case strings.HasPrefix(model, "gpt"), strings.HasPrefix(model, "codex"), isOSeries(model):
		return "pi", "openai-codex", nil
	// opencode-go before opencode: distinct pi providers, shared prefix.
	case strings.HasPrefix(model, "opencode-go"):
		return "pi", "opencode-go", nil
	case strings.HasPrefix(model, "opencode"):
		return "pi", "opencode", nil
	}
	return "", "", fmt.Errorf("unknown model %q: add it to %s/%s", model, scaffold.DirName, FileName)
}

// isOSeries reports whether model names an OpenAI o-series model (o3, o4-mini).
func isOSeries(model string) bool {
	return len(model) > 1 && model[0] == 'o' && model[1] >= '0' && model[1] <= '9'
}

// Validate checks that a named config entry is launchable.
func (c Config) Validate(name string) error {
	if err := validName(name); err != nil {
		return err
	}
	switch c.Integration {
	case "claude":
		if c.Provider != "" {
			return fmt.Errorf("agent %q: provider is pi-only", name)
		}
	case "pi":
		if c.PermissionMode != "" {
			return fmt.Errorf("agent %q: permission_mode is claude-only", name)
		}
	default:
		return fmt.Errorf("agent %q: invalid integration %q: use \"claude\" or \"pi\"", name, c.Integration)
	}
	return nil
}

// ReservedOrchestrator is the single name exempt from the agent naming rule.
// fledge start brings this profile up on every interactive start and the agent
// runs under this exact string — hyphen included, and with no species suffix —
// so that the one agent an operator always has is the one whose name they
// already know without looking it up.
const ReservedOrchestrator = "fledge-orchestrator"

// validName accepts lowercase alphanumerics only, matching the agent naming
// rule the daemon enforces. The reserved orchestrator name is the sole
// exception; every other name keeps the rule, hyphens included.
func validName(name string) error {
	if name == "" {
		return errors.New("missing agent name")
	}
	if name == ReservedOrchestrator {
		return nil
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return fmt.Errorf("invalid agent name %q: use lowercase letters and digits only", name)
		}
	}
	return nil
}

// CommandArgv assembles the full launch argv for the config. sessionID is used
// by the claude integration only; pi has no equivalent flag. An integration the
// table does not know yields nil rather than a guess.
func (c Config) CommandArgv(sessionID string) []string {
	var argv []string
	switch c.Integration {
	case "claude":
		argv = []string{"claude", "--session-id", sessionID}
		if c.PermissionMode != "" {
			argv = append(argv, "--permission-mode", c.PermissionMode)
		}
	case "pi":
		argv = []string{"pi", "--mode", "rpc"}
		if c.Provider != "" {
			argv = append(argv, "--provider", c.Provider)
		}
	default:
		return nil
	}
	if c.Model != "" {
		argv = append(argv, "--model", c.Model)
	}
	return append(argv, c.Argv...)
}

// NewSessionID returns a fresh RFC-4122 version 4 UUID.
func NewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("agentcfg: crypto/rand: " + err.Error())
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80

	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}
