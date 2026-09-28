package spawn

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/memory"
	"github.com/Harrison-Blair/fledge/internal/lib/profiles"
)

// applyProfile fills launch settings the caller left unset from p. An explicit
// harness that differs from the profile's drops the profile's model and args,
// which are specific to its harness; an explicit model or explicit native
// arguments replace the profile's.
func applyProfile(o Options, p profiles.Profile) Options {
	crossHarness := o.Harness != "" && p.Harness != "" && o.Harness != p.Harness
	if o.Harness == "" {
		o.Harness = p.Harness
	}
	if crossHarness {
		return o
	}
	if o.Model == "" {
		o.Model = p.Model
	}
	if len(o.Args) == 0 {
		o.Args = append([]string{}, p.Args...)
	}
	return o
}

// profileBrief renders p with only the reads present under dir, returning
// the missing reads. A missing read never fails a spawn. A protocol profile's
// brief ends with dir's project memory block.
func profileBrief(ctx context.Context, p profiles.Profile, dir string) (string, []string) {
	var present, missing []string
	for _, r := range p.Reads {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(r))); err != nil {
			missing = append(missing, r)
		} else {
			present = append(present, r)
		}
	}
	p.Reads = present
	brief := p.Brief()
	if p.Protocol {
		sep := "\n\n"
		if strings.HasSuffix(brief, "\n") {
			sep = "\n"
		}
		brief += sep + memoryBrief(ctx, dir)
	}
	return brief, missing
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
