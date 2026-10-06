package board

import (
	"context"
	"io"

	tea "charm.land/bubbletea/v2"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"golang.org/x/term"
)

// Run checks both supplied terminal streams before loading state, then runs the
// board. Bubble Tea owns terminal restoration; all background work is cancelled
// before returning on quit, external cancellation, or runtime failure.
func Run(ctx context.Context, c libagent.Client, in io.Reader, out io.Writer) libagent.Outcome {
	return run(ctx, c, in, out, isTerminal, func(m tea.Model, in io.Reader, out io.Writer, ctx context.Context) error {
		// Bubble Tea's forced context teardown can close its cancel reader
		// before the input goroutine exits. Send a graceful quit instead;
		// the model's work context still cancels IO immediately.
		p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(context.WithoutCancel(ctx)))
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				p.Send(tea.QuitMsg{})
			case <-done:
			}
		}()
		_, err := p.Run()
		return err
	})
}
func isTerminal(stream any) bool {
	fd, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(fd.Fd()))
}
func run(ctx context.Context, c libagent.Client, in io.Reader, out io.Writer, terminal func(any) bool, program func(tea.Model, io.Reader, io.Writer, context.Context) error) libagent.Outcome {
	result := libagent.Outcome{Operation: "task.board", Status: "success", Effects: []libagent.Effect{}}
	fail := func(err error, phase string) libagent.Outcome {
		result.Fail(err, phase, false)
		result.Error.Message = singleLine(result.Error.Message)
		return result
	}
	if !terminal(in) || !terminal(out) {
		return fail(libagent.Invalid("task board requires terminal input and output"), "validation")
	}
	r, err := OpenRepository(ctx, c.Cwd)
	if err != nil {
		return fail(err, "state")
	}
	initial := Load(ctx, c, r, Tasks)
	if initial.Err != nil {
		return fail(initial.Err, "state")
	}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = program(newModel(workCtx, cancel, c, r, initial.Snapshot), in, out, ctx)
	if err != nil && ctx.Err() == nil {
		return fail(err, "terminal")
	}
	return result
}
