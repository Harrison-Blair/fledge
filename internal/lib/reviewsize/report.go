package reviewsize

import (
	"fmt"
	"io"
)

// Kind selects the limits a diff is compared against.
type Kind string

const (
	KindTask Kind = "task"
	KindPR   Kind = "pr"
)

// Review-unit limits from reference/plan/review-units.md. A task has one
// limit per measure; a pull request has a target and a ceiling.
const (
	TaskLines, TaskFiles, TaskDomains    = 200, 20, 3
	PRLines, PRFiles, PRDomains          = 200, 20, 3
	PRMaxLines, PRMaxFiles, PRMaxDomains = 300, 30, 5
	TotalReadLimit                       = 750
)

// Status is where one measure stands against its limits.
type Status string

const (
	Within      Status = "within"
	OverTarget  Status = "over target"
	OverCeiling Status = "over ceiling"
)

// Check is one measure against its limits. A measure with one limit has
// equal target and ceiling.
type Check struct {
	Value   int    `json:"value"`
	Target  int    `json:"target"`
	Ceiling int    `json:"ceiling"`
	Status  Status `json:"status"`
}

// Report is a diff's measures against the limits of one kind. It is advisory:
// an over-limit status refuses nothing.
type Report struct {
	Kind        Kind     `json:"kind"`
	SourceLines Check    `json:"source_lines"`
	Files       Check    `json:"files"`
	CodeDomains Check    `json:"code_domains"`
	TotalLines  Check    `json:"total_lines"`
	Domains     []string `json:"domains"`
	DocAreas    []string `json:"documentation_areas"`
}

// Compare checks d against the limits of kind.
func Compare(d Diff, kind Kind) Report {
	r := Report{Kind: kind, Domains: d.CodeDomains, DocAreas: d.DocAreas, TotalLines: check(d.TotalLines, TotalReadLimit, TotalReadLimit)}
	if kind == KindPR {
		r.SourceLines = check(d.SourceLines, PRLines, PRMaxLines)
		r.Files = check(d.Files, PRFiles, PRMaxFiles)
		r.CodeDomains = check(len(d.CodeDomains), PRDomains, PRMaxDomains)
	} else {
		r.SourceLines = check(d.SourceLines, TaskLines, TaskLines)
		r.Files = check(d.Files, TaskFiles, TaskFiles)
		r.CodeDomains = check(len(d.CodeDomains), TaskDomains, TaskDomains)
	}
	return r
}

func check(v, target, ceiling int) Check {
	c := Check{Value: v, Target: target, Ceiling: ceiling, Status: Within}
	switch {
	case v > ceiling:
		c.Status = OverCeiling
	case v > target:
		c.Status = OverTarget
	}
	return c
}

// Render writes r as one line per measure.
func Render(w io.Writer, r Report) {
	line := func(name string, c Check) {
		limit := fmt.Sprint(c.Target)
		if c.Ceiling != c.Target {
			limit = fmt.Sprintf("%d (ceiling %d)", c.Target, c.Ceiling)
		}
		fmt.Fprintf(w, "%-13s %5d / %-16s %s\n", name, c.Value, limit, c.Status)
	}
	line("source lines", r.SourceLines)
	line("files", r.Files)
	line("code domains", r.CodeDomains)
	line("total read", r.TotalLines)
}
