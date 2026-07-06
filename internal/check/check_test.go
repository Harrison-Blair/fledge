package check

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func req(id, status string) *spec.Requirement {
	return &spec.Requirement{
		ID: id, Title: "t", Status: status, Priority: "P1",
		Authored: "2026-07-06T12:00:00Z", Agent: "a", FledgeVersion: "0.1.0",
		Path: "spec/requirements/" + id + "-t.md",
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
		Path: "spec/tasks/" + id + "-t.md",
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
		[]*spec.Requirement{req("REQ-001", "approved")},
		[]*spec.Task{task("TASK-001", "REQ-001", "ready")},
	)
	if fs := Run(s, nil); len(fs) != 0 {
		t.Errorf("clean set produced findings: %v", fs)
	}
}

func TestParseErrorsSurface(t *testing.T) {
	s := newSet(nil, nil)
	s.Errors = []spec.FileError{{Path: "spec/tasks/TASK-009-x.md", Err: errors.New("boom")}}
	if !hasRule(Run(s, nil), "parse", Error) {
		t.Error("want parse error finding")
	}
}

func TestUnknownFieldWarning(t *testing.T) {
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")}, nil)
	s.UnknownFields["spec/requirements/REQ-001-t.md"] = []string{"extra"}
	if !hasRule(Run(s, nil), "unknown-field", Warning) {
		t.Error("want unknown-field warning")
	}
}

func TestSchemaRules(t *testing.T) {
	badStatus := task("TASK-001", "REQ-001", "wip")
	missingTitle := task("TASK-002", "REQ-001", "ready")
	missingTitle.Title = ""
	badPriority := task("TASK-003", "REQ-001", "ready")
	badPriority.Priority = "P9"
	badOversight := task("TASK-004", "REQ-001", "ready")
	badOversight.Oversight = "always"
	badAuthored := task("TASK-005", "REQ-001", "ready")
	badAuthored.Authored = "yesterday"
	badReqStatus := req("REQ-002", "pending")

	s := newSet(
		[]*spec.Requirement{req("REQ-001", "approved"), badReqStatus},
		[]*spec.Task{badStatus, missingTitle, badPriority, badOversight, badAuthored},
	)
	fs := Run(s, nil)
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
	tk := task("TASK-001", "REQ-001", "ready")
	tk.Path = "spec/tasks/TASK-002-t.md" // filename says 002
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")}, []*spec.Task{tk})
	if !hasRule(Run(s, nil), "id-filename", Error) {
		t.Error("want id-filename error")
	}
}

func TestDuplicateIDs(t *testing.T) {
	t1 := task("TASK-001", "REQ-001", "ready")
	t2 := task("TASK-001", "REQ-001", "ready")
	t2.Path = "spec/tasks/TASK-001-other.md"
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")}, []*spec.Task{t1, t2})
	if !hasRule(Run(s, nil), "duplicate-id", Error) {
		t.Error("want duplicate-id error")
	}
}

func TestDanglingRefs(t *testing.T) {
	missingReq := task("TASK-001", "REQ-099", "ready")
	missingDep := task("TASK-002", "REQ-001", "blocked", "TASK-098")
	selfRef := task("TASK-003", "REQ-001", "blocked", "TASK-003")
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")},
		[]*spec.Task{missingReq, missingDep, selfRef})
	fs := Run(s, nil)
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
	s := newSet([]*spec.Requirement{req("REQ-001", "draft")},
		[]*spec.Task{task("TASK-001", "REQ-001", "ready")})
	if !hasRule(Run(s, nil), "unapproved-req", Error) {
		t.Error("want unapproved-req error")
	}
	// done requirement is fine
	s2 := newSet([]*spec.Requirement{req("REQ-001", "done")},
		[]*spec.Task{task("TASK-001", "REQ-001", "done")})
	if hasRule(Run(s2, nil), "unapproved-req", Error) {
		t.Error("done requirement should not trigger unapproved-req")
	}
}

func TestCycleRule(t *testing.T) {
	t1 := task("TASK-001", "REQ-001", "blocked", "TASK-002")
	t2 := task("TASK-002", "REQ-001", "blocked", "TASK-001")
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")}, []*spec.Task{t1, t2})
	if !hasRule(Run(s, nil), "cycle", Error) {
		t.Error("want cycle error")
	}
}

func TestTestsSectionRequired(t *testing.T) {
	noTests := task("TASK-001", "REQ-001", "ready")
	noTests.Body = []byte("## Description\nx\n## Acceptance Criteria\nx\n")
	emptyTests := task("TASK-002", "REQ-001", "ready")
	emptyTests.Body = []byte("## Description\nx\n## Tests\n\n## Acceptance Criteria\nx\n")
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")},
		[]*spec.Task{noTests, emptyTests})
	fs := Run(s, nil)
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
	bare := task("TASK-001", "REQ-001", "ready")
	bare.Body = []byte("## Tests\n- x\n")
	bareReq := req("REQ-002", "approved")
	bareReq.Body = []byte("## Context\nx\n")
	s := newSet([]*spec.Requirement{req("REQ-001", "approved"), bareReq}, []*spec.Task{bare})
	fs := Run(s, nil)
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
	dep := task("TASK-001", "REQ-001", "done")
	staleReady := task("TASK-002", "REQ-001", "ready", "TASK-003")   // dep not done → warn
	satisfiedBlocked := task("TASK-003", "REQ-001", "blocked", "TASK-001") // routine, no warning
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")},
		[]*spec.Task{dep, staleReady, satisfiedBlocked})
	fs := Run(s, nil)
	count := 0
	var files []string
	for _, f := range fs {
		if f.Rule == "stale-ready-hint" && f.Severity == Warning {
			count++
			files = append(files, f.File)
		}
	}
	if count != 1 || !strings.Contains(files[0], "TASK-002") {
		t.Errorf("want exactly 1 stale-ready-hint warning on TASK-002, got %d: %v", count, fs)
	}
}

func TestLockConsistency(t *testing.T) {
	notStarted := task("TASK-001", "REQ-001", "ready")
	inProgress := task("TASK-002", "REQ-001", "in-progress")
	s := newSet([]*spec.Requirement{req("REQ-001", "approved")},
		[]*spec.Task{notStarted, inProgress})
	// TASK-001 locked but not in-progress; TASK-002 in-progress but not locked
	fs := Run(s, []string{"TASK-001"})
	count := 0
	for _, f := range fs {
		if f.Rule == "lock-consistency" && f.Severity == Warning {
			count++
		}
	}
	if count != 2 {
		t.Errorf("want 2 lock-consistency warnings, got %d: %v", count, fs)
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
