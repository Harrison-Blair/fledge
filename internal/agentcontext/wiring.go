package agentcontext

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// openCodeExportTimeout bounds the export edge so a hung harness cannot block a
// context report indefinitely.
const openCodeExportTimeout = 15 * time.Second

// ProductionDeps wires the real side effects: the system clock, filesystem
// reads and globbing, and the bounded OpenCode export command. ctx bounds the
// OpenCode edge alongside its own timeout.
func ProductionDeps(ctx context.Context, home string) Deps {
	return Deps{
		Home:     home,
		Now:      func() time.Time { return time.Now().UTC() },
		ReadFile: os.ReadFile,
		Glob:     filepath.Glob,
		OpenCodeExport: func(sessionID string) ([]byte, error) {
			return runOpenCodeExport(ctx, sessionID)
		},
	}
}

func runOpenCodeExport(ctx context.Context, sessionID string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, openCodeExportTimeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(runCtx, "opencode", "export", "--pure", "--sanitize", sessionID)
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		message := stderr.String()
		if message == "" {
			return nil, fmt.Errorf("export OpenCode session: %w", err)
		}
		return nil, fmt.Errorf("export OpenCode session: %w: %s", err, message)
	}
	return stdout.Bytes(), nil
}
