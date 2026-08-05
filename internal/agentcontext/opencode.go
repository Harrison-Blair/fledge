package agentcontext

import (
	"encoding/json"
	"time"
)

// collectOpenCode reads usage through the bounded export edge rather than the
// SQLite store. Deps.OpenCodeExport runs `opencode export --pure --sanitize
// <session-id>`, whose JSON is {info, messages:[...]}. The latest completed
// assistant message supplies the reading.
//
// used = tokens.input + tokens.cache.read + tokens.cache.write. tokens.output
// and tokens.reasoning are excluded. OpenCode's export does not carry a context
// window, so the window (and percent) is left null, and its export exposes no
// authoritative compaction boundary record, so after_compaction does not apply.
//
// The export command is the only OpenCode access path; a nil hook or a command
// failure is a telemetry failure, never a not-found.
func collectOpenCode(ref Ref, deps Deps) (reading, error) {
	if ref.Kind != "id" {
		return reading{}, errUnsupportedFormat
	}
	if deps.OpenCodeExport == nil {
		return reading{}, errUnsupportedFormat
	}
	contents, err := deps.OpenCodeExport(ref.Value)
	if err != nil {
		return reading{}, err
	}

	var export struct {
		Messages []struct {
			Info json.RawMessage `json:"info"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(contents, &export); err != nil {
		return reading{}, errUnsupportedFormat
	}
	if export.Messages == nil {
		return reading{}, errUnsupportedFormat
	}

	var (
		found  bool
		result reading
	)
	for _, exported := range export.Messages {
		if len(exported.Info) == 0 {
			continue
		}
		var message openCodeMessage
		if err := json.Unmarshal(exported.Info, &message); err != nil {
			continue
		}
		if message.Role != "assistant" || message.Finish == "" || message.Tokens == nil {
			continue
		}
		tokens := message.Tokens
		result = reading{used: tokens.Input + tokens.Cache.Read + tokens.Cache.Write}
		if message.Time != nil && message.Time.Completed > 0 {
			result.observedAt = time.UnixMilli(message.Time.Completed).UTC()
			result.hasObservedAt = true
		}
		found = true
	}
	if !found {
		return reading{}, errAwaitingFirstResponse
	}
	return result, nil
}

type openCodeMessage struct {
	Role   string `json:"role"`
	Finish string `json:"finish"`
	Tokens *struct {
		Input int `json:"input"`
		Cache struct {
			Read  int `json:"read"`
			Write int `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Time *struct {
		Completed int64 `json:"completed"`
	} `json:"time"`
}
