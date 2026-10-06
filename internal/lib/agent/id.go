package agent

import "github.com/Harrison-Blair/fledge/internal/lib/state"

// ValidateID rejects an id flag value that is not a record id; kind names the
// record the flag refers to, such as agent or task.
func ValidateID(flag, kind, id string) error {
	if !state.ValidID(id) {
		return Invalid("--%s must be an 8 lowercase hexadecimal %s id", flag, kind)
	}
	return nil
}
