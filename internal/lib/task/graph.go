package task

import "fmt"

// Progress counts a parent's direct children. Cancelled children are shown
// but not counted in Total.
type Progress struct {
	Verified  int `json:"verified"`
	Total     int `json:"total"`
	Cancelled int `json:"cancelled"`
}

// ChildProgress counts the direct children of task id among rs; it is nil
// when id has none.
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
		case Verified:
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
// dependents: it is verified, or cancelled.
func Satisfied(status string) bool { return status == Verified || status == Cancelled }

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
