package spec

import (
	"os"
	"path/filepath"
	"strings"
)

// FileError attributes a parse failure to one spec file.
type FileError struct {
	Path string
	Err  error
}

// Set is every loaded spec file plus per-file parse errors and any unknown
// frontmatter keys (keyed by file path). Slices are in filename order.
type Set struct {
	Reqs          []*Requirement
	Tasks         []*Task
	Errors        []FileError
	UnknownFields map[string][]string
}

// Load reads all REQ and TASK files. Parse failures become Set.Errors, not a
// returned error; missing directories yield an empty set.
func Load(reqDir, taskDir string) (*Set, error) {
	set := &Set{UnknownFields: map[string][]string{}}
	for _, path := range specFiles(reqDir) {
		b, err := os.ReadFile(path)
		if err != nil {
			set.Errors = append(set.Errors, FileError{path, err})
			continue
		}
		r, unknown, err := ParseRequirementFile(path, b)
		if err != nil {
			set.Errors = append(set.Errors, FileError{path, err})
			continue
		}
		set.Reqs = append(set.Reqs, r)
		if len(unknown) > 0 {
			set.UnknownFields[path] = unknown
		}
	}
	for _, path := range specFiles(taskDir) {
		b, err := os.ReadFile(path)
		if err != nil {
			set.Errors = append(set.Errors, FileError{path, err})
			continue
		}
		t, unknown, err := ParseTaskFile(path, b)
		if err != nil {
			set.Errors = append(set.Errors, FileError{path, err})
			continue
		}
		set.Tasks = append(set.Tasks, t)
		if len(unknown) > 0 {
			set.UnknownFields[path] = unknown
		}
	}
	return set, nil
}

// specFiles lists .md files in dir (sorted by ReadDir); empty when dir is missing.
func specFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// Req returns the requirement with the given ID, or nil.
func (s *Set) Req(id string) *Requirement {
	for _, r := range s.Reqs {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// Task returns the task with the given ID, or nil.
func (s *Set) Task(id string) *Task {
	for _, t := range s.Tasks {
		if t.ID == id {
			return t
		}
	}
	return nil
}
