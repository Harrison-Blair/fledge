package workspace

import (
	"errors"
	"reflect"
	"testing"
)

func TestRegisteredSpecs(t *testing.T) {
	if got, want := Roles(), []Role{Orchestrator, Agents}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Roles() = %#v, want %#v", got, want)
	}
	// Callers cannot mutate the package-level ordering through the returned slice.
	roles := Roles()
	roles[0] = Agents
	if got := Roles()[0]; got != Orchestrator {
		t.Fatalf("Roles()[0] after caller mutation = %q, want %q", got, Orchestrator)
	}

	tests := []struct {
		role      Role
		label     string
		creatable bool
	}{
		// The orchestrator f: prefix is the bootstrap integration contract.
		// Dependent wiring must consume this spec rather than retain an older
		// duplicated literal.
		{role: Orchestrator, label: "f:project", creatable: false},
		{role: Agents, label: "f-agents:project", creatable: true},
	}
	for _, test := range tests {
		t.Run(string(test.role), func(t *testing.T) {
			spec, err := Lookup(test.role)
			if err != nil {
				t.Fatal(err)
			}
			label, err := spec.Label("project")
			if err != nil {
				t.Fatal(err)
			}
			if spec.Role() != test.role || spec.Creatable() != test.creatable || label != test.label {
				t.Fatalf("spec = role %q, creatable %v, label %q", spec.Role(), spec.Creatable(), label)
			}
		})
	}
}

func TestLookupRejectsUnregisteredRole(t *testing.T) {
	_, err := Lookup(Role("future"))
	var unregistered *UnregisteredRoleError
	if !errors.As(err, &unregistered) || unregistered.Role != "future" {
		t.Fatalf("Lookup() error = %#v, want UnregisteredRoleError for future", err)
	}
}

func TestSpecLabelRejectsEmptyBasenameAndZeroSpec(t *testing.T) {
	spec, err := Lookup(Agents)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Label(""); err == nil {
		t.Fatal("Label(\"\") succeeded")
	}
	if _, err := (Spec{}).Label("project"); err == nil {
		t.Fatal("zero Spec Label() succeeded")
	}
}
