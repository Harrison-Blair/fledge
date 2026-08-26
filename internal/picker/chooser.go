package picker

import (
	"context"
	"fmt"
	"io"

	"fledge/internal/catalog"
	"fledge/internal/session"
)

const (
	harnessPrompt  = "Start an agent in the orchestrator pane?"
	shellOnlyTitle = "none — shell only"
	defaultTitle   = "harness default"
	freeTextTitle  = "enter model ID"
)

// AgentChooser asks which harness and model the orchestrator pane should run.
// Terminal detection is supplied by the CLI boundary so this package does not
// depend on a particular file-descriptor implementation.
type AgentChooser struct {
	Input            io.Reader
	Output           io.Writer
	InputIsTerminal  bool
	OutputIsTerminal bool
	// Models reports the model IDs a harness accepts.
	Models func(context.Context, catalog.Harness) []string

	// selectFn replaces Select in tests.
	selectFn func(io.Reader, io.Writer, string, []Option) (Option, error)
}

// Choose presents the harness list, then the model list for the chosen
// harness. Choosing shell only reports an empty harness and skips the model
// step. It reports ErrCancelled when the user dismisses either list.
func (c AgentChooser) Choose(ctx context.Context) (session.AgentChoice, error) {
	if !c.InputIsTerminal || !c.OutputIsTerminal {
		return session.AgentChoice{}, fmt.Errorf("agent selection requires terminal input and output")
	}
	if c.Input == nil {
		return session.AgentChoice{}, fmt.Errorf("agent selection input is nil")
	}
	if c.Output == nil {
		return session.AgentChoice{}, fmt.Errorf("agent selection output is nil")
	}
	if c.Models == nil {
		return session.AgentChoice{}, fmt.Errorf("agent selection model lookup is nil")
	}

	harnesses := catalog.Harnesses()
	// The harness lists are slow enough to notice, so fetch them all while the
	// first question is on screen and await only the one that is chosen.
	pending := make(map[catalog.Harness]chan []string, len(harnesses))
	for _, harness := range harnesses {
		models := make(chan []string, 1)
		pending[harness] = models
		go func() { models <- c.Models(ctx, harness) }()
	}

	options := make([]Option, 0, len(harnesses)+1)
	for _, harness := range harnesses {
		options = append(options, Option{ID: string(harness), Title: string(harness)})
	}
	options = append(options, Option{Title: shellOnlyTitle})

	harness, err := c.selectOne(harnessPrompt, options)
	if err != nil {
		return session.AgentChoice{}, err
	}
	if harness.ID == "" {
		return session.AgentChoice{}, nil
	}

	var models []string
	select {
	case models = <-pending[catalog.Harness(harness.ID)]:
	case <-ctx.Done():
		return session.AgentChoice{}, ctx.Err()
	}

	modelOptions := make([]Option, 0, len(models)+2)
	modelOptions = append(modelOptions,
		Option{Title: defaultTitle},
		Option{Title: freeTextTitle, FreeText: true},
	)
	for _, id := range models {
		modelOptions = append(modelOptions, Option{ID: id, Title: id})
	}

	model, err := c.selectOne("Model for "+harness.ID, modelOptions)
	if err != nil {
		return session.AgentChoice{}, err
	}
	return session.AgentChoice{Harness: harness.ID, Model: model.ID}, nil
}

func (c AgentChooser) selectOne(title string, options []Option) (Option, error) {
	choose := c.selectFn
	if choose == nil {
		choose = Select
	}
	return choose(c.Input, c.Output, title, options)
}
