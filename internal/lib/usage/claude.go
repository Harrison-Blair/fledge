package usage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// claudeSlug is claude's project directory name for cwd.
func claudeSlug(cwd string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(cwd)
}

type claudeLine struct {
	Type      string     `json:"type"`
	RequestID string     `json:"requestId"`
	Timestamp *time.Time `json:"timestamp"`
	Message   struct {
		Model string `json:"model"`
		Usage *struct {
			Input      int64 `json:"input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			Output     int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// readClaude sums a transcript and its sub-agent transcripts.
func readClaude(_ context.Context, d Discovery, ref Ref, w Window) (*tally, error) {
	path, err := Locate(d, "claude", ref)
	if err != nil {
		return nil, err
	}
	t, err := readClaudeFile(path, w)
	if err != nil {
		return nil, err
	}
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	for _, sub := range subs {
		st, err := readClaudeFile(sub, w)
		if err != nil {
			return nil, err
		}
		t.merge(st)
	}
	return t, nil
}

// readClaudeFile keeps one response per requestId: a request spans several
// assistant lines, and its last line carries the final output count. Claude's
// "<synthetic>" placeholder lines come from no model and are skipped.
func readClaudeFile(path string, w Window) (*tally, error) {
	t := &tally{}
	byRequest := map[string]int{}
	var responses []response
	err := t.scanLines(path, func(line []byte) error {
		var head struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			return err
		}
		t.recognized = t.recognized || head.SessionID != "" || head.Type == "user" || head.Type == "assistant"
		if head.Type != "assistant" {
			return nil
		}
		var l claudeLine
		if err := json.Unmarshal(line, &l); err != nil || l.Message.Usage == nil || l.Message.Model == "<synthetic>" {
			return err
		}
		t.records++
		u := l.Message.Usage
		r := response{at: l.Timestamp, model: l.Message.Model, tokens: Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}}
		if i, ok := byRequest[l.RequestID]; ok && l.RequestID != "" {
			responses[i] = r
			return nil
		}
		byRequest[l.RequestID] = len(responses)
		responses = append(responses, r)
		return nil
	})
	if err == nil {
		err = t.check(path, "claude")
	}
	if err != nil {
		return nil, err
	}
	for _, r := range responses {
		t.add(w, r)
	}
	return t, nil
}
