package cli

import "testing"

// TestCommandOrderMatchesRegistrations pins FC-3: commandOrder and the
// registered commands map must always contain the same set of names. A
// command registered via init()/register but missing from commandOrder would
// silently vanish from usage output and the generated agent allow-list; a
// commandOrder entry with no registration would dangle. This test fails
// loudly, naming the offending command(s), whenever the two drift apart in
// either direction.
func TestCommandOrderMatchesRegistrations(t *testing.T) {
	inOrder := make(map[string]bool, len(commandOrder))
	for _, name := range commandOrder {
		inOrder[name] = true
	}

	for name := range commands {
		if !inOrder[name] {
			t.Errorf("command %q is registered but missing from commandOrder", name)
		}
	}

	for name := range inOrder {
		if _, ok := commands[name]; !ok {
			t.Errorf("commandOrder lists %q but no command is registered under that name", name)
		}
	}
}
