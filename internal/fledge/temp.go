package fledge

import (
	"context"
	"fmt"

	"github.com/Harrison-Blair/fledge/internal/project"
)

// TempCleanResult identifies the project-local directory reset by CleanTemp.
type TempCleanResult struct {
	ProjectRoot string `json:"project_root"`
	TempDir     string `json:"temp_dir"`
}

// CleanTemp resets disposable project files only when the deterministic
// project session is stopped. A session reported as running is treated as in
// use even when its socket is temporarily unreachable.
func (s *Service) CleanTemp(ctx context.Context) (TempCleanResult, error) {
	s.Binary.TempDir = project.TempDir(s.Project.Root)
	session, found, err := s.Binary.FindSession(ctx, s.Project.Session)
	if err != nil {
		return TempCleanResult{}, Wrap("herdr_discovery_failed", err.Error(), err)
	}
	if found && session.Running {
		return TempCleanResult{}, NewError("temp_in_use",
			fmt.Sprintf("project temp directory is in use by running Fledge session %q; stop it before cleaning", s.Project.Session))
	}
	tempDir, err := project.ResetTempDir(s.Project.Root)
	if err != nil {
		return TempCleanResult{}, Wrap("temp_clean_failed", fmt.Sprintf("clean project temp directory: %v", err), err)
	}
	return TempCleanResult{ProjectRoot: s.Project.Root, TempDir: tempDir}, nil
}
