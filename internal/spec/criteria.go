package spec

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// Criterion is one acceptance-criteria checkbox line in a spec body.
type Criterion struct {
	N       int
	Label   string // "AC-<N>"
	Checked bool
	Text    string

	boxOff int // byte offset in the body of the state char inside [ ]
}

const criteriaHeading = "## Acceptance Criteria"

// acLineRe matches a checkbox criterion line (trailing \r already stripped).
// No leading indentation is allowed; write form is always lowercase x.
var acLineRe = regexp.MustCompile(`^- \[([ xX])\] (AC-(\d+)):[ \t]?(.*)$`)

// ParseCriteria returns the checkbox criteria inside the Acceptance Criteria
// section, in file order. Non-matching lines in the section are ignored.
func ParseCriteria(body []byte) []Criterion {
	var out []Criterion
	in := false
	off := 0
	for off < len(body) {
		nl := bytes.IndexByte(body[off:], '\n')
		var line []byte
		var next int
		if nl == -1 {
			line = body[off:]
			next = len(body)
		} else {
			line = body[off : off+nl]
			next = off + nl + 1
		}
		trimmed := strings.TrimRight(strings.TrimSuffix(string(line), "\r"), " ")
		if strings.HasPrefix(trimmed, "#") {
			in = trimmed == criteriaHeading
		} else if in {
			if m := acLineRe.FindStringSubmatch(strings.TrimSuffix(string(line), "\r")); m != nil {
				n := 0
				fmt.Sscanf(m[3], "%d", &n)
				out = append(out, Criterion{
					N:       n,
					Label:   m[2],
					Checked: m[1] != " ",
					Text:    m[4],
					boxOff:  off + 3, // "- [" is 3 bytes
				})
			}
		}
		off = next
	}
	return out
}

// SetCriterion returns a copy of body with criterion AC-n's box set to
// checked. Exactly one byte differs when a change is made; the body is
// otherwise byte-preserved. changed is false when the box already has the
// requested state.
func SetCriterion(body []byte, n int, checked bool) (newBody []byte, changed bool, err error) {
	cs := ParseCriteria(body)
	if len(cs) == 0 {
		return nil, false, fmt.Errorf("no acceptance-criteria checkboxes found")
	}
	for _, c := range cs {
		if c.N != n {
			continue
		}
		if c.Checked == checked {
			return body, false, nil
		}
		out := append([]byte(nil), body...)
		if checked {
			out[c.boxOff] = 'x'
		} else {
			out[c.boxOff] = ' '
		}
		return out, true, nil
	}
	return nil, false, fmt.Errorf("AC-%d not found", n)
}
