package lifecycle

import "time"

const DefaultAgentTimeout = 30 * time.Second

// StartOptions controls creation of a fresh project's orchestrator. Selection
// options are rejected when the project already has a Herdr session.
type StartOptions struct {
	Harness    string
	Model      string
	Timeout    time.Duration
	NativeArgs []string
	HarnessSet bool
	ModelSet   bool
	TimeoutSet bool
}

// SpawnOptions controls creation of one ad-hoc agent tab.
type SpawnOptions struct {
	Name       string
	Harness    string
	Model      string
	Cwd        string
	Timeout    time.Duration
	Prompt     string
	NativeArgs []string
	ModelSet   bool
}

func (o StartOptions) HasSelection() bool {
	return o.HarnessSet || o.ModelSet || o.TimeoutSet || len(o.NativeArgs) > 0
}
