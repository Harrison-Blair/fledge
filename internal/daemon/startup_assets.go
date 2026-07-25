package daemon

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/agentcfg"
	"github.com/Harrison-Blair/fledge/internal/flock"
)

const orchestratorRuntimeDir = "runtime/fledge-orchestrator"

const claudePluginManifest = `{
  "name": "fledge-orchestrator-readiness",
  "description": "Authenticates the managed Fledge orchestrator at session startup",
  "version": "1.0.0"
}
`

const claudeHooks = `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "\"${CLAUDE_PLUGIN_ROOT}/ready.sh\""
          }
        ]
      }
    ]
  }
}
`

const claudeReadyScript = `#!/bin/sh
result=$(fledge agent ready --no-wait 2>&1)
status=$?
if [ "$status" -eq 0 ]; then
	printf 'Fledge readiness succeeded: %s\n' "$result"
else
	printf 'Fledge readiness failed (exit %s): %s\n' "$status" "$result"
fi
exit 0
`

const piReadyExtension = `import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export default function (pi: ExtensionAPI) {
	const record = (content: string, details: unknown) => {
		pi.sendMessage(
			{
				customType: "fledge-readiness",
				content,
				display: true,
				details,
			},
			{ deliverAs: "nextTurn", triggerTurn: false },
		);
	};

	pi.on("session_start", async (event) => {
		if (event.reason !== "startup") return;

		try {
			const result = await pi.exec("fledge", ["agent", "ready", "--no-wait"]);
			const output = [result.stdout.trim(), result.stderr.trim()].filter(Boolean).join("\n");
			const content =
				result.code === 0
					? "Fledge readiness succeeded" + (output ? ": " + output : ".")
					: "Fledge readiness failed (exit " + result.code + ")" + (output ? ": " + output : ".");
			record(content, result);
		} catch (error) {
			const detail = error instanceof Error ? error.message : String(error);
			record("Fledge readiness failed: " + detail, { error: detail });
		}
	});
}
`

// orchestratorStartupArgs publishes the integration-owned startup automation
// before Herdr can launch the managed orchestrator. Each file is replaced
// atomically; a partial set is harmless because its argv is not returned until
// every required file has landed.
func (d *Daemon) orchestratorStartupArgs(integration string) ([]string, error) {
	base := filepath.Join(flock.Dir(d.root, d.flockName), filepath.FromSlash(orchestratorRuntimeDir))
	switch integration {
	case "claude":
		plugin := filepath.Join(base, "claude")
		files := []struct {
			name string
			body string
			mode os.FileMode
		}{
			{filepath.Join(plugin, ".claude-plugin", "plugin.json"), claudePluginManifest, 0o600},
			{filepath.Join(plugin, "hooks", "hooks.json"), claudeHooks, 0o600},
			{filepath.Join(plugin, "ready.sh"), claudeReadyScript, 0o700},
		}
		for _, file := range files {
			if err := writeRuntimeFile(file.name, []byte(file.body), file.mode); err != nil {
				return nil, fmt.Errorf("write Claude orchestrator readiness plugin: %w", err)
			}
		}
		return []string{"--plugin-dir", plugin}, nil
	case "pi":
		extension := filepath.Join(base, "pi", "readiness.ts")
		if err := writeRuntimeFile(extension, []byte(piReadyExtension), 0o600); err != nil {
			return nil, fmt.Errorf("write Pi orchestrator readiness extension: %w", err)
		}
		return []string{"--extension", extension}, nil
	default:
		return nil, nil
	}
}

func writeRuntimeFile(name string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".fledge-runtime-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, name); err != nil {
		return err
	}
	keep = true
	return nil
}

func orchestratorUsesStartupAsset(agentType string, cfg agentcfg.Config) bool {
	return agentType == agentcfg.ReservedOrchestrator && (cfg.Integration == "claude" || cfg.Integration == "pi")
}
