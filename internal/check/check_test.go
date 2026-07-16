package check

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func req(id, status string) *spec.Requirement {
	return &spec.Requirement{
		ID: id, Title: "t", Status: status, Priority: "P1",
		Authored: "2026-07-06T12:00:00Z", Agent: "a", FledgeVersion: "0.1.0",
		Path: ".fledge/pluma/plumage/" + id + "-t.md",
		Body: []byte("## Context\nx\n## Functional Criteria\nx\n## Acceptance Criteria\nx\n"),
	}
}

func task(id, reqID, status string, deps ...string) *spec.Task {
	if deps == nil {
		deps = []string{}
	}
	return &spec.Task{
		ID: id, Title: "t", Requirement: reqID, Status: status, Priority: "P1",
		DependsOn: deps, Authored: "2026-07-06T12:00:00Z", Agent: "a", FledgeVersion: "0.1.0",
		Path: ".fledge/pluma/feathers/" + id + "-t.md",
		Body: []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\nx\n"),
	}
}

func newSet(reqs []*spec.Requirement, tasks []*spec.Task) *spec.Set {
	return &spec.Set{Reqs: reqs, Tasks: tasks, UnknownFields: map[string][]string{}}
}

func hasRule(fs []Finding, rule string, sev Severity) bool {
	for _, f := range fs {
		if f.Rule == rule && f.Severity == sev {
			return true
		}
	}
	return false
}

func TestCleanSetHasNoFindings(t *testing.T) {
	s := newSet(
		[]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{task("FTHR-001", "PLM-001", "pipping")},
	)
	if fs := Run(s, nil, ""); len(fs) != 0 {
		t.Errorf("clean set produced findings: %v", fs)
	}
}

func TestParseErrorsSurface(t *testing.T) {
	s := newSet(nil, nil)
	s.Errors = []spec.FileError{{Path: ".fledge/pluma/feathers/FTHR-009-x.md", Err: errors.New("boom")}}
	if !hasRule(Run(s, nil, ""), "parse", Error) {
		t.Error("want parse error finding")
	}
}

func TestUnknownFieldWarning(t *testing.T) {
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, nil)
	s.UnknownFields[".fledge/pluma/plumage/PLM-001-t.md"] = []string{"extra"}
	if !hasRule(Run(s, nil, ""), "unknown-field", Warning) {
		t.Error("want unknown-field warning")
	}
}

func TestSchemaRules(t *testing.T) {
	badStatus := task("FTHR-001", "PLM-001", "wip")
	missingTitle := task("FTHR-002", "PLM-001", "pipping")
	missingTitle.Title = ""
	badPriority := task("FTHR-003", "PLM-001", "pipping")
	badPriority.Priority = "P9"
	badOversight := task("FTHR-004", "PLM-001", "pipping")
	badOversight.Oversight = "always"
	badAuthored := task("FTHR-005", "PLM-001", "pipping")
	badAuthored.Authored = "yesterday"
	badReqStatus := req("PLM-002", "pending")

	s := newSet(
		[]*spec.Requirement{req("PLM-001", "hatched"), badReqStatus},
		[]*spec.Task{badStatus, missingTitle, badPriority, badOversight, badAuthored},
	)
	fs := Run(s, nil, "")
	countSchema := 0
	for _, f := range fs {
		if f.Rule == "schema" && f.Severity == Error {
			countSchema++
		}
	}
	if countSchema < 6 {
		t.Errorf("want >=6 schema errors, got %d: %v", countSchema, fs)
	}
}

func TestIDFilenameAgreement(t *testing.T) {
	tk := task("FTHR-001", "PLM-001", "pipping")
	tk.Path = ".fledge/pluma/feathers/FTHR-002-t.md" // filename says 002
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{tk})
	if !hasRule(Run(s, nil, ""), "id-filename", Error) {
		t.Error("want id-filename error")
	}
}

func TestDuplicateIDs(t *testing.T) {
	t1 := task("FTHR-001", "PLM-001", "pipping")
	t2 := task("FTHR-001", "PLM-001", "pipping")
	t2.Path = ".fledge/pluma/feathers/FTHR-001-other.md"
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{t1, t2})
	if !hasRule(Run(s, nil, ""), "duplicate-id", Error) {
		t.Error("want duplicate-id error")
	}
}

func TestDanglingRefs(t *testing.T) {
	missingReq := task("FTHR-001", "PLM-099", "pipping")
	missingDep := task("FTHR-002", "PLM-001", "egg", "FTHR-098")
	selfRef := task("FTHR-003", "PLM-001", "egg", "FTHR-003")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{missingReq, missingDep, selfRef})
	fs := Run(s, nil, "")
	count := 0
	for _, f := range fs {
		if f.Rule == "dangling-ref" && f.Severity == Error {
			count++
		}
	}
	if count != 3 {
		t.Errorf("want 3 dangling-ref errors, got %d: %v", count, fs)
	}
}

