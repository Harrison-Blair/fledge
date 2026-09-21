package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

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

// worst returns the more severe of two statuses.
func worst(a, b Status) Status {
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

// newReport tallies the summary from the checks it wraps.
func newReport(checks []Check) Report {
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

// nameGap is the space between the widest check name and the status column.
const nameGap = 3

// configKeyWidth pads the configuration block's label column so its values
// align; it matches the labeled key layout of the grouped human report.
const configKeyWidth = 15

// maxLineWidth caps a rendered human line, including its indentation, so wrapped
// verbose lists stay within a standard 80-column terminal.
const maxLineWidth = 80

// detailIndent is the leading indent Write adds to every detail line.
const detailIndent = 4

// continuationIndent is the extra indent a wrapped continuation line carries on
// top of detailIndent, so continuations read as part of their parent item.
const continuationIndent = 4

// Write renders the report as grouped per-check blocks or a single JSON
// document. verbose reveals the long per-check detail in human output and has
// no effect on JSON.
func (r Report) Write(w io.Writer, asJSON, verbose bool) error {
	if asJSON {
		return json.NewEncoder(w).Encode(r)
	}
	home, _ := os.UserHomeDir()
	var b strings.Builder
	b.WriteString("fledge doctor\n\n")
	width := nameColumnWidth(r.Checks)
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "  %-*s%s\n", width, c.Name, c.Status)
		for _, line := range humanDetail(c, verbose) {
			fmt.Fprintf(&b, "%s%s\n", strings.Repeat(" ", detailIndent), shortenHome(line, home))
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "  %d ok · %d warn · %d fail\n", r.Summary.OK, r.Summary.Warn, r.Summary.Fail)
	_, err := io.WriteString(w, b.String())
	return err
}

// shortenHome replaces the user's home directory prefix with "~" in a human
// detail line. It is applied to human output only; JSON keeps absolute paths.
func shortenHome(s, home string) string {
	if home == "" || home == "/" {
		return s
	}
	s = strings.ReplaceAll(s, home+"/", "~/")
	if strings.HasSuffix(s, home) {
		s = s[:len(s)-len(home)] + "~"
	}
	return s
}

// nameColumnWidth aligns the status column against the widest check name.
func nameColumnWidth(checks []Check) int {
	longest := 0
	for _, c := range checks {
		if len(c.Name) > longest {
			longest = len(c.Name)
		}
	}
	return longest + nameGap
}

// humanDetail returns a check's indented detail lines. OK checks with structured
// data render a short summary (plus verbose extras); configuration always shows
// its labeled block; every other case shows the actionable diagnostic detail.
func humanDetail(c Check, verbose bool) []string {
	switch c.Name {
	case "herdr_compatibility":
		data, hasData := c.Data.(CompatibilityData)
		if c.Status == OK && hasData {
			return compatLines(data, verbose)
		}
		// Warn/fail: show the diagnostic, plus the capabilities when they are
		// known and verbose is requested, so the actionable context is complete.
		lines := wrapDetail(c.humanText())
		if verbose && hasData {
			lines = append(lines, wrapList("capabilities", capabilityTokens(data.Capabilities))...)
		}
		return lines
	case "harness_installations":
		if data, ok := c.Data.(HarnessData); ok && c.Status == OK {
			return harnessLines(data, verbose)
		}
	case "model_discovery":
		if data, ok := c.Data.(ModelData); ok {
			return modelLines(data)
		}
	case "configuration":
		if data, ok := c.Data.(ConfigData); ok {
			return configLines(data)
		}
	}
	return wrapDetail(c.humanText())
}

// wrapDetail splits a stored detail string on its own newlines, then word-wraps
// each line so no rendered line exceeds maxLineWidth once Write's indent is
// added. Whitespace-delimited tokens are never split, so an unbreakable single
// token (such as an absolute path) may still overflow.
func wrapDetail(detail string) []string {
	if detail == "" {
		return nil
	}
	var lines []string
	for _, part := range strings.Split(detail, "\n") {
		lines = append(lines, wrapText(part)...)
	}
	return lines
}

// wrapText word-wraps a single plain line at spaces. The first line carries no
// indent of its own (Write adds detailIndent); continuation lines carry
// continuationIndent leading spaces. Tokens are never split.
func wrapText(s string) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	budget := maxLineWidth - detailIndent
	cont := strings.Repeat(" ", continuationIndent)
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		candidate := line + " " + word
		if len(candidate) <= budget {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = cont + word
	}
	return append(lines, line)
}

