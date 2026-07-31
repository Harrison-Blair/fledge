package herdr

import "testing"

func TestRequiredMethodsIncludePaneAndAgentInput(t *testing.T) {
	required := make(map[string]bool, len(RequiredMethods))
	for _, method := range RequiredMethods {
		required[method] = true
	}
	for _, method := range []string{"pane.send_input", "agent.send_keys"} {
		if !required[method] {
			t.Fatalf("%s is not required by the Herdr compatibility check", method)
		}
	}
}
