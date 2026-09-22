package report

import "fmt"

// Status is a single check's verdict.
type Status string

const (
	OK   Status = "ok"
	Warn Status = "warn"
	Fail Status = "fail"
)

// severity orders statuses so a report can roll up to its worst member.
func severity(s Status) int {
	switch s {
	case Fail:
		return 2
	case Warn:
		return 1
	default:
		return 0
	}
}

// Worst returns the more severe of two statuses.
func Worst(a, b Status) Status {
	if severity(b) > severity(a) {
		return b
	}
	return a
}

// Check is one diagnosis with a stable name, a status, a human detail line, and
// optional structured data specific to the check.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Data   any    `json:"data,omitempty"`
	// Human overrides Detail in the grouped human report when set. It is never
	// serialized, so the JSON detail is unaffected. Empty means "use Detail".
	Human string `json:"-"`
}

// humanText is the phrasing the grouped human report shows for a check: its
// Human override when set, otherwise the stored (JSON) Detail.
func (c Check) humanText() string {
	if c.Human != "" {
		return c.Human
	}
	return c.Detail
}

// Summary counts checks by status.
type Summary struct {
	OK   int `json:"ok"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// Report is the full diagnosis: every check plus a summary.
type Report struct {
	Checks  []Check `json:"checks"`
	Summary Summary `json:"summary"`
}

// New tallies the summary from the checks it wraps.
func New(checks []Check) Report {
	var s Summary
	for _, c := range checks {
		switch c.Status {
		case Fail:
			s.Fail++
		case Warn:
			s.Warn++
		default:
			s.OK++
		}
	}
	return Report{Checks: checks, Summary: s}
}

// ReportError carries a failing report's exit status without re-printing it; the
// report was already written to the command's output before it is returned.
type ReportError struct{ Report Report }

func (e *ReportError) Error() string { return fmt.Sprintf("%d checks failed", e.Report.Summary.Fail) }
func (e *ReportError) ExitCode() int { return 1 }
func (e *ReportError) Rendered()     {}
