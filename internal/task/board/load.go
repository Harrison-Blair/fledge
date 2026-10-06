package board

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/identity"
	"github.com/Harrison-Blair/fledge/internal/lib/task"
	"github.com/Harrison-Blair/fledge/internal/lib/termtext"
)

// Source identifies independently refreshed observations.
type Source int

const (
	Tasks Source = iota
	Workers
)
const requestBudget = 5 * time.Second

// Snapshot is an immutable task graph with precomputed outline text and content keys.
type Snapshot struct {
	Records  map[string]task.Record
	Nodes    map[string]node
	Roots    []string
	Children map[string][]string
	Active   map[string]bool
	Keys     map[string]string
}
type node struct {
	Title, ID, State, Progress string
	MissingParent              bool
}

// Worker contains only read-only, currently attributed worker display data.
type Worker struct{ Name, Activity string }

// Observation replaces only its named source, and only when Err is nil.
type Observation struct {
	Source   Source
	Snapshot *Snapshot
	Workers  map[string]Worker
	Err      error
}

// Load is the read-only storage and Herdr observation boundary. Call it off the
// UI event loop. Workers use a fresh terminal attribution join, never a cached pane.
func Load(ctx context.Context, c libagent.Client, source Source) Observation {
	ctx, cancel := context.WithTimeout(ctx, requestBudget)
	defer cancel()
	out := Observation{Source: source}
	if source == Tasks {
		s, err := task.Existing(ctx, c.Cwd)
		if err != nil {
			out.Err = err
			return out
		}
		records, err := task.List(s)
		if err == nil {
			out.Snapshot, err = project(records)
		}
		out.Err = err
		return out
	}
	agents, err := c.List(ctx)
	if err != nil {
		out.Err = err
		return out
	}
	s, err := identity.Existing(ctx, c.Cwd)
	if err != nil {
		out.Err = err
		return out
	}
	records := map[string]identity.Record{}
	if s != nil {
		records, err = identity.LiveByTerminal(s)
	}
	if err != nil {
		out.Err = err
		return out
	}
	out.Workers = map[string]Worker{}
	for _, a := range agents {
		if rec, ok := identity.Attributed(records, a); ok {
			name := rec.ID
			if a.Name != nil && *a.Name != "" {
				name = *a.Name
			}
			activity := a.AgentStatus
			if a.Agent != nil {
				activity += " (" + *a.Agent + ")"
			}
			out.Workers[rec.ID] = Worker{Name: singleLine(name), Activity: singleLine(activity)}
		}
	}
	return out
}

func project(records []task.Record) (*Snapshot, error) {
	seen := map[string]bool{}
	for _, r := range records {
		if task.ValidateID(r.ID) != nil || seen[r.ID] || !slices.Contains(task.Statuses, r.Status) {
			return nil, fmt.Errorf("malformed task identity or state: %s", termtext.Clean(r.ID))
		}
		seen[r.ID] = true
	}
	s := &Snapshot{Records: task.Index(records), Nodes: map[string]node{}, Children: map[string][]string{}, Active: map[string]bool{}, Keys: map[string]string{}}
	// Follow parent links with three-color marking before walking the outline.
	colors := map[string]uint8{}
	for _, r := range records {
		path := []string{}
		id := r.ID
		for id != "" && colors[id] == 0 {
			rec, ok := s.Records[id]
			if !ok {
				break
			}
			colors[id] = 1
			path = append(path, id)
			id = ""
			if rec.Parent != nil {
				id = *rec.Parent
			}
		}
		if colors[id] == 1 {
			return nil, fmt.Errorf("cyclic task hierarchy at %s", termtext.Clean(id))
		}
		for _, id := range path {
			colors[id] = 2
		}
	}
	for _, r := range records {
		n := node{Title: singleLine(r.Title), ID: singleLine(r.ID), State: singleLine(r.Status)}
		if r.Parent != nil {
			_, ok := s.Records[*r.Parent]
			n.MissingParent = !ok
			if ok {
				s.Children[*r.Parent] = append(s.Children[*r.Parent], r.ID)
			}
		}
		if r.Parent == nil || n.MissingParent {
			s.Roots = append(s.Roots, r.ID)
		}
		s.Nodes[r.ID] = n
	}
	for _, r := range records {
		n := s.Nodes[r.ID]
		if len(s.Children[r.ID]) > 0 {
			n.Progress = task.ChildProgress(r.ID, records).String()
		} else if r.Status == task.Created {
			n.State = "ready"
			if len(task.Unmet(r, s.Records)) > 0 {
				n.State = "waiting"
			}
		}
		if r.Status == task.Completed {
			n.State = "completed (awaiting verification)"
		}
		s.Nodes[r.ID] = n
		if !task.Satisfied(r.Status) {
			id := r.ID
			for id != "" && !s.Active[id] {
				s.Active[id] = true
				parent := s.Records[id].Parent
				id = ""
				if parent != nil {
					if _, ok := s.Records[*parent]; ok {
						id = *parent
					}
				}
			}
		}
		// Hash raw content off the event loop; no report wrapping or sanitizing here.
		h := sha256.New()
		enc := json.NewEncoder(h)
		_ = enc.Encode(r)
		for _, id := range r.After {
			dep := s.Records[id]
			_ = enc.Encode([]any{id, dep.Title, dep.Status, dep.CancelReason})
		}
		s.Keys[r.ID] = fmt.Sprintf("%x", h.Sum(nil))
	}
	return s, nil
}
