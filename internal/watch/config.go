package watch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/statedir"
)

const configFilename = "watch.json"

// Config is the watcher's project-local configuration.
type Config struct {
	// Version identifies the schema a file was written against so a future
	// rename can migrate rather than silently ignore the old spelling.
	Version                 int  `json:"version"`
	Enabled                 bool `json:"enabled"`
	PollIntervalSeconds     int  `json:"poll_interval_seconds"`
	IdlePollIntervalSeconds int  `json:"idle_poll_interval_seconds"`
	SignalGraceSeconds      int  `json:"signal_grace_seconds"`
	HeartbeatSeconds        int  `json:"heartbeat_seconds"`
	HeartbeatMaxSeconds     int  `json:"heartbeat_max_seconds"`
	WakeMinIntervalSeconds  int  `json:"wake_min_interval_seconds"`
	DoneMessageGraceSeconds int  `json:"done_message_grace_seconds"`
	EventStream             bool `json:"event_stream"`
	MinProtocol             int  `json:"min_protocol"`
}

// defaultConfig is the configuration the watcher runs with when a project has
// no .fledge/watch.json.
func defaultConfig() Config {
	return Config{
		Version:                 1,
		Enabled:                 true,
		PollIntervalSeconds:     15,
		IdlePollIntervalSeconds: 60,
		SignalGraceSeconds:      2,
		HeartbeatSeconds:        600,
		HeartbeatMaxSeconds:     7200,
		WakeMinIntervalSeconds:  30,
		DoneMessageGraceSeconds: 90,
		EventStream:             true,
		MinProtocol:             16,
	}
}

// LoadConfig reads the watcher configuration belonging to a project root. The
// file is advisory and read one field at a time: a missing, unreadable or
// corrupt file yields the defaults, and a single unusable field falls back to
// its own default without disturbing the settings around it. A typo can
// therefore never keep supervision from starting, nor quietly discard the
// "enabled": false beside it.
func LoadConfig(root string) Config {
	config := defaultConfig()

	contents, err := os.ReadFile(filepath.Join(statedir.Root(root), configFilename))
	if err != nil {
		return config
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(contents, &raw); err != nil {
		return config
	}

	// Decoding by hand would otherwise lose encoding/json's case-insensitive
	// field matching, and a hand-written "Enabled" silently ignored is exactly
	// the failure this loader exists to avoid.
	fields := make(map[string]json.RawMessage, len(raw))
	for key, value := range raw {
		fields[strings.ToLower(key)] = value
	}

	config.Version = intField(fields, "version", config.Version)
	config.Enabled = boolField(fields, "enabled", config.Enabled)
	config.PollIntervalSeconds = intField(fields, "poll_interval_seconds", config.PollIntervalSeconds)
	config.IdlePollIntervalSeconds = intField(fields, "idle_poll_interval_seconds", config.IdlePollIntervalSeconds)
	config.SignalGraceSeconds = intField(fields, "signal_grace_seconds", config.SignalGraceSeconds)
	config.HeartbeatSeconds = intField(fields, "heartbeat_seconds", config.HeartbeatSeconds)
	config.HeartbeatMaxSeconds = intField(fields, "heartbeat_max_seconds", config.HeartbeatMaxSeconds)
	config.WakeMinIntervalSeconds = intField(fields, "wake_min_interval_seconds", config.WakeMinIntervalSeconds)
	config.DoneMessageGraceSeconds = intField(fields, "done_message_grace_seconds", config.DoneMessageGraceSeconds)
	config.EventStream = boolField(fields, "event_stream", config.EventStream)
	config.MinProtocol = intField(fields, "min_protocol", config.MinProtocol)

	return config
}

func intField(fields map[string]json.RawMessage, key string, fallback int) int {
	var value int
	if !decodeField(fields, key, &value) {
		return fallback
	}
	return value
}

func boolField(fields map[string]json.RawMessage, key string, fallback bool) bool {
	var value bool
	if !decodeField(fields, key, &value) {
		return fallback
	}
	return value
}

// decodeField reports whether key held a value of the wanted type. An absent
// key and an explicit null are both treated as "unset" so they keep the
// default, while an explicit zero or false is a real setting.
func decodeField(fields map[string]json.RawMessage, key string, target any) bool {
	contents, ok := fields[key]
	if !ok || string(contents) == "null" {
		return false
	}

	return json.Unmarshal(contents, target) == nil
}