func TestUnapprovedRequirement(t *testing.T) {
	s := newSet([]*spec.Requirement{req("PLM-001", "egg")},
		[]*spec.Task{task("FTHR-001", "PLM-001", "pipping")})
	if !hasRule(Run(s, nil, ""), "unhatched-plumage", Error) {
		t.Error("want unhatched-plumage error")
	}
	// done requirement is fine
	s2 := newSet([]*spec.Requirement{req("PLM-001", "fledged")},
		[]*spec.Task{task("FTHR-001", "PLM-001", "fledged")})
	if hasRule(Run(s2, nil, ""), "unhatched-plumage", Error) {
		t.Error("done requirement should not trigger unhatched-plumage")
	}
}

func TestCycleRule(t *testing.T) {
	t1 := task("FTHR-001", "PLM-001", "egg", "FTHR-002")
	t2 := task("FTHR-002", "PLM-001", "egg", "FTHR-001")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{t1, t2})
	if !hasRule(Run(s, nil, ""), "cycle", Error) {
		t.Error("want cycle error")
	}
}

func TestTestsSectionRequired(t *testing.T) {
	noTests := task("FTHR-001", "PLM-001", "pipping")
	noTests.Body = []byte("## Description\nx\n## Acceptance Criteria\nx\n")
	emptyTests := task("FTHR-002", "PLM-001", "pipping")
	emptyTests.Body = []byte("## Description\nx\n## Tests\n\n## Acceptance Criteria\nx\n")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{noTests, emptyTests})
	fs := Run(s, nil, "")
	count := 0
	for _, f := range fs {
		if f.Rule == "tests-section" && f.Severity == Error {
			count++
		}
	}
	if count != 2 {
		t.Errorf("want 2 tests-section errors, got %d: %v", count, fs)
	}
}

func TestRequiredSectionsWarning(t *testing.T) {
	bare := task("FTHR-001", "PLM-001", "pipping")
	bare.Body = []byte("## Tests\n- x\n")
	bareReq := req("PLM-002", "hatched")
	bareReq.Body = []byte("## Context\nx\n")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched"), bareReq}, []*spec.Task{bare})
	fs := Run(s, nil, "")
	count := 0
	for _, f := range fs {
		if f.Rule == "required-sections" && f.Severity == Warning {
			count++
		}
	}
	// bare task missing Description + AC (2), bare req missing FC + AC (2)
	if count != 4 {
		t.Errorf("want 4 required-sections warnings, got %d: %v", count, fs)
	}
}

func TestStaleReadyHint(t *testing.T) {
	dep := task("FTHR-001", "PLM-001", "fledged")
	staleReady := task("FTHR-002", "PLM-001", "pipping", "FTHR-003")   // dep not done → warn
	satisfiedBlocked := task("FTHR-003", "PLM-001", "egg", "FTHR-001") // routine, no warning
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{dep, staleReady, satisfiedBlocked})
	fs := Run(s, nil, "")
	count := 0
	var files []string
	for _, f := range fs {
		if f.Rule == "stale-pipping-hint" && f.Severity == Warning {
			count++
			files = append(files, f.File)
		}
	}
	if count != 1 || !strings.Contains(files[0], "FTHR-002") {
		t.Errorf("want exactly 1 stale-pipping-hint warning on FTHR-002, got %d: %v", count, fs)
	}
}

func TestLockConsistency(t *testing.T) {
	notStarted := task("FTHR-001", "PLM-001", "pipping")
	inProgress := task("FTHR-002", "PLM-001", "hatching")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{notStarted, inProgress})
	// FTHR-001 locked but not in-progress; FTHR-002 in-progress but not locked
	fs := Run(s, []string{"FTHR-001"}, "")
	count := 0
	for _, f := range fs {
		if f.Rule == "brood-consistency" && f.Severity == Warning {
			count++
		}
	}
	if count != 2 {
		t.Errorf("want 2 brood-consistency warnings, got %d: %v", count, fs)
	}
}

func TestCriteriaIncomplete(t *testing.T) {
	doneUnchecked := task("FTHR-001", "PLM-001", "fledged")
	doneUnchecked.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [x] AC-1: verified\n- [ ] AC-2: not yet\n")
	doneChecked := task("FTHR-002", "PLM-001", "fledged")
	doneChecked.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [x] AC-1: verified\n")
	inFlight := task("FTHR-003", "PLM-001", "hatching")
	inFlight.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [ ] AC-1: pending\n")
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{doneUnchecked, doneChecked, inFlight})
	fs := Run(s, []string{"FTHR-003"}, "")
	var files []string
	for _, f := range fs {
		if f.Rule == "criteria-incomplete" && f.Severity == Error {
			files = append(files, f.File)
		}
	}
	if len(files) != 1 || !strings.Contains(files[0], "FTHR-001") {
		t.Errorf("want exactly 1 criteria-incomplete error on FTHR-001, got %v: %v", files, fs)
	}
}

