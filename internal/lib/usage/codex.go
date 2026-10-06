package usage

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/harnessenv"
)

// errAmbiguousCodex reports local totals followed by a fork's inherited total,
// whose histories cannot be reconciled.
var errAmbiguousCodex = errors.New("codex usage is ambiguous: local token totals precede the inherited parent total")

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

// readCodex sums token_usage_record entries deduplicated by response_id.
// Cumulative token_count totals before the first valid record (all of them in
// a rollout without records) count as the last one inside the window minus the
// last one before it; totals after that first record are ignored. In a fork,
// whose first session_meta names a parent_thread_id, totals before the first
// valid turn_context are the parent's: they are a baseline, never usage. A
// local total followed by such an inherited total makes usage unavailable.
func readCodex(_ context.Context, d harnessenv.Env, ref Ref, w Window) (*tally, error) {
	path, err := Locate(d, "codex", ref)
	if err != nil {
		return nil, err
	}
	t := &tally{}
	seen := map[string]bool{}
	var model string
	var fallbackModels []string
	var before, inside *codexUsage
	var sawMeta, sawContext, inherited, sawTotal, transitioned, ambiguous bool
	err = t.scanLines(path, func(line []byte) error {
		var l codexLine
		if err := json.Unmarshal(line, &l); err != nil {
			return err
		}
		switch l.Type {
		case "session_meta":
			t.recognized = true
			if sawMeta {
				return nil
			}
			sawMeta = true
			// Undecodable optional metadata leaves the rollout unmarked.
			var p struct {
				ParentThreadID string `json:"parent_thread_id"`
			}
			inherited = !sawContext && json.Unmarshal(l.Payload, &p) == nil && p.ParentThreadID != ""
		case "turn_context":
			var p struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(l.Payload, &p); err != nil {
				return err
			}
			model = p.Model
			sawContext, inherited = true, false
			if !transitioned && w.contains(l.Timestamp) && model != "" {
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
			transitioned = true
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
			if transitioned {
				return nil
			}
			total := *p.Info.Total
			if inherited {
				ambiguous = ambiguous || sawTotal
				before = &total
				return nil
			}
			sawTotal = true
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
	if ambiguous {
		return nil, errAmbiguousCodex
	}
	if !sawTotal || transitioned && inside == nil {
		return t, nil
	}
	t.note = "no token_usage_record entries; totals from the last token_count total_token_usage"
	if transitioned {
		t.note = "includes token_count total_token_usage from before the first token_usage_record"
	}
	if inside != nil {
		if before != nil {
			*inside = inside.minus(*before)
		}
		t.tokens.add(inside.tokens())
	}
	// Legacy models precede the token_usage_record models.
	models := t.models
	t.models = nil
	for _, m := range append(fallbackModels, models...) {
		if !slices.Contains(t.models, m) {
			t.models = append(t.models, m)
		}
	}
	return t, nil
}
