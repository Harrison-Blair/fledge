package agentcontext

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// collectClaude reads Claude Code's JSONL transcript. Claude stores one file
// per session at ~/.claude/projects/<slug>/<session-id>.jsonl; because the
// session id is globally unique we locate it by glob rather than deriving the
// project slug from a cwd.
//
// used = input_tokens + cache_creation_input_tokens + cache_read_input_tokens
// from the last assistant usage block. Output tokens are excluded: they are the
// model's reply, not context the next turn carries.
//
// Compaction is a boundary, not a transparent shrink. Claude writes a
// system/compact_boundary record; if that boundary is the latest relevant event
// after the last usage block, the pre-compaction figure is stale and no
// post-compaction response has landed, so the collector reports after_compaction
// until a fresh usage block appears.
func collectClaude(ref Ref, deps Deps) (reading, error) {
	contents, err := readTranscript(ref, deps, filepath.Join(deps.Home, ".claude", "projects", "*", ref.Value+".jsonl"))
	if err != nil {
		return reading{}, err
	}
	return scanClaude(contents)
}

type claudeEntry struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Message struct {
		Model string `json:"model"`
		Usage *struct {
			InputTokens         int `json:"input_tokens"`
			CacheCreationTokens int `json:"cache_creation_input_tokens"`
			CacheReadTokens     int `json:"cache_read_input_tokens"`
			OutputTokens        int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Timestamp string `json:"timestamp"`
}

func scanClaude(contents []byte) (reading, error) {
	var (
		result      reading
		lastUsage   = -1
		lastCompact = -1
		haveUsage   bool
	)
	for index, line := range splitLines(contents) {
		var entry claudeEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "system" && entry.Subtype == "compact_boundary" {
			lastCompact = index
			continue
		}
		if entry.Message.Usage == nil {
			continue
		}
		usage := entry.Message.Usage
		window, hasWindow := claudeWindow(entry.Message.Model)
		result = reading{
			used:      usage.InputTokens + usage.CacheCreationTokens + usage.CacheReadTokens,
			window:    window,
			hasWindow: hasWindow,
		}
		if ts, ok := parseTime(entry.Timestamp); ok {
			result.observedAt = ts
			result.hasObservedAt = true
		}
		lastUsage = index
		haveUsage = true
	}
	return resolveCompaction(result, haveUsage, lastUsage, lastCompact)
}

// resolveCompaction applies the shared boundary rule: a usage record makes the
// figure available unless a compaction record supersedes it, and no usage at
// all is awaiting-first-response.
func resolveCompaction(result reading, haveUsage bool, lastUsage, lastCompact int) (reading, error) {
	if haveUsage {
		if lastCompact > lastUsage {
			return reading{}, errAfterCompaction
		}
		return result, nil
	}
	return reading{}, errAwaitingFirstResponse
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}