func TestCriteriaIncompleteRequirement(t *testing.T) {
	doneReq := req("PLM-001", "fledged")
	doneReq.Body = []byte("## Context\nx\n## Functional Criteria\nx\n## Acceptance Criteria\n- [ ] AC-1: unmet\n")
	s := newSet([]*spec.Requirement{doneReq}, nil)
	if !hasRule(Run(s, nil, ""), "criteria-incomplete", Error) {
		t.Error("want criteria-incomplete error on done requirement with unchecked box")
	}
	approvedReq := req("PLM-002", "hatched")
	approvedReq.Body = doneReq.Body
	s2 := newSet([]*spec.Requirement{approvedReq}, nil)
	if hasRule(Run(s2, nil, ""), "criteria-incomplete", Error) {
		t.Error("approved requirement with unchecked boxes should not error")
	}
}

func TestCriteriaFormatLegacyWarning(t *testing.T) {
	legacyDone := task("FTHR-001", "PLM-001", "fledged") // AC section is prose "x"
	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{legacyDone})
	if !hasRule(Run(s, nil, ""), "criteria-format", Warning) {
		t.Error("want criteria-format warning for done task without parseable checkboxes")
	}
	legacyReady := task("FTHR-002", "PLM-001", "pipping")
	s2 := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{legacyReady})
	if hasRule(Run(s2, nil, ""), "criteria-format", Warning) {
		t.Error("non-done task without checkboxes should not warn")
	}
}

func TestCriteriaEvidence(t *testing.T) {
	dir := t.TempDir()
	withEvidence := task("FTHR-001", "PLM-001", "hatching")
	withEvidence.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [x] AC-1: verified\n")
	missingSection := task("FTHR-002", "PLM-001", "hatching")
	missingSection.Body = withEvidence.Body
	noFile := task("FTHR-003", "PLM-001", "hatching")
	noFile.Body = withEvidence.Body
	unchecked := task("FTHR-004", "PLM-001", "hatching")
	unchecked.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [ ] AC-1: pending\n")

	writeFile := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("FTHR-001.md", "# Evidence\n## AC-1\noutput\n")
	writeFile("FTHR-002.md", "# Evidence\n## AC-9\nwrong section\n")

	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")},
		[]*spec.Task{withEvidence, missingSection, noFile, unchecked})
	fs := Run(s, []string{"FTHR-001", "FTHR-002", "FTHR-003", "FTHR-004"}, dir)
	var files []string
	for _, f := range fs {
		if f.Rule == "criteria-evidence" && f.Severity == Warning {
			files = append(files, f.File)
		}
	}
	if len(files) != 2 {
		t.Fatalf("want 2 criteria-evidence warnings, got %v: %v", files, fs)
	}
	if !strings.Contains(files[0], "FTHR-002") || !strings.Contains(files[1], "FTHR-003") {
		t.Errorf("warnings on wrong files: %v", files)
	}

	// empty evidenceDir disables the rule
	if hasRule(Run(s, []string{"FTHR-001", "FTHR-002", "FTHR-003", "FTHR-004"}, ""), "criteria-evidence", Warning) {
		t.Error("evidence rule should be disabled with empty dir")
	}
}

func TestCriteriaEvidenceLabeledHeadingMessage(t *testing.T) {
	dir := t.TempDir()
	labeled := task("FTHR-001", "PLM-001", "hatching")
	labeled.Body = []byte("## Description\nx\n## Tests\n- a test\n## Acceptance Criteria\n- [x] AC-1: verified\n")

	// Evidence file uses a labeled heading instead of the bare "## AC-1"
	// form, so the checked criterion is reported as missing evidence.
	if err := os.WriteFile(filepath.Join(dir, "FTHR-001.md"),
		[]byte("# Evidence\n## AC-1: failing test capture\noutput\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSet([]*spec.Requirement{req("PLM-001", "hatched")}, []*spec.Task{labeled})
	fs := Run(s, []string{"FTHR-001"}, dir)
	var msg string
	for _, f := range fs {
		if f.Rule == "criteria-evidence" && f.Severity == Warning {
			msg = f.Message
		}
	}
	if msg == "" {
		t.Fatalf("want a criteria-evidence warning, got %v", fs)
	}
	if !strings.Contains(msg, `"## AC-N"`) {
		t.Errorf("message should name the required bare heading form %q, got: %s", `"## AC-N"`, msg)
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors([]Finding{{Severity: Warning}}) {
		t.Error("warnings alone are not errors")
	}
	if !HasErrors([]Finding{{Severity: Warning}, {Severity: Error}}) {
		t.Error("want true with an error present")
	}
}
