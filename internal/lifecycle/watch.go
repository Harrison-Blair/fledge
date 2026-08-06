package lifecycle

import (
	"context"
	"errors"

	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// Watch runs or attaches to the active session's watcher.
func (m *Manager) Watch(ctx context.Context, dir string, options WatchOptions) error {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	if err := project.EnsureRuntimeIgnore(root); err != nil {
		return err
	}
	if err := m.herdr.Check(); err != nil {
		return err
	}
	value, found, err := readRecord(root)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("project has no Fledge session; run fledge start first")
	}
	return m.watchRunner(ctx, watchproc.Options{
		Root: root, Session: value.SessionName, Herdr: m.herdr,
		Daemon: options.Daemon, Output: m.output,
	})
}
