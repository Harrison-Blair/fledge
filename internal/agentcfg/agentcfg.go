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

// FileName is the generated user index, relative to the .fledge directory.
const FileName = "agents/fledge/user-agents.json"

// ManagedIndexName is the generated built-in index, relative to .fledge.
const ManagedIndexName = "agents/fledge/" + ManagedFileName

// CatalogName is the generated model catalog, relative to the .fledge
// directory. fledge init regenerates it wholesale from the installed
// integrations; it is never hand-edited, and a user-agents.json entry shadows
// a catalog entry of the same name.
const CatalogName = "agents/fledge/catalog.json"

// Config describes one launchable agent.
type Config struct {
	Integration    string            `json:"integration" yaml:"integration"`
	Model          string            `json:"model,omitempty" yaml:"model,omitempty"`
	Provider       string            `json:"provider,omitempty" yaml:"provider,omitempty"`
	Cwd            string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	PermissionMode string            `json:"permission_mode,omitempty" yaml:"permission_mode,omitempty"`
	Sandbox        string            `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
	Argv           []string          `json:"argv,omitempty" yaml:"argv,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

// Load reads resolved profiles in deterministic user, managed, catalog order.
// Synchronize rejects differing collisions; identical declarations coalesce.
func Load(root string) (map[string]Config, error) {
	configs := map[string]Config{}
	for _, file := range []string{FileName, ManagedIndexName, CatalogName} {
		entries, err := loadFile(root, file)
		if err != nil {
			return nil, err
		}
		for name, cfg := range entries {
			if _, exists := configs[name]; !exists {
				configs[name] = cfg
			}
		}
	}
	return configs, nil
}

// loadFile reads one config file under root's .fledge directory; a missing
// file yields an empty map.
func loadFile(root, file string) (map[string]Config, error) {
	data, err := os.ReadFile(filepath.Join(root, scaffold.DirName, file))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err == nil && idx.Version != 0 {
		if idx.Version != IndexVersion {
			return nil, fmt.Errorf("parse %s: unsupported index version %d", file, idx.Version)
		}
		if idx.Profiles == nil {
			return map[string]Config{}, nil
		}
		return idx.Profiles, nil
	}
	// Compatibility for focused daemon tests that install a flat profile map
	// directly at the canonical generated-index path. Commands synchronize the
	// file from Markdown before loading, so this is not a user-facing format.
	configs := map[string]Config{}
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
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
	return "", "", fmt.Errorf("unknown model %q: reference a configured profile with \"fledge.profile: <name>\" or use a routable model prefix (claude*, gpt*, codex*, o-series, opencode*)", model)
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
	if err := c.ValidateFields(); err != nil {
		return fmt.Errorf("agent %q: %w", name, err)
	}
	return nil
}

// ValidateProfile validates a user-configurable profile name and its fields.
func (c Config) ValidateProfile(name string) error {
	return c.validateProfile(name, false)
}

func (c Config) validateProfile(name string, managed bool) error {
	if err := validPortableName(name); err != nil {
		return err
	}
	if strings.HasPrefix(name, "fledge-") && !managed {
		return fmt.Errorf("profile %q uses the reserved fledge-* namespace", name)
	}
	if err := c.ValidateFields(); err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	return nil
}

// ValidateFields cross-checks integration-specific fields without a name, so
// the daemon can run the same checks on a config assembled from spawn flags.
// permission_mode and sandbox stay separate fields: claude's vocabulary (plan,
// acceptEdits, …) and codex's (read-only, workspace-write, …) don't map 1:1.
func (c Config) ValidateFields() error {
	for _, arg := range c.Argv {
		if arg == "--" {
			return errors.New("argv is option-only and must not contain --")
		}
	}
	if err := validateInteractiveArgv(c.Integration, c.Argv); err != nil {
		return err
	}
	switch c.Integration {
	case "claude":
		if c.Provider != "" {
			return errors.New("provider is pi-only")
		}
		if c.Sandbox != "" {
			return errors.New("sandbox is codex-only")
		}
	case "pi":
		if c.PermissionMode != "" {
			return errors.New("permission_mode is claude-only")
		}
		if c.Sandbox != "" {
			return errors.New("sandbox is codex-only")
		}
	case "codex":
		if c.Provider != "" {
			return errors.New("provider is pi-only")
		}
		if c.PermissionMode != "" {
			return errors.New("permission_mode is claude-only")
		}
	default:
		return fmt.Errorf("invalid integration %q: use \"claude\", \"pi\" or \"codex\"", c.Integration)
	}
	return nil
}

// validateInteractiveArgv rejects options that change the process from the
// live interactive session Fledge launches or seize session/instruction
// ownership. Those modes cannot be combined coherently with Herdr's TUI pane
// and a future owned same-session control channel.
func validateInteractiveArgv(integration string, argv []string) error {
	for i, arg := range argv {
		flag := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			flag = before
		}
		forbidden := false
		switch integration {
		case "claude":
			switch flag {
			case "--print", "-p", "--resume", "-r", "--continue", "-c",
				"--session-id", "--input-format", "--output-format",
				"--append-system-prompt":
				forbidden = true
			}
		case "pi":
			switch flag {
			case "--print", "-p", "--session", "--session-id", "--mode",
				"--rpc", "--append-system-prompt":
				forbidden = true
			}
		case "codex":
			switch flag {
			case "exec", "resume", "app-server":
				forbidden = true
			}
			if flag == "--config" {
				value := ""
				if _, after, ok := strings.Cut(arg, "="); ok {
					value = after
				} else if i+1 < len(argv) {
					value = argv[i+1]
				}
				if strings.HasPrefix(value, "developer_instructions=") {
					forbidden = true
				}
			}
		}
		if forbidden {
			return fmt.Errorf("argv option %q is not supported for %s interactive profiles because Fledge owns session and instruction control", arg, integration)
		}
	}
	return nil
}

