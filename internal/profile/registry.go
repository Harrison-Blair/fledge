package profile

import _ "embed"

// OrchestratorName is the reserved name of Fledge's root orchestrator profile.
const OrchestratorName = "fledge-orchestrator"

// Defaults are optional launch choices supplied by a profile. Explicit launch
// choices take precedence over these values.
type Defaults struct {
	Harness string
	Model   string
	Args    []string
}

// Profile is one immutable snapshot of Fledge-managed agent behavior.
type Profile struct {
	Name         string
	Description  string
	Instructions string
	Defaults     Defaults
}

//go:embed fledge-orchestrator.md
var orchestratorInstructions string

var managed = []Profile{{
	Name:         OrchestratorName,
	Description:  "Delegates project work through Fledge agents and independently verifies every material result.",
	Instructions: orchestratorInstructions,
}}

// List returns independent snapshots of every managed profile in presentation
// order.
func List() []Profile {
	profiles := make([]Profile, len(managed))
	for i, configured := range managed {
		profiles[i] = clone(configured)
	}
	return profiles
}

// Get returns an independent snapshot of the named managed profile.
func Get(name string) (Profile, bool) {
	for _, configured := range managed {
		if configured.Name == name {
			return clone(configured), true
		}
	}
	return Profile{}, false
}

func clone(configured Profile) Profile {
	configured.Defaults.Args = append([]string(nil), configured.Defaults.Args...)
	return configured
}
