package reviewsize

import (
	"fmt"
	"io"
	"strings"
)

// Limits of reference/plan/review-units.md. A task has one limit per measure;
// a pull request has a target and a ceiling. Total read has one limit.
const (
	TaskLines        = 200
	TaskFiles        = 20
	TaskDomains      = 3
	PRTargetLines    = 200
	PRTargetFiles    = 20
	PRTargetDomains  = 3
	PRCeilingLines   = 300
	PRCeilingFiles   = 30
	PRCeilingDomains = 5
	TotalReadLimit   = 750
)

// Kind is what a diff is judged as.
type Kind string

const (
	KindTask Kind = "task"
	KindPR   Kind = "pr"
)

// Status is a measure's standing against its limits.
type Status string

const (
	Within      Status = "within"
	OverTarget  Status = "over target"
	OverCeiling Status = "over ceiling"
)

// Check is one measure against its limits. A single-limit check has Target
// equal to Ceiling.
type Check struct {
	Value   int    `json:"value"`
	Target  int    `json:"target"`
	Ceiling int    `json:"ceiling"`
	Status  Status `json:"status"`
}

// Report is a diff judged against the limits of its kind.
type Report struct {
	Kind        Kind     `json:"kind"`
	SourceLines Check    `json:"source_lines"`
	Files       Check    `json:"files"`
	CodeDomains Check    `json:"code_domains"`
	TotalLines  Check    `json:"total_lines"`
	Domains     []string `json:"domains"`
	DocAreas    []string `json:"doc_areas"`
}

// Compare judges d against the limits of kind. It is advisory: an over-limit
// diff only gets an over status.
func Compare(d Diff, kind Kind) Report {
	r := Report{
		Kind:       kind,
		TotalLines: check(d.TotalLines, TotalReadLimit, TotalReadLimit),
		Domains:    d.CodeDomains,
		DocAreas:   d.DocAreas,
	}
	if kind == KindPR {
		r.SourceLines = check(d.SourceLines, PRTargetLines, PRCeilingLines)
		r.Files = check(d.Files, PRTargetFiles, PRCeilingFiles)
		r.CodeDomains = check(len(d.CodeDomains), PRTargetDomains, PRCeilingDomains)
	} else {
		r.SourceLines = check(d.SourceLines, TaskLines, TaskLines)
		r.Files = check(d.Files, TaskFiles, TaskFiles)
		r.CodeDomains = check(len(d.CodeDomains), TaskDomains, TaskDomains)
	}
	return r
}

func check(value, target, ceiling int) Check {
	c := Check{Value: value, Target: target, Ceiling: ceiling, Status: Within}
	if value > ceiling {
		c.Status = OverCeiling
	} else if value > target {
		c.Status = OverTarget
	}
	return c
}

// Render writes r as one line per measure and the code domain list.
func Render(w io.Writer, r Report) {
	fmt.Fprintf(w, "%s size\n", r.Kind)
	for _, m := range []struct {
		name string
		c    Check
	}{{"source lines", r.SourceLines}, {"files", r.Files}, {"code domains", r.CodeDomains}, {"total read", r.TotalLines}} {
		limits := fmt.Sprintf("limit %d", m.c.Ceiling)
		if m.c.Target != m.c.Ceiling {
			limits = fmt.Sprintf("target %d, ceiling %d", m.c.Target, m.c.Ceiling)
		}
		fmt.Fprintf(w, "  %-12s %5d  %-24s %s\n", m.name, m.c.Value, limits, m.c.Status)
	}
	fmt.Fprintf(w, "  domains: %s\n", strings.Join(r.Domains, ", "))
}
