// Package capabilities implements agent capabilities: reporting the harness
// registry's capability levels, optionally with Herdr's live integration facts.
package capabilities

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/harness"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// Options limits the report to one harness kind; Live adds integration.list facts.
type Options struct {
	Harness string
	Live    bool
}
type Result struct {
	Harnesses []Harness `json:"harnesses"`
	live      bool
}

// Harness is one kind's capability rows; Live is null without --live or when
// Herdr lists no integration target for the kind.
type Harness struct {
	Kind         string        `json:"kind"`
	Capabilities []harness.Row `json:"capabilities"`
	Live         *Live         `json:"live"`
}

// Live is whether Herdr finds the harness binary on PATH and its hook state.
type Live struct {
	Available bool   `json:"available"`
	HookState string `json:"hook_state"`
}

// Run reports capabilities; only Live opens the Herdr socket.
func Run(ctx context.Context, c libagent.Client, o Options) libagent.Outcome {
	out := libagent.Outcome{Operation: "agent.capabilities", Status: "success", Effects: []libagent.Effect{}}
	if o.Harness != "" {
		if err := libagent.ValidateHarness(o.Harness); err != nil {
			out.Fail(err, "validation", false)
			return out
		}
	}
	live := map[string]*Live{}
	if o.Live {
		var list herdr.IntegrationListResult
		err := c.Call(ctx, "integration.list", nil, &list)
		if err == nil && (list.Type != "integration_list" || list.Integrations == nil) {
			err = libagent.Protocol("incomplete integration.list result")
		}
		if err != nil {
			out.Fail(err, "integration.list", false)
			return out
		}
		for _, in := range list.Integrations {
			live[in.Target] = &Live{Available: in.Available, HookState: in.State}
		}
	}
	r := Result{Harnesses: []Harness{}, live: o.Live}
	for _, kind := range harness.Kinds() {
		if o.Harness != "" && kind != o.Harness {
			continue
		}
		r.Harnesses = append(r.Harnesses, Harness{Kind: kind, Capabilities: harness.Capabilities(kind), Live: live[kind]})
	}
	out.Result = r
	return out
}

// Render writes the capability table and, for --live, a per-harness facts table.
func Render(w io.Writer, o libagent.Outcome) error {
	r, ok := o.Result.(Result)
	if o.Error != nil || !ok {
		return nil
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "HARNESS\tCAPABILITY\tLEVEL\tEVIDENCE")
	for _, h := range r.Harnesses {
		for _, c := range h.Capabilities {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", h.Kind, c.Name, c.Level, c.Evidence)
		}
	}
	if err := table.Flush(); err != nil || !r.live {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	fmt.Fprintln(table, "HARNESS\tAVAILABLE\tHOOK_STATE")
	for _, h := range r.Harnesses {
		available, state := "-", "-"
		if h.Live != nil {
			available, state = "no", h.Live.HookState
			if h.Live.Available {
				available = "yes"
			}
		}
		fmt.Fprintf(table, "%s\t%s\t%s\n", h.Kind, available, state)
	}
	return table.Flush()
}
