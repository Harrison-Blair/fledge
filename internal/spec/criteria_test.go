package spec

import (
	"bytes"
	"testing"
)

const criteriaBody = `
# TASK-001: Example

## Description
Prose with - [ ] AC-9: decoy outside the section.

## Acceptance Criteria
Checkbox list, one criterion per line.
- [ ] AC-1: The tests listed above were observed failing before implementation and pass after.
- [x] AC-2: Satisfies REQ-001 FC-2.
- [X] AC-3: Uppercase checked variant.
  - [ ] AC-4: indented, not a criterion
- [ ] AC-5 missing colon, not a criterion
continuation prose under a criterion is legal and ignored.
- [ ] AC-6: last one

## Notes
- [ ] AC-7: after the section ends, ignored
`

func TestParseCriteria(t *testing.T) {
	cs := ParseCriteria([]byte(criteriaBody))
	want := []struct {
		n       int
		label   string
		checked bool
		text    string
	}{
		{1, "AC-1", false, "The tests listed above were observed failing before implementation and pass after."},
		{2, "AC-2", true, "Satisfies REQ-001 FC-2."},
		{3, "AC-3", true, "Uppercase checked variant."},
		{6, "AC-6", false, "last one"},
	}
	if len(cs) != len(want) {
		t.Fatalf("got %d criteria, want %d: %+v", len(cs), len(want), cs)
	}
	for i, w := range want {
		c := cs[i]
		if c.N != w.n || c.Label != w.label || c.Checked != w.checked || c.Text != w.text {
			t.Errorf("criterion %d = {N:%d Label:%q Checked:%v Text:%q}, want %+v", i, c.N, c.Label, c.Checked, c.Text, w)
		}
	}
}

func TestParseCriteriaNoSection(t *testing.T) {
	if cs := ParseCriteria([]byte("## Tests\n- [ ] AC-1: no AC section\n")); len(cs) != 0 {
		t.Fatalf("expected no criteria without an Acceptance Criteria section, got %+v", cs)
	}
}

func TestParseCriteriaCRLF(t *testing.T) {
	body := []byte("## Acceptance Criteria\r\n- [ ] AC-1: crlf line\r\n- [x] AC-2: checked\r\n")
	cs := ParseCriteria(body)
	if len(cs) != 2 {
		t.Fatalf("got %d criteria, want 2: %+v", len(cs), cs)
	}
	if cs[0].Checked || cs[0].Text != "crlf line" || !cs[1].Checked {
		t.Fatalf("bad CRLF parse: %+v", cs)
	}
}

// diffBytes returns the offsets at which a and b differ (or -1 length mismatch marker).
func diffBytes(t *testing.T, a, b []byte) []int {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("length changed: %d -> %d", len(a), len(b))
	}
	var offs []int
	for i := range a {
		if a[i] != b[i] {
			offs = append(offs, i)
		}
	}
	return offs
}

func TestSetCriterionCheckOneByte(t *testing.T) {
	body := []byte(criteriaBody)
	out, changed, err := SetCriterion(body, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	offs := diffBytes(t, body, out)
	if len(offs) != 1 {
		t.Fatalf("expected exactly one differing byte, got %d at %v", len(offs), offs)
	}
	if out[offs[0]] != 'x' {
		t.Fatalf("box byte = %q, want 'x'", out[offs[0]])
	}
	if body[offs[0]] != ' ' {
		t.Fatalf("original box byte = %q, want ' '", body[offs[0]])
	}
	cs := ParseCriteria(out)
	if !cs[0].Checked {
		t.Fatal("AC-1 not checked after SetCriterion")
	}
}

func TestSetCriterionUncheck(t *testing.T) {
	body := []byte(criteriaBody)
	out, changed, err := SetCriterion(body, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if offs := diffBytes(t, body, out); len(offs) != 1 || out[offs[0]] != ' ' {
		t.Fatalf("expected one byte flipped to space, got %v", offs)
	}
}

func TestSetCriterionIdempotent(t *testing.T) {
	body := []byte(criteriaBody)
	out, changed, err := SetCriterion(body, 2, true) // already checked
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected changed=false for already-checked box")
	}
	if !bytes.Equal(body, out) {
		t.Fatal("body bytes changed on no-op")
	}
	// Uppercase X stays untouched too.
	out, changed, err = SetCriterion(body, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed || !bytes.Equal(body, out) {
		t.Fatal("expected no-op for X-checked box")
	}
}

func TestSetCriterionUnknownN(t *testing.T) {
	if _, _, err := SetCriterion([]byte(criteriaBody), 9, true); err == nil {
		t.Fatal("expected error for unknown AC number")
	}
}

func TestSetCriterionNoCriteria(t *testing.T) {
	if _, _, err := SetCriterion([]byte("## Acceptance Criteria\nprose only\n"), 1, true); err == nil {
		t.Fatal("expected error when no parseable criteria exist")
	}
}

func TestSetCriterionCRLFPreserved(t *testing.T) {
	body := []byte("## Acceptance Criteria\r\n- [ ] AC-1: crlf line\r\n")
	out, changed, err := SetCriterion(body, 1, true)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if offs := diffBytes(t, body, out); len(offs) != 1 {
		t.Fatalf("expected one differing byte, got %v", offs)
	}
	if !bytes.HasSuffix(out, []byte("crlf line\r\n")) {
		t.Fatal("CRLF endings not preserved")
	}
}