// compatLines shows the version and protocol; the capability list is verbose-only.
func compatLines(d CompatibilityData, verbose bool) []string {
	lines := []string{fmt.Sprintf("version %s · protocol %d (expected %d)", dash(d.Version), d.Protocol, d.Expected)}
	if verbose {
		lines = append(lines, wrapList("capabilities", capabilityTokens(d.Capabilities))...)
	}
	return lines
}

// harnessLines shows the target and availability counts; the available names are
// verbose-only.
func harnessLines(d HarnessData, verbose bool) []string {
	var available []string
	for _, in := range d.Integrations {
		if in.Available {
			available = append(available, in.Target)
		}
	}
	lines := []string{fmt.Sprintf("%d targets, %d available", len(d.Integrations), len(available))}
	if verbose && len(available) > 0 {
		lines = append(lines, wrapList("available", available)...)
	}
	return lines
}

// wrapList renders "label: a, b, c" wrapping at the ", " separators so no
// rendered line (once Write adds detailIndent, and continuationIndent on
// continuations) exceeds maxLineWidth. Tokens are never split; continuation
// lines carry continuationIndent leading spaces. An empty list renders as
// "label: none".
func wrapList(label string, items []string) []string {
	head := label + ":"
	if len(items) == 0 {
		return []string{head + " none"}
	}
	// budget bounds a returned line's own length; Write adds detailIndent, so
	// keeping the length within maxLineWidth-detailIndent keeps the printed line
	// within maxLineWidth. Continuation lines include their indent in that length.
	budget := maxLineWidth - detailIndent
	cont := strings.Repeat(" ", continuationIndent)

	var lines []string
	line := head
	for i, item := range items {
		unit := item
		if i < len(items)-1 {
			unit += ","
		}
		candidate := line + " " + unit
		switch {
		case line == head:
			// The first unit always joins the label line, even if it overflows,
			// because a token is never split onto its own wrapped fragment.
			line = candidate
		case len(candidate) <= budget:
			line = candidate
		default:
			lines = append(lines, line)
			line = cont + unit
		}
	}
	return append(lines, line)
}

// modelLines shows the short per-harness counts, then the diagnostic detail of
// any harness that is not ok so warnings and failures stay actionable.
func modelLines(d ModelData) []string {
	var counts []string
	for _, h := range d.Harnesses {
		counts = append(counts, fmt.Sprintf("%s %d", h.Harness, h.Count))
	}
	lines := []string{strings.Join(counts, " · ")}
	for _, h := range d.Harnesses {
		if h.Status != OK {
			// Word-wrap the per-harness diagnostic; an absolute path inside a
			// discovery error is a single token and may still overflow.
			lines = append(lines, wrapText(fmt.Sprintf("%s: %s", h.Harness, h.Detail))...)
		}
	}
	return lines
}

// configLines renders the always-present labeled configuration block with an
// aligned key column.
func configLines(d ConfigData) []string {
	rows := []struct{ key, val string }{
		{"HERDR_ENV", d.HerdrEnv},
		{"socket", d.SocketField},
		{"pane", d.PaneID},
		{"session", d.Session},
		{"cwd", d.Cwd},
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = fmt.Sprintf("%-*s%s", configKeyWidth, row.key, dash(row.val))
	}
	return lines
}

// ReportError carries a failing report's exit status without re-printing it; the
// report was already written to the command's output before it is returned.
type ReportError struct{ Report Report }

func (e *ReportError) Error() string { return fmt.Sprintf("%d checks failed", e.Report.Summary.Fail) }
func (e *ReportError) ExitCode() int { return 1 }

// OutputError reports a failure to render the report, without a second write.
type OutputError struct{ Cause error }

func (e *OutputError) Error() string { return e.Cause.Error() }
func (e *OutputError) Unwrap() error { return e.Cause }
func (e *OutputError) ExitCode() int { return 1 }
