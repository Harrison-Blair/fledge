package spawn

import (
	"context"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/profiles"
)

// profileBrief is p's brief followed by the project memory block of dir,
// the directory the agent was placed in.
func profileBrief(ctx context.Context, p profiles.Profile, dir string) string {
	return p.Brief() + "\n" + memoryBrief(ctx, dir)
}

// firstPrompt places a profile brief before the task body, separated by a
// blank line, omitting whichever is absent.
func firstPrompt(brief, body string) string {
	switch {
	case brief == "":
		return body
	case body == "":
		return brief
	}
	return brief + "\n\n" + body
}

// memoryBrief is the Project memory block for a worker placed in dir: the
// index of its repository's memories, a note that there are none, or why they
// could not be read. It is read at spawn time and never fails a spawn; a dir
// outside any repository has no memories.
func memoryBrief(ctx context.Context, dir string) string {
	text := "No project memories yet."
	memories, err := memory.Dir(ctx, dir)
	var ms []memory.Memory
	if err == nil {
		ms, err = memory.List(memories)
	}
	switch {
	case err != nil && memories != "":
		text = "Project memory could not be read: " + err.Error()
	case len(ms) > 0:
		text = "Read one in full with `fledge memory get --name <name>`.\n\n" + strings.TrimSuffix(memory.Index(ms), "\n")
	}
	return "## Project memory\n" + text
}
