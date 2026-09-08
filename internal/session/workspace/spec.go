package workspace

import "fmt"

// Role is a stable logical key persisted for a managed workspace.
type Role string

const (
	// Orchestrator is the pre-existing workspace that hosts the orchestrator.
	Orchestrator Role = "orchestrator"
	// Agents is the workspace in which managed worker agents are placed.
	Agents Role = "agents"
)

// Spec is the immutable policy registered for one managed-workspace role.
type Spec struct {
	role        Role
	labelPrefix string
	creatable   bool
}

// specs is the sole source of managed-workspace label policy. Lifecycle and
// bootstrap wiring must consume Lookup and Label rather than duplicate these
// prefixes at their call sites.
var specs = []Spec{
	{role: Orchestrator, labelPrefix: "f:", creatable: false},
	{role: Agents, labelPrefix: "f-agents:", creatable: true},
}

// UnregisteredRoleError reports a role absent from the managed-workspace table.
type UnregisteredRoleError struct {
	Role Role
}

func (e *UnregisteredRoleError) Error() string {
	return fmt.Sprintf("unregistered managed workspace role %q", e.Role)
}

// Roles returns all registered roles in deterministic lifecycle order.
func Roles() []Role {
	roles := make([]Role, len(specs))
	for i, spec := range specs {
		roles[i] = spec.role
	}
	return roles
}

// Lookup returns the immutable policy for role.
func Lookup(role Role) (Spec, error) {
	for _, spec := range specs {
		if spec.role == role {
			return spec, nil
		}
	}
	return Spec{}, &UnregisteredRoleError{Role: role}
}

// Role returns the spec's stable persisted key.
func (s Spec) Role() Role { return s.role }

// Creatable reports whether the workflow may create a missing workspace.
func (s Spec) Creatable() bool { return s.creatable }

// Label computes the role's exact display label from a project basename.
func (s Spec) Label(projectBase string) (string, error) {
	if s.role == "" {
		return "", fmt.Errorf("managed workspace spec is empty")
	}
	if projectBase == "" {
		return "", fmt.Errorf("managed workspace label: project basename is empty")
	}
	return s.labelPrefix + projectBase, nil
}