// ReservedOrchestrator is the single name exempt from the agent naming rule.
// fledge start brings this profile up on every interactive start and the agent
// runs under this exact string — hyphen included, and with no species suffix —
// so that the one agent an operator always has is the one whose name they
// already know without looking it up.
const ReservedOrchestrator = "fledge-orchestrator"

// validName accepts portable kebab-case agent/profile fixture names.
func validName(name string) error {
	return validPortableName(name)
}

// CommandArgv assembles the full launch argv for the config. sessionID is used
// by the claude integration only; pi and codex persist their own sessions and
// have no equivalent flag. An integration the table does not know yields nil
// rather than a guess.
func (c Config) CommandArgv(sessionID string) []string {
	var argv []string
	switch c.Integration {
	case "claude":
		argv = []string{"claude", "--session-id", sessionID}
		permissionMode := c.PermissionMode
		if permissionMode == "" {
			permissionMode = "bypassPermissions"
		}
		argv = append(argv, "--permission-mode", permissionMode)
	case "pi":
		argv = []string{"pi"}
		if sessionID != "" {
			argv = append(argv, "--session-id", sessionID)
		}
		if c.Provider != "" {
			argv = append(argv, "--provider", c.Provider)
		}
	case "codex":
		argv = []string{"codex"}
		if c.Sandbox != "" {
			argv = append(argv, "--sandbox", c.Sandbox)
		}
	default:
		return nil
	}
	if c.Model != "" {
		argv = append(argv, "--model", c.Model)
	}
	return append(argv, c.Argv...)
}

// LaunchArgv assembles the complete interactive launch command. Profile argv
// is deliberately placed before Fledge's native instruction option so a
// profile cannot override the identity and role assigned to this run. The
// readiness bootstrap is the CLI's initial positional prompt.
func (c Config) LaunchArgv(sessionID, instructions, bootstrap string) []string {
	argv := c.CommandArgv(sessionID)
	if argv == nil {
		return nil
	}
	switch c.Integration {
	case "claude", "pi":
		argv = append(argv, "--append-system-prompt", instructions)
	case "codex":
		encoded, _ := json.Marshal(instructions)
		argv = append(argv, "--config", "developer_instructions="+string(encoded))
	}
	return append(argv, bootstrap)
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
