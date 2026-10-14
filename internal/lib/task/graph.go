package task

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/lib/state"
)

// Progress counts a parent's direct children. Cancelled children are shown
// but not counted in Total.
type Progress struct {
	Verified  int `json:"verified"`
	Total     int `json:"total"`
	Cancelled int `json:"cancelled"`
}

// ChildProgress counts the direct children of task id among rs; it is nil
// when id has none. A landed child counts as verified.
func ChildProgress(id string, rs []Record) *Progress {
	var p *Progress
	for _, r := range rs {
		if r.Parent == nil || *r.Parent != id {
			continue
		}
		if p == nil {
			p = &Progress{}
		}
		switch r.Status {
		case Cancelled:
			p.Cancelled++
		case Verified, Landed:
			p.Verified++
			p.Total++
		default:
			p.Total++
		}
	}
	return p
}

// String is the progress as "2/3 verified", adding ", 1 cancelled" when any
// child was cancelled.
func (p Progress) String() string {
	s := fmt.Sprintf("%d/%d verified", p.Verified, p.Total)
	if p.Cancelled > 0 {
		s += fmt.Sprintf(", %d cancelled", p.Cancelled)
	}
	return s
}

// Satisfied reports whether a prerequisite in status no longer holds back its
// dependents: it is verified, landed, or cancelled.
func Satisfied(status string) bool {
	return status == Verified || status == Landed || status == Cancelled
}

// Index maps rs by id.
func Index(rs []Record) map[string]Record {
	byID := make(map[string]Record, len(rs))
	for _, r := range rs {
		byID[r.ID] = r
	}
	return byID
}

// Unmet returns r's prerequisites that are not satisfied, in declaration
// order. A prerequisite missing from byID is unmet.
func Unmet(r Record, byID map[string]Record) []string {
	unmet := []string{}
	for _, id := range r.After {
		if dep, ok := byID[id]; !ok || !Satisfied(dep.Status) {
			unmet = append(unmet, id)
		}
	}
	return unmet
}

// CheckLinks requires parent, when set, to accept subtasks and every task in
// after to exist. Call it inside the store lock that creates the linking task.
func CheckLinks(s *state.Store, parent string, after []string) error {
	if parent != "" {
		r, err := Get(s, parent)
		if err == nil {
			err = Require(&r, "adding a subtask", Created, Assigned, Completed)
		}
		if err != nil {
			return err
		}
	}
	for _, id := range after {
		if _, err := Get(s, id); err != nil {
			return err
		}
	}
	return nil
}

// Layers returns the direct subtasks of parent among rs, bottom first. Only
// after edges between these layers count, and cancelled layers stay in the
// chain. It fails, naming the fork or the gap, unless the edges form one
// chain. One chain has these properties:
//   - One layer, the bottom, has no prerequisite layer.
//   - Each other layer has one prerequisite layer.
//   - Each layer has at most one dependent layer.
//   - Every layer is in the chain.
func Layers(parent string, rs []Record) ([]Record, error) {
	byID := map[string]Record{}
	var ids []string
	for _, r := range rs {
		if r.Parent != nil && *r.Parent == parent {
			byID[r.ID] = r
			ids = append(ids, r.ID)
		}
	}
	var bottoms []string
	dependents := map[string][]string{}
	for _, id := range ids {
		var below []string
		for _, dep := range byID[id].After {
			if _, ok := byID[dep]; ok && !slices.Contains(below, dep) {
				below = append(below, dep)
			}
		}
		switch len(below) {
		case 0:
			bottoms = append(bottoms, id)
		case 1:
			dependents[below[0]] = append(dependents[below[0]], id)
		default:
			return nil, chainError("layer %s is after more than one layer: %s", id, strings.Join(below, ", "))
		}
	}
	for _, id := range ids {
		if len(dependents[id]) > 1 {
			return nil, chainError("layer %s has more than one dependent layer: %s", id, strings.Join(dependents[id], ", "))
		}
	}
	layers := []Record{}
	switch {
	case len(ids) == 0:
		return layers, nil
	case len(bottoms) == 0:
		return nil, chainError("no layer is without a prerequisite layer")
	case len(bottoms) > 1:
		return nil, chainError("more than one layer has no prerequisite layer: %s", strings.Join(bottoms, ", "))
	}
	for id := bottoms[0]; ; id = dependents[id][0] {
		layers = append(layers, byID[id])
		if len(dependents[id]) == 0 {
			break
		}
	}
	if len(layers) < len(ids) {
		var gap []string
		for _, id := range ids {
			if !slices.ContainsFunc(layers, func(r Record) bool { return r.ID == id }) {
				gap = append(gap, id)
			}
		}
		return nil, chainError("the chain from %s does not include %s", bottoms[0], strings.Join(gap, ", "))
	}
	return layers, nil
}

// chainError is a Layers failure.
func chainError(format string, args ...any) error {
	return fmt.Errorf("stack layers must form one after chain: "+format, args...)
}
