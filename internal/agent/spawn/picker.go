package spawn

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// Picker interactively chooses spawn options on a terminal.
type Picker struct {
	In  io.Reader
	Out io.Writer
	// Models lists discovered model IDs for a harness; an empty result offers only the harness default.
	Models func(ctx context.Context, harness string) []string
	// CallerTab resolves the caller's tab ID for split placements, after every prompt is answered.
	CallerTab func(ctx context.Context) (string, error)
}

// CallerTab resolves the tab containing the caller's pane.
func CallerTab(ctx context.Context, c libagent.Client) (string, error) {
	p, err := (&spawner{Client: c}).caller(ctx)
	return p.TabID, err
}

var errCanceled = libagent.Invalid("spawn canceled")

// Pick prompts for harness, model, name, and placement, fills them into base,
// and prints the equivalent flag form. EOF or a canceled context returns an
// input error before any Herdr call.
func (p Picker) Pick(ctx context.Context, base Options) (Options, error) {
	lines := make(chan string)
	go func() {
		defer close(lines)
		r := bufio.NewReader(p.In)
		for {
			// A read ending in EOF cancels, even with a partial final answer.
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			select {
			case lines <- strings.TrimSpace(line):
			case <-ctx.Done():
				return
			}
		}
	}()
	ask := func(prompt string) (string, error) {
		fmt.Fprint(p.Out, prompt)
		select {
		case line, ok := <-lines:
			if !ok {
				fmt.Fprintln(p.Out)
				return "", errCanceled
			}
			return line, nil
		case <-ctx.Done():
			fmt.Fprintln(p.Out)
			return "", errCanceled
		}
	}
	choose := func(title string, choices []string, def int) (int, error) {
		fmt.Fprintf(p.Out, "%s:\n", title)
		for i, c := range choices {
			fmt.Fprintf(p.Out, "  %d) %s\n", i+1, c)
		}
		for {
			answer, err := ask(fmt.Sprintf("%s [%s]: ", title, choices[def]))
			if err != nil {
				return 0, err
			}
			if answer == "" {
				return def, nil
			}
			if i := slices.Index(choices, answer); i >= 0 {
				return i, nil
			}
			if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(choices) {
				return n - 1, nil
			}
			fmt.Fprintf(p.Out, "choose a number from 1 to %d\n", len(choices))
		}
	}

	o := base
	harnesses := libagent.Harnesses()
	i, err := choose("Harness", harnesses, slices.Index(harnesses, "claude"))
	if err != nil {
		return base, err
	}
	o.Harness = harnesses[i]

	models := []string{"harness default"}
	if _, err := modelArguments(o.Harness, "probe", nil); err == nil {
		found := p.Models(ctx, o.Harness)
		slices.Sort(found)
		models = append(models, slices.Compact(found)...)
	}
	if len(models) == 1 {
		fmt.Fprintln(p.Out, "Model: harness default")
	} else if i, err = choose("Model", models, 0); err != nil {
		return base, err
	} else if i > 0 {
		o.Model = models[i]
	}

	for {
		if o.Name, err = ask("Name: "); err != nil {
			return base, err
		}
		if o.Name == "" {
			fmt.Fprintln(p.Out, "name is required")
			continue
		}
		if _, err := o.Validate(); err != nil {
			fmt.Fprintln(p.Out, err)
			continue
		}
		break
	}

	flags := []string{"--harness", o.Harness}
	if o.Model != "" {
		flags = append(flags, "--model", o.Model)
	}
	flags = append(flags, "--name", o.Name)
	placements := []string{"new tab", "split right", "split down", "new worktree"}
	if i, err = choose("Placement", placements, 0); err != nil {
		return base, err
	}
	switch i {
	case 1, 2:
		if o.TabID, err = p.CallerTab(ctx); err != nil {
			return base, err
		}
		o.Direction, o.DirectionSet = strings.TrimPrefix(placements[i], "split "), true
		flags = append(flags, "--tab-id", o.TabID, "--direction", o.Direction)
	case 3:
		o.Worktree = "new"
		for {
			if o.Branch, err = ask(fmt.Sprintf("Branch [%s]: ", o.Name)); err != nil {
				return base, err
			}
			if o.Branch == "" {
				o.Branch = o.Name
			}
			if _, err := o.Validate(); err != nil {
				fmt.Fprintln(p.Out, err)
				continue
			}
			break
		}
		flags = append(flags, "--worktree", "new", "--branch", o.Branch)
	}
	for i, f := range flags {
		if strings.ContainsFunc(f, func(r rune) bool {
			return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:=@+,", r)
		}) {
			flags[i] = "'" + strings.ReplaceAll(f, "'", `'\''`) + "'"
		}
	}
	if _, err := o.Validate(); err != nil {
		return base, err
	}
	fmt.Fprintf(p.Out, "fledge agent spawn %s\n", strings.Join(flags, " "))
	return o, nil
}
