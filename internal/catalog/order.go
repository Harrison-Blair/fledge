package catalog

import "strings"

// rank classifies a segment position so digit runs sort ahead of a missing
// segment, which sorts ahead of letter runs.
const (
	rankLetters   = 0
	rankExhausted = 1
	rankDigits    = 2
)

// segments splits id on '-', '.', and '/' into runs of digits and runs of
// non-digits, dropping the separators themselves. A run ends whenever the
// separator class changes or a separator byte is seen.
func segments(id string) []string {
	var segs []string
	var run strings.Builder
	var runIsDigit bool
	flush := func() {
		if run.Len() > 0 {
			segs = append(segs, run.String())
			run.Reset()
		}
	}
	for i := 0; i < len(id); i++ {
		b := id[i]
		switch b {
		case '-', '.', '/':
			flush()
			continue
		}
		isDigit := b >= '0' && b <= '9'
		if run.Len() > 0 && isDigit != runIsDigit {
			flush()
		}
		runIsDigit = isDigit
		run.WriteByte(b)
	}
	flush()
	return segs
}

// rank reports the rank of segs at position i: a digit run, a letter run, or
// an exhausted (missing) segment.
func rank(segs []string, i int) int {
	if i >= len(segs) {
		return rankExhausted
	}
	if s := segs[i]; len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		return rankDigits
	}
	return rankLetters
}

// compareDigits compares two digit runs numerically, ascending, treating
// leading zeros as insignificant.
func compareDigits(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	if len(a) == 0 {
		return 0
	}
	return strings.Compare(a, b)
}

// compareIDs orders model IDs highest-version-first: a natural, descending
// comparison where digit runs compare numerically and other runs compare as
// text. IDs split on '-', '.', and '/' only. At a given position a digit run
// outranks a missing segment, which outranks a letter run. IDs that segment
// identically fall back to reverse byte order so the result is deterministic
// under the unstable slices.SortFunc; byte-wise comparison assumes the
// lowercase ASCII IDs the harnesses emit.
func compareIDs(a, b string) int {
	as, bs := segments(a), segments(b)
	for i := range max(len(as), len(bs)) {
		ra, rb := rank(as, i), rank(bs, i)
		if ra != rb {
			return rb - ra
		}
		switch ra {
		case rankExhausted:
			continue
		case rankDigits:
			if c := compareDigits(as[i], bs[i]); c != 0 {
				return -c
			}
		case rankLetters:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return -c
			}
		}
	}
	return strings.Compare(b, a)
}
