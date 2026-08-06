package lifecycle

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/watchproc"
	"github.com/charmbracelet/x/term"
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
		JSON: options.JSON, Color: colorEnabled(m.output, options.JSON, m.getenv,
			func(file *os.File) bool { return term.IsTerminal(file.Fd()) }),
	})
}

// colorEnabled reports whether the human trace may use ANSI: never for machine
// output, never when NO_COLOR asks for none, and only when the trace is going
// to a terminal rather than a file or a pipe.
func colorEnabled(output io.Writer, jsonMode bool, getenv func(string) string, isTerminal func(*os.File) bool) bool {
	if jsonMode || getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := output.(*os.File)
	return ok && isTerminal(file)
}
