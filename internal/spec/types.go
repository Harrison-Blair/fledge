// Package spec parses, validates, and rewrites fledge REQ/TASK spec files:
// YAML frontmatter plus an opaque markdown body that is preserved byte-for-byte.
package spec

// Requirement statuses.
const (
	ReqDraft    = "draft"
	ReqApproved = "approved"
	ReqDone     = "done"
)

// Task statuses.
const (
	TaskBlocked    = "blocked"
	TaskReady      = "ready"
	TaskInProgress = "in-progress"
	TaskDone       = "done"
)

// Priorities and oversight values.
var (
	Priorities      = []string{"P0", "P1", "P2", "P3"}
	OversightValues = []string{"merge", "during"}
)

// Requirement is one spec/requirements/REQ-###-<kebab>.md file.
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

// Task is one spec/tasks/TASK-###-<kebab>.md file.
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
