package usage

import (
	"context"
	"encoding/json"
	"time"
)

type piLine struct {
	Type      string     `json:"type"`
	Timestamp *time.Time `json:"timestamp"`
	Message   struct {
		Role     string `json:"role"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Usage    *struct {
			Input      int64 `json:"input"`
			Output     int64 `json:"output"`
			CacheRead  int64 `json:"cacheRead"`
			CacheWrite int64 `json:"cacheWrite"`
			Reasoning  int64 `json:"reasoning"`
			Cost       struct {
				Total float64 `json:"total"`
			} `json:"cost"`
		} `json:"usage"`
	} `json:"message"`
}

// readPi sums assistant message usage and pi's own cost estimate.
func readPi(_ context.Context, d Discovery, ref Ref, w Window) (*tally, error) {
	path, err := Locate(d, "pi", ref)
	if err != nil {
		return nil, err
	}
	t := &tally{cost: &Cost{Currency: "USD", Basis: "estimate", Source: "pi price table"}}
	err = t.scanLines(path, func(line []byte) error {
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &head); err != nil || head.Type != "message" {
			return err
		}
		var l piLine
		if err := json.Unmarshal(line, &l); err != nil || l.Message.Role != "assistant" || l.Message.Usage == nil {
			return err
		}
		u := l.Message.Usage
		model := l.Message.Model
		if l.Message.Provider != "" && model != "" {
			model = l.Message.Provider + "/" + model
		}
		t.add(w, response{at: l.Timestamp, model: model, cost: u.Cost.Total,
			tokens: Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Reasoning: u.Reasoning}})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}
