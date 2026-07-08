package bootstrap

// PrimitiveOrder is the canonical order of fledge's 6 orchestration primitives.
// Adapters declare subsets of these; the orchestration core is written to them.
var PrimitiveOrder = []string{
	"confirm-gate",
	"read-only-shell",
	"write-file",
	"run-fledge",
	"spawn-worker",
	"message-peer",
}

// primitiveDesc is the capability description (what the worker may attempt) for
// each primitive. Used to render adapter primitive maps and in consistency
// tests.
var primitiveDesc = map[string]string{
	"confirm-gate":   "present material, get a structured Accept/Make-changes or option choice",
	"read-only-shell": "run read-only shell commands",
	"write-file":     "write a file",
	"run-fledge":     "run any fledge CLI subcommand (incl. all spec mutation)",
	"spawn-worker":   "spawn a fresh, context-free, named, addressable sub-session returning one final message",
	"message-peer":   "send an async by-name message; sender may idle, woken on reply",
}

// primitiveTier is the tier each primitive is first required for.
var primitiveTier = map[string]string{
	"confirm-gate":    "A",
	"read-only-shell": "A",
	"write-file":      "A",
	"run-fledge":      "A",
	"spawn-worker":    "B",
	"message-peer":    "C",
}

// TierPrimitives is the full primitive set required to reach each tier.
var TierPrimitives = map[string][]string{
	"A": {"confirm-gate", "read-only-shell", "write-file", "run-fledge"},
	"B": {"confirm-gate", "read-only-shell", "write-file", "run-fledge", "spawn-worker"},
	"C": {"confirm-gate", "read-only-shell", "write-file", "run-fledge", "spawn-worker", "message-peer"},
}

// DeriveTier returns the highest tier whose required primitive set is fully
// provided, or "" if even Tier A is not satisfied. Tier is never declared — it
// falls out of which primitives an adapter provides (Q5).
func DeriveTier(provided map[string]bool) string {
	switch {
	case allProvided(TierPrimitives["C"], provided):
		return "C"
	case allProvided(TierPrimitives["B"], provided):
		return "B"
	case allProvided(TierPrimitives["A"], provided):
		return "A"
	default:
		return ""
	}
}

func allProvided(need []string, provided map[string]bool) bool {
	for _, p := range need {
		if !provided[p] {
			return false
		}
	}
	return true
}

// PrimitiveDesc returns the capability description for p.
func PrimitiveDesc(p string) string { return primitiveDesc[p] }

// PrimitiveTier returns the tier primitive p is first required for.
func PrimitiveTier(p string) string { return primitiveTier[p] }
