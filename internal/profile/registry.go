package profile

import _ "embed"

// OrchestratorName is the reserved name of Fledge's root orchestrator profile.
const OrchestratorName = "fledge-orchestrator"

// GeneralName is the name of Fledge's general managed-worker profile.
const GeneralName = "fledge-general"

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

// orchestratorRoleRules is the manager role section of the orchestrator
// profile; the full instructions are composed with the canonical fragments.
//
//go:embed fledge-orchestrator.md
var orchestratorRoleRules string

var managed = []Profile{{
	Name:         GeneralName,
	Description:  "Executes one dispatched work unit as a Fledge-managed worker and reports through the canonical callback protocol.",
	Instructions: managedWorker(),
}, {
	Name:         OrchestratorName,
	Description:  "Delegates project work through Fledge agents and independently verifies every material result.",
	Instructions: managedManager(orchestratorRoleRules),
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
