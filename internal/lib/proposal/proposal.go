// Package proposal owns the task proposal file: a TOML list of tasks with
// template briefs and local dependencies, its strict validation, a creation
// order, and a printable skeleton.
package proposal

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// schemaVersion is the only proposal file version this binary reads.
const schemaVersion = 1

// Parent is the optional parent task the proposed tasks belong under.
type Parent struct {
	Title string `toml:"title"`
	Brief string `toml:"brief"`
}

// Task is one proposed task. After holds local keys or existing task ids.
type Task struct {
	Key   string   `toml:"key"`
	Title string   `toml:"title"`
	Brief string   `toml:"brief"`
	After []string `toml:"after"`
}

// Proposal is a decoded, validated proposal file.
type Proposal struct {
	Parent *Parent
	Tasks  []Task
}

// file is the TOML shape; a pointer distinguishes an omitted version.
type file struct {
	SchemaVersion *int64  `toml:"schema_version"`
	Parent        *Parent `toml:"parent"`
	Tasks         []Task  `toml:"tasks"`
}

var (
	topKeys   = []string{"schema_version", "parent", "tasks"}
	tableKeys = []string{"title", "brief"}
	taskKeys  = []string{"key", "title", "brief", "after"}
)

// Decode strictly parses and validates a proposal file, including that its
// local dependencies form no cycle.
func Decode(data []byte) (Proposal, error) {
	// The typed decoder matches keys case-insensitively, so check exact
	// names on a raw decode first.
	var raw map[string]any
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return Proposal{}, err
	}
	if err := checkKeys(raw); err != nil {
		return Proposal{}, err
	}
	var f file
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return Proposal{}, err
	}
	if f.SchemaVersion == nil {
		return Proposal{}, fmt.Errorf("schema_version is required")
	}
	if *f.SchemaVersion != schemaVersion {
		return Proposal{}, fmt.Errorf("unsupported schema_version %d; this Fledge reads %d", *f.SchemaVersion, schemaVersion)
	}
	if f.Parent != nil {
		if err := checkText(f.Parent.Title, f.Parent.Brief); err != nil {
			return Proposal{}, fmt.Errorf("parent: %w", err)
		}
	}
	if len(f.Tasks) == 0 {
		return Proposal{}, fmt.Errorf("a proposal needs at least one [[tasks]] entry")
	}
	keys := map[string]bool{}
	for i, t := range f.Tasks {
		switch {
		case t.Key == "":
			return Proposal{}, fmt.Errorf("task #%d: key is required", i+1)
		case strings.ContainsAny(t.Key, "\r\n"):
			return Proposal{}, fmt.Errorf("task #%d: key must be a single line", i+1)
		case state.ValidID(t.Key):
			return Proposal{}, fmt.Errorf("task %q: key must not look like a task id", t.Key)
		case keys[t.Key]:
			return Proposal{}, fmt.Errorf("task %q: duplicate key", t.Key)
		}
		keys[t.Key] = true
		if err := checkText(t.Title, t.Brief); err != nil {
			return Proposal{}, fmt.Errorf("task %q: %w", t.Key, err)
		}
	}
	for _, t := range f.Tasks {
		for _, dep := range t.After {
			if !keys[dep] && !state.ValidID(dep) {
				return Proposal{}, fmt.Errorf("task %q: after names unknown key %q", t.Key, dep)
			}
		}
	}
	p := Proposal{Parent: f.Parent, Tasks: f.Tasks}
	if _, err := p.Order(); err != nil {
		return Proposal{}, err
	}
	return p, nil
}

// checkKeys rejects any key outside the documented shape.
func checkKeys(raw map[string]any) error {
	for k := range raw {
		if !slices.Contains(topKeys, k) {
			return fmt.Errorf("unknown key %q", k)
		}
	}
	if parent, ok := raw["parent"].(map[string]any); ok {
		for k := range parent {
			if !slices.Contains(tableKeys, k) {
				return fmt.Errorf("unknown key %q in [parent]", k)
			}
		}
	}
	// [[tasks]] decodes as []map[string]any, an inline array as []any.
	var tasks []map[string]any
	switch v := raw["tasks"].(type) {
	case []map[string]any:
		tasks = v
	case []any:
		for _, t := range v {
			if m, ok := t.(map[string]any); ok {
				tasks = append(tasks, m)
			}
		}
	}
	for i, t := range tasks {
		for k := range t {
			if !slices.Contains(taskKeys, k) {
				if key, ok := t["key"].(string); ok {
					return fmt.Errorf("task %q: unknown key %q", key, k)
				}
				return fmt.Errorf("task #%d: unknown key %q", i+1, k)
			}
		}
	}
	return nil
}

// checkText validates a title and its template brief.
func checkText(title, text string) error {
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if strings.ContainsAny(title, "\r\n") {
		return fmt.Errorf("title must be a single line")
	}
	return brief.Validate(text)
}

// Order returns the tasks so every local prerequisite precedes its dependent,
// keeping file order where dependencies allow. It fails on a cycle among
// local keys, naming the chain.
func (p Proposal) Order() ([]Task, error) {
	index := map[string]int{}
	for i, t := range p.Tasks {
		index[t.Key] = i
	}
	const (
		visiting = 1
		done     = 2
	)
	mark := make([]int, len(p.Tasks))
	var order []Task
	var stack []string
	var visit func(i int) error
	visit = func(i int) error {
		switch mark[i] {
		case done:
			return nil
		case visiting:
			start := slices.Index(stack, p.Tasks[i].Key)
			chain := append(slices.Clone(stack[start:]), p.Tasks[i].Key)
			return fmt.Errorf("dependency cycle: %s", strings.Join(chain, " → "))
		}
		mark[i] = visiting
		stack = append(stack, p.Tasks[i].Key)
		for _, dep := range p.Tasks[i].After {
			if j, ok := index[dep]; ok {
				if err := visit(j); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		mark[i] = done
		order = append(order, p.Tasks[i])
		return nil
	}
	for i := range p.Tasks {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// Skeleton is an unfilled proposal: a parent and one task whose briefs are
// the brief skeleton. It decodes structurally but fails brief validation.
func Skeleton() string {
	return fmt.Sprintf(`schema_version = 1

# Optional: the parent task the proposed tasks are created under.
[parent]
title = "Parent task title"
brief = '''
%s'''

# One [[tasks]] entry per task. key names the task within this file;
# after lists local keys or existing 8-hex task ids it waits on.
[[tasks]]
key = "first"
title = "First task title"
after = []
brief = '''
%s'''
`, brief.Skeleton(), brief.Skeleton())
}
