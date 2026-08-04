package watch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	defaults := Config{
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

	partial := defaults
	partial.PollIntervalSeconds = 5

	disabled := defaults
	disabled.Enabled = false

	streamless := defaults
	streamless.EventStream = false

	zeroed := defaults
	zeroed.PollIntervalSeconds = 0
	zeroed.DoneMessageGraceSeconds = 0
	zeroed.MinProtocol = 0

	full := Config{
		Version:                 2,
		Enabled:                 false,
		PollIntervalSeconds:     1,
		IdlePollIntervalSeconds: 2,
		SignalGraceSeconds:      3,
		HeartbeatSeconds:        4,
		HeartbeatMaxSeconds:     5,
		WakeMinIntervalSeconds:  6,
		DoneMessageGraceSeconds: 7,
		EventStream:             false,
		MinProtocol:             8,
	}

	tests := []struct {
		name     string
		contents string
		write    bool
		want     Config
	}{
		{name: "missing file", want: defaults},
		{name: "corrupt file", write: true, contents: "{not json", want: defaults},
		{name: "empty object", write: true, contents: "{}", want: defaults},
		{name: "wrong types", write: true, contents: `{"poll_interval_seconds":"fast"}`, want: defaults},
		{name: "partial file", write: true, contents: `{"poll_interval_seconds":5}`, want: partial},
		{name: "unknown fields ignored", write: true, contents: `{"poll_interval_seconds":5,"mystery":true}`, want: partial},
		{name: "enabled false honored", write: true, contents: `{"enabled":false}`, want: disabled},
		{name: "wrong type keeps its neighbours", write: true, contents: `{"enabled":false,"poll_interval_seconds":"fast"}`, want: disabled},
		{name: "explicit zeroes honored", write: true, contents: `{"poll_interval_seconds":0,"done_message_grace_seconds":0,"min_protocol":0}`, want: zeroed},
		{name: "null keeps the default", write: true, contents: `{"poll_interval_seconds":null,"enabled":null}`, want: defaults},
		{name: "keys are matched case insensitively", write: true, contents: `{"Enabled":false}`, want: disabled},
		{name: "json array", write: true, contents: `[1,2]`, want: defaults},
		{name: "event stream false honored", write: true, contents: `{"event_stream":false}`, want: streamless},
		{
			// A negative duration is not a shorter one: it inverts the windows
			// the engine derives from it, so it falls back like a wrong type.
			name:     "negative durations keep their defaults",
			write:    true,
			contents: `{"poll_interval_seconds":-1,"idle_poll_interval_seconds":-2,"signal_grace_seconds":-3,"heartbeat_seconds":-4,"heartbeat_max_seconds":-5,"wake_min_interval_seconds":-6,"done_message_grace_seconds":-7}`,
			want:     defaults,
		},
		{name: "a negative duration keeps its neighbours", write: true, contents: `{"enabled":false,"done_message_grace_seconds":-7}`, want: disabled},
		{
			name:     "every field set",
			write:    true,
			contents: `{"version":2,"enabled":false,"poll_interval_seconds":1,"idle_poll_interval_seconds":2,"signal_grace_seconds":3,"heartbeat_seconds":4,"heartbeat_max_seconds":5,"wake_min_interval_seconds":6,"done_message_grace_seconds":7,"event_stream":false,"min_protocol":8}`,
			want:     full,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			if test.write {
				stateDir := filepath.Join(root, ".fledge")
				if err := os.MkdirAll(stateDir, 0o700); err != nil {
					t.Fatalf("create state directory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(stateDir, "watch.json"), []byte(test.contents), 0o600); err != nil {
					t.Fatalf("write watch configuration: %v", err)
				}
			}

			if got := LoadConfig(root); got != test.want {
				t.Errorf("LoadConfig() = %+v, want %+v", got, test.want)
			}
		})
	}
}
