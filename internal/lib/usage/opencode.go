package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// opencodeTimeout bounds opencode export, which starts the opencode runtime.
const opencodeTimeout = 30 * time.Second

type opencodeExport struct {
	Info     *json.RawMessage `json:"info"`
	Messages *[]struct {
		Info struct {
			Role       string  `json:"role"`
			ModelID    string  `json:"modelID"`
			ProviderID string  `json:"providerID"`
			Cost       float64 `json:"cost"`
			Tokens     *struct {
				Input     int64 `json:"input"`
				Output    int64 `json:"output"`
				Reasoning int64 `json:"reasoning"`
				Cache     struct {
					Read  int64 `json:"read"`
					Write int64 `json:"write"`
				} `json:"cache"`
			} `json:"tokens"`
			Time struct {
				Created int64 `json:"created"`
			} `json:"time"`
		} `json:"info"`
	} `json:"messages"`
}

// readOpencode sums assistant messages from opencode export; it never opens
// opencode's database, which also holds account tokens.
func readOpencode(ctx context.Context, d Discovery, ref Ref, w Window) (*tally, error) {
	if ref.Kind != "id" || ref.Value == "" {
		return nil, fmt.Errorf("opencode needs a session id ref, got %q", ref.Kind)
	}
	ctx, cancel := context.WithTimeout(ctx, opencodeTimeout)
	defer cancel()
	source := "opencode export " + ref.Value
	out, err := d.Run(ctx, "opencode", "export", ref.Value)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", source, err)
	}
	var export opencodeExport
	if err := json.Unmarshal(out, &export); err != nil {
		return nil, fmt.Errorf("%s output is not the observed export shape: %w", source, err)
	}
	if export.Info == nil || export.Messages == nil {
		return nil, fmt.Errorf("%s output lacks info and messages; not the observed export shape", source)
	}
	t := &tally{cost: &Cost{Currency: "USD", Basis: "estimate", Source: "opencode"}, sources: []string{source}, recognized: true}
	for _, m := range *export.Messages {
		i := m.Info
		if i.Role != "assistant" {
			continue
		}
		t.lines++
		if i.Tokens == nil {
			t.malformed++
			continue
		}
		t.records++
		var at *time.Time
		if i.Time.Created > 0 {
			created := time.UnixMilli(i.Time.Created).UTC()
			at = &created
		}
		model := i.ModelID
		if i.ProviderID != "" && model != "" {
			model = i.ProviderID + "/" + model
		}
		t.add(w, response{at: at, model: model, cost: i.Cost, tokens: Tokens{
			Input: i.Tokens.Input, Output: i.Tokens.Output, Reasoning: i.Tokens.Reasoning,
			CacheRead: i.Tokens.Cache.Read, CacheWrite: i.Tokens.Cache.Write}})
	}
	if err := t.check(source, "opencode"); err != nil {
		return nil, err
	}
	return t, nil
}
