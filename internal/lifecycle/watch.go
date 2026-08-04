package lifecycle

import (
	"context"
	"errors"

	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/watch"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
)

// Watch runs or attaches to the active session's watcher.
func (m *Manager) Watch(ctx context.Context, dir string, options WatchOptions) error {
	root, err := project.Find(dir)
	if err != nil {
		return err
	}
	config := watch.LoadConfig(root)
	if !config.Enabled {
		return nil
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
		Root: root, Session: value.SessionName, Config: config, Herdr: m.herdr,
		Daemon: options.Daemon, Output: m.output,
		Deliver: func(deliveryCtx context.Context, body string) (string, error) {
			message, err := m.SendWatcherWake(deliveryCtx, root, body)
			return message.ID, err
		},
	})
}
