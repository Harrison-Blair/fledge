// Package proposal owns the task proposal file: a TOML list of tasks with
// briefs and local dependencies, its strict validation, a creation order, and
// a printable skeleton.
package proposal

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/Harrison-Blair/fledge/internal/lib/brief"
	"github.com/Harrison-Blair/fledge/internal/lib/state"
	libtask "github.com/Harrison-Blair/fledge/internal/lib/task"
)

// schemaVersion is the only proposal file version this binary reads.
const schemaVersion = 1

// Parent is the optional parent task the proposed tasks belong under. Each
// pointer is nil when the file omits its key. The importer validates Branch
// and BaseBranch.
type Parent struct {
	Title        string  `toml:"title"`
	Brief        string  `toml:"brief"`
	Shape        *string `toml:"shape"`
	Branch       *string `toml:"branch"`
	BaseBranch   *string `toml:"base_branch"`
	DeliveryMode *string `toml:"delivery_mode"`
}

// Task is one proposed task. After holds local keys or existing task ids.
// DeliveryMode, BaseBranch, and Branch are nil when the file omits them. The
// importer validates BaseBranch and Branch.
type Task struct {
	Key          string   `toml:"key"`
	Title        string   `toml:"title"`
	Brief        string   `toml:"brief"`
	After        []string `toml:"after"`
	DeliveryMode *string  `toml:"delivery_mode"`
	BaseBranch   *string  `toml:"base_branch"`
	Branch       *string  `toml:"branch"`
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
	tableKeys = []string{"title", "brief", "shape", "branch", "base_branch", "delivery_mode"}
	taskKeys  = []string{"key", "title", "brief", "after", "delivery_mode", "base_branch", "branch"}
)

// Decode strictly parses and validates a proposal file, including that its
// local dependencies form no cycle.
func Decode(data []byte) (Proposal, error) {
	p, _, err := DecodeOrdered(data)
	return p, err
}

// DecodeOrdered is Decode that also returns the creation order it checked,
// as Order would. When the file's [parent] has shape stack, the tasks must
// form one after chain. When it has shape feature, it must have a branch,
// and FeatureMembers applies to the tasks.
func DecodeOrdered(data []byte) (Proposal, []Task, error) {
	// The typed decoder matches keys case-insensitively, so check exact
	// names on a raw decode first.
	var raw map[string]any
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return Proposal{}, nil, err
	}
	if err := checkKeys(raw); err != nil {
		return Proposal{}, nil, err
	}
	var f file
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&f); err != nil {
		return Proposal{}, nil, err
	}
	if f.SchemaVersion == nil {
		return Proposal{}, nil, fmt.Errorf("schema_version is required")
	}
	if *f.SchemaVersion != schemaVersion {
		return Proposal{}, nil, fmt.Errorf("unsupported schema_version %d; this Fledge reads %d", *f.SchemaVersion, schemaVersion)
	}
	if f.Parent != nil {
		if err := checkText(f.Parent.Title, f.Parent.Brief); err != nil {
			return Proposal{}, nil, fmt.Errorf("parent: %w", err)
		}
		if f.Parent.Shape != nil && !slices.Contains(libtask.Shapes, *f.Parent.Shape) {
			return Proposal{}, nil, fmt.Errorf("[parent]: shape must be one of %s", strings.Join(libtask.Shapes, ", "))
		}
		if f.Parent.Shape != nil && *f.Parent.Shape == libtask.ShapeFeature && f.Parent.Branch == nil {
			return Proposal{}, nil, fmt.Errorf("[parent]: shape feature needs a branch")
		}
		if f.Parent.DeliveryMode != nil && !slices.Contains(libtask.DeliveryModes, *f.Parent.DeliveryMode) {
			return Proposal{}, nil, fmt.Errorf("[parent]: delivery_mode must be one of %s", strings.Join(libtask.DeliveryModes, ", "))
		}
	}
	if len(f.Tasks) == 0 {
		return Proposal{}, nil, fmt.Errorf("a proposal needs at least one [[tasks]] entry")
	}
	keys := map[string]bool{}
	for i, t := range f.Tasks {
		switch {
		case t.Key == "":
			return Proposal{}, nil, fmt.Errorf("task #%d: key is required", i+1)
		case strings.ContainsAny(t.Key, "\r\n"):
			return Proposal{}, nil, fmt.Errorf("task #%d: key must be a single line", i+1)
		case state.ValidID(t.Key):
			return Proposal{}, nil, fmt.Errorf("task %q: key must not look like a task id", t.Key)
		case keys[t.Key]:
			return Proposal{}, nil, fmt.Errorf("task %q: duplicate key", t.Key)
		}
		keys[t.Key] = true
		if err := checkText(t.Title, t.Brief); err != nil {
			return Proposal{}, nil, fmt.Errorf("task %q: %w", t.Key, err)
		}
		if t.DeliveryMode != nil && !slices.Contains(libtask.DeliveryModes, *t.DeliveryMode) {
			return Proposal{}, nil, fmt.Errorf("task %q: delivery_mode must be one of %s", t.Key, strings.Join(libtask.DeliveryModes, ", "))
		}
	}
	for _, t := range f.Tasks {
		for _, dep := range t.After {
			if !keys[dep] && !state.ValidID(dep) {
				return Proposal{}, nil, fmt.Errorf("task %q: after names unknown key %q", t.Key, dep)
			}
		}
	}
	if f.Parent != nil && f.Parent.Shape != nil && *f.Parent.Shape == libtask.ShapeFeature {
		if err := FeatureMembers(f.Tasks, f.Parent.Branch); err != nil {
			return Proposal{}, nil, err
		}
	}
	p := Proposal{Parent: f.Parent, Tasks: f.Tasks}
	order, err := p.Order()
	if err != nil {
		return Proposal{}, nil, err
	}
	if p.Parent != nil && p.Parent.Shape != nil && *p.Parent.Shape == libtask.ShapeStack {
		if err := p.CheckStack("[parent]", nil); err != nil {
			return Proposal{}, nil, err
		}
	}
	return p, order, nil
}

// CheckStack requires the layers of parent among rs and the tasks, as new
// layers of parent, to form one after chain, as task.Layers checks it. A key
// never looks like an id, so each task takes its key as its id.
func (p Proposal) CheckStack(parent string, rs []libtask.Record) error {
	rs = slices.Clone(rs)
	for _, t := range p.Tasks {
		rs = append(rs, libtask.Record{ID: t.Key, Parent: &parent, After: t.After})
	}
	_, err := libtask.Layers(parent, rs)
	return err
}

// FeatureMembers gives each task without delivery_mode the mode local. When
// branch is not nil, it gives each task without base_branch the base branch.
// A task with another mode or base is an error that names its key.
func FeatureMembers(ts []Task, branch *string) error {
	name := ""
	if branch != nil {
		name = *branch
	}
	for i, t := range ts {
		if t.DeliveryMode != nil && *t.DeliveryMode != libtask.ModeLocal {
			return fmt.Errorf("task %q: a member of feature branch %q needs delivery_mode local, not %q", t.Key, name, *t.DeliveryMode)
		}
		if branch != nil && t.BaseBranch != nil && *t.BaseBranch != *branch {
			return fmt.Errorf("task %q: a member of feature branch %q needs base_branch %q, not %q", t.Key, name, name, *t.BaseBranch)
		}
		mode := libtask.ModeLocal
		ts[i].DeliveryMode = &mode
		if branch != nil {
			ts[i].BaseBranch = branch
		}
	}
	return nil
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

// checkText validates a title and its decoded brief.
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
// the advisory brief skeleton. It decodes as a valid proposal.
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
