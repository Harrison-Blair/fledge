// Package spec parses, validates, and rewrites fledge PLM/FTHR spec files:
// YAML frontmatter plus an opaque markdown body that is preserved byte-for-byte.
package spec

// Plumage (requirement) statuses.
const (
	ReqEgg     = "egg"
	ReqHatched = "hatched"
	ReqFledged = "fledged"
)

// Feather (task) statuses.
const (
	TaskEgg      = "egg"
	TaskPipping  = "pipping"
	TaskHatching = "hatching"
	TaskFledged  = "fledged"
)

// Priorities and oversight values.
var (
	Priorities      = []string{"P0", "P1", "P2", "P3"}
	OversightValues = []string{"merge", "during"}
)

// Requirement is one pluma/plumage/PLM-###-<kebab>.md file.
type Requirement struct {
	ID            string
	Title         string
	Status        string
	Priority      string
	Authored      string
	Agent         string
	FledgeVersion string

	Path string // file path as loaded; "" for unsaved
	Body []byte // everything after the closing ---, byte-preserved
}

// Task is one pluma/feathers/FTHR-###-<kebab>.md file.
type Task struct {
	ID            string
	Title         string
	Requirement   string
	Status        string
	Priority      string
	DependsOn     []string
	Oversight     string // "" when absent
	Authored      string
	Agent         string
	FledgeVersion string

	Path string
	Body []byte
}
