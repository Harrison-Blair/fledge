package agentcontext

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// collectPi reads Pi's JSONL transcript. Pi stores one file per session at
// ~/.pi/agent/sessions/<slug>/<timestamp>_<session-id>.jsonl; the session id is
// the file-name suffix after the timestamp, so a glob on "*_<id>.jsonl" locates
// it by exact id.
//
// used = usage.input + usage.cacheRead + usage.cacheWrite, taken from the last
// assistant message. Pi's usage.totalTokens is not used because it folds in
// output and reasoning tokens, which are not part of the carried context. Pi
// does not record a context window in its transcript, so the window (and hence
// percent) is left null. Pi's transcript exposes no authoritative compaction
// boundary record, so after_compaction does not apply to it.
func collectPi(ref Ref, deps Deps) (reading, error) {
	contents, err := readTranscript(ref, deps, filepath.Join(deps.Home, ".pi", "agent", "sessions", "*", "*_"+ref.Value+".jsonl"))
	if err != nil {
		return reading{}, err
	}
	if r, ok := lastPiUsage(contents); ok {
		return r, nil
	}
	return reading{}, errAwaitingFirstResponse
}

type piEntry struct {
	Type    string `json:"type"`
	Message struct {
		Role      string          `json:"role"`
		Timestamp json.RawMessage `json:"timestamp"`
		Usage     *struct {
			Input      int `json:"input"`
			CacheRead  int `json:"cacheRead"`
			CacheWrite int `json:"cacheWrite"`
		} `json:"usage"`
	} `json:"message"`
}

func lastPiUsage(contents []byte) (reading, bool) {
	var (
		found  bool
		result reading
	)
	for _, line := range splitLines(contents) {
		var entry piEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Message.Role != "assistant" || entry.Message.Usage == nil {
			continue
		}
		usage := entry.Message.Usage
		result = reading{used: usage.Input + usage.CacheRead + usage.CacheWrite}
		if ts, ok := parsePiTime(entry.Message.Timestamp); ok {
			result.observedAt = ts
			result.hasObservedAt = true
		}
		found = true
	}
	return result, found
}

func parsePiTime(raw json.RawMessage) (time.Time, bool) {
	var milliseconds int64
	if err := json.Unmarshal(raw, &milliseconds); err == nil && milliseconds > 0 {
		return time.UnixMilli(milliseconds).UTC(), true
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return time.Time{}, false
	}
	return parseTime(value)
}
