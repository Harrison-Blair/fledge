package agentcontext

import (
	"encoding/json"
	"path/filepath"
)

// collectCodex reads Codex's rollout transcript, located by its session-id
// file-name suffix under ~/.codex/sessions/<yyyy>/<mm>/<dd>/.
//
// Occupancy comes from the latest token_count event's last_token_usage: used =
// last_token_usage.input_tokens and window = model_context_window. The
// cumulative total_token_usage is not used — it grows with every turn's output
// and would misreport the live window.
//
// Codex marks compaction with a top-level "compacted" record and a
// context_compacted event. If a compaction record is the latest relevant event
// after the last token_count, the collector reports after_compaction until a
// fresh token_count appears.
func collectCodex(ref Ref, deps Deps) (reading, error) {
	contents, err := readTranscript(ref, deps, filepath.Join(deps.Home, ".codex", "sessions", "*", "*", "*", "*-"+ref.Value+".jsonl"))
	if err != nil {
		return reading{}, err
	}
	return scanCodex(contents)
}

type codexEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Payload   struct {
		Type string `json:"type"`
		Info *struct {
			LastTokenUsage *struct {
				InputTokens int `json:"input_tokens"`
			} `json:"last_token_usage"`
			ModelContextWindow *int `json:"model_context_window"`
		} `json:"info"`
	} `json:"payload"`
}

func scanCodex(contents []byte) (reading, error) {
	var (
		result      reading
		lastUsage   = -1
		lastCompact = -1
		haveUsage   bool
		sawToken    bool
	)
	for index, line := range splitLines(contents) {
		var entry codexEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type == "compacted" || entry.Payload.Type == "context_compacted" {
			lastCompact = index
			continue
		}
		if entry.Payload.Type != "token_count" {
			continue
		}
		sawToken = true
		info := entry.Payload.Info
		if info == nil || info.LastTokenUsage == nil || info.ModelContextWindow == nil {
			continue
		}
		result = reading{
			used:      info.LastTokenUsage.InputTokens,
			window:    *info.ModelContextWindow,
			hasWindow: *info.ModelContextWindow > 0,
		}
		if ts, ok := parseTime(entry.Timestamp); ok {
			result.observedAt = ts
			result.hasObservedAt = true
		}
		lastUsage = index
		haveUsage = true
	}
	if haveUsage {
		if lastCompact > lastUsage {
			return reading{}, errAfterCompaction
		}
		return result, nil
	}
	// token_count events present but none well-formed is a schema we do not
	// recognize; none at all is simply awaiting the first response.
	if sawToken {
		return reading{}, errUnsupportedFormat
	}
	return reading{}, errAwaitingFirstResponse
}
