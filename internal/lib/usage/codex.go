package usage

import (
	"context"
	"encoding/json"
	"slices"
	"time"
)

type codexLine struct {
	Timestamp *time.Time      `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// codexUsage is codex's token usage; input includes cache reads and writes,
// and output includes reasoning.
type codexUsage struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
}

func (u codexUsage) tokens() Tokens {
	return Tokens{Input: u.Input - u.Cached - u.CacheWrite, Output: u.Output, CacheRead: u.Cached, CacheWrite: u.CacheWrite, Reasoning: u.Reasoning}
}

func (u codexUsage) minus(o codexUsage) codexUsage {
	return codexUsage{u.Input - o.Input, u.Cached - o.Cached, u.CacheWrite - o.CacheWrite, u.Output - o.Output, u.Reasoning - o.Reasoning}
}

// readCodex sums token_usage_record entries deduplicated by response_id. A
// rollout without them falls back to cumulative token_count totals: the last
// one inside the window minus the last one before it.
func readCodex(_ context.Context, d Discovery, ref Ref, w Window) (*tally, error) {
	path, err := Locate(d, "codex", ref)
	if err != nil {
		return nil, err
	}
	t := &tally{}
	seen := map[string]bool{}
	var model string
	var fallbackModels []string
	var before, inside *codexUsage
	var sawTotal bool
	err = t.scanLines(path, func(line []byte) error {
		var l codexLine
		if err := json.Unmarshal(line, &l); err != nil {
			return err
		}
		switch l.Type {
		case "session_meta":
			t.recognized = true
		case "turn_context":
			var p struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(l.Payload, &p); err != nil {
				return err
			}
			model = p.Model
			if w.contains(l.Timestamp) && model != "" {
				fallbackModels = append(fallbackModels, model)
			}
		case "token_usage_record":
			var p struct {
				ResponseID string      `json:"response_id"`
				Usage      *codexUsage `json:"usage"`
			}
			if err := json.Unmarshal(l.Payload, &p); err != nil {
				return err
			}
			if p.Usage == nil {
				return errMissingUsage
			}
			t.records++
			if p.ResponseID != "" && seen[p.ResponseID] {
				return nil
			}
			seen[p.ResponseID] = true
			t.add(w, response{at: l.Timestamp, model: model, tokens: p.Usage.tokens()})
		case "event_msg":
			var p struct {
				Type string `json:"type"`
				Info *struct {
					Total *codexUsage `json:"total_token_usage"`
				} `json:"info"`
			}
			if err := json.Unmarshal(l.Payload, &p); err != nil {
				return err
			}
			// Codex writes a null info before any usage exists.
			if p.Type != "token_count" || p.Info == nil {
				return nil
			}
			if p.Info.Total == nil {
				return errMissingUsage
			}
			t.records++
			sawTotal = true
			total := *p.Info.Total
			switch {
			case w.From != nil && l.Timestamp != nil && l.Timestamp.Before(*w.From):
				before = &total
			case w.contains(l.Timestamp):
				inside = &total
			}
		}
		return nil
	})
	if err == nil {
		err = t.check(path, "codex")
	}
	if err != nil {
		return nil, err
	}
	if len(seen) == 0 && sawTotal {
		t.note = "no token_usage_record entries; totals from the last token_count total_token_usage"
		if inside != nil {
			if before != nil {
				*inside = inside.minus(*before)
			}
			t.tokens = inside.tokens()
		}
		for _, m := range fallbackModels {
			if !slices.Contains(t.models, m) {
				t.models = append(t.models, m)
			}
		}
	}
	return t, nil
}
