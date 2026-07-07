// Package check validates a loaded spec set and reports findings.
package check

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/graph"
	"github.com/Harrison-Blair/fledge/internal/spec"
)

// Severity of a finding.
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Finding is one validation result attributed to a file.
type Finding struct {
	File     string   `json:"file"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

var (
	reqIDRe  = regexp.MustCompile(`^PLM-\d{3,}$`)
	taskIDRe = regexp.MustCompile(`^FTHR-\d{3,}$`)
)

// Run validates the set. lockedTasks are feather IDs with a held brood file.
// evidenceDir is where per-feather evidence files (FTHR-###.md) live; empty
// disables the criteria-evidence rule.
func Run(set *spec.Set, lockedTasks []string, evidenceDir string) []Finding {
	var fs []Finding
	add := func(file, rule string, sev Severity, format string, a ...any) {
		fs = append(fs, Finding{File: file, Rule: rule, Severity: sev, Message: fmt.Sprintf(format, a...)})
	}

	// parse
	for _, fe := range set.Errors {
		add(fe.Path, "parse", Error, "%v", fe.Err)
	}
	// unknown-field
	paths := make([]string, 0, len(set.UnknownFields))
	for p := range set.UnknownFields {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		add(p, "unknown-field", Warning, "unknown frontmatter keys: %s", strings.Join(set.UnknownFields[p], ", "))
	}

	// duplicate-id
	seen := map[string]string{}
	for _, r := range set.Reqs {
		if prev, ok := seen[r.ID]; ok {
			add(r.Path, "duplicate-id", Error, "%s already defined in %s", r.ID, prev)
		} else {
			seen[r.ID] = r.Path
		}
	}
	for _, t := range set.Tasks {
		if prev, ok := seen[t.ID]; ok {
			add(t.Path, "duplicate-id", Error, "%s already defined in %s", t.ID, prev)
		} else {
			seen[t.ID] = t.Path
		}
	}

	for _, r := range set.Reqs {
		checkRequired(add, r.Path, map[string]string{
			"id": r.ID, "title": r.Title, "status": r.Status, "priority": r.Priority,
			"authored": r.Authored, "agent": r.Agent, "fledge_version": r.FledgeVersion,
		})
		checkEnum(add, r.Path, "status", r.Status, []string{spec.ReqEgg, spec.ReqHatched, spec.ReqFledged})
		checkEnum(add, r.Path, "priority", r.Priority, spec.Priorities)
		checkAuthored(add, r.Path, r.Authored)
		checkIDFilename(add, r.Path, r.ID, reqIDRe)
		for _, h := range []string{"## Functional Criteria", "## Acceptance Criteria"} {
			if !hasSection(r.Body, h) {
				add(r.Path, "required-sections", Warning, "missing %q section", h)
			}
		}
		if r.Status == spec.ReqFledged {
			checkCriteriaComplete(add, r.Path, r.Body)
		}
	}

	for _, t := range set.Tasks {
		checkRequired(add, t.Path, map[string]string{
			"id": t.ID, "title": t.Title, "plumage": t.Requirement, "status": t.Status,
			"priority": t.Priority, "authored": t.Authored, "agent": t.Agent,
			"fledge_version": t.FledgeVersion,
		})
		checkEnum(add, t.Path, "status", t.Status,
			[]string{spec.TaskEgg, spec.TaskPipping, spec.TaskHatching, spec.TaskFledged})
		checkEnum(add, t.Path, "priority", t.Priority, spec.Priorities)
		if t.Oversight != "" && !slices.Contains(spec.OversightValues, t.Oversight) {
			add(t.Path, "schema", Error, "oversight %q not one of merge|during", t.Oversight)
		}
		checkAuthored(add, t.Path, t.Authored)
		checkIDFilename(add, t.Path, t.ID, taskIDRe)

		// dangling-ref / unhatched-plumage
		if t.Requirement != "" {
			if req := set.Req(t.Requirement); req == nil {
				add(t.Path, "dangling-ref", Error, "plumage %s does not exist", t.Requirement)
			} else if req.Status == spec.ReqEgg {
				add(t.Path, "unhatched-plumage", Error, "plumage %s is still an egg", t.Requirement)
			}
		}
		for _, dep := range t.DependsOn {
			if dep == t.ID {
				add(t.Path, "dangling-ref", Error, "depends_on references itself")
			} else if set.Task(dep) == nil {
				add(t.Path, "dangling-ref", Error, "depends_on references unknown %s", dep)
			}
		}

		// tests-section
		if !sectionNonEmpty(t.Body, "## Tests") {
			add(t.Path, "tests-section", Error, "missing or empty \"## Tests\" section")
		}
		for _, h := range []string{"## Description", "## Acceptance Criteria"} {
			if !hasSection(t.Body, h) {
				add(t.Path, "required-sections", Warning, "missing %q section", h)
			}
		}

		if t.Status == spec.TaskFledged {
			checkCriteriaComplete(add, t.Path, t.Body)
		}
		if evidenceDir != "" {
			checkCriteriaEvidence(add, t, evidenceDir)
		}

		// stale-pipping-hint: only the over-promising direction is suspicious.
		// egg-with-deps-fledged is routine (egg→pipping is never written
		// back; readiness is recomputed at dispatch).
		if t.Status == spec.TaskPipping && !allDepsDone(set, t) {
			add(t.Path, "stale-pipping-hint", Warning, "status says pipping but not all depends_on are fledged")
		}
	}

	// cycle
	if c := graph.New(set.Tasks).Cycle(); c != nil {
		file := ""
		if t := set.Task(c[0]); t != nil {
			file = t.Path
		}
		add(file, "cycle", Error, "dependency cycle: %s", strings.Join(c, " -> "))
	}

	// brood-consistency
	for _, id := range lockedTasks {
		t := set.Task(id)
		if t == nil {
			add(id, "brood-consistency", Warning, "brood held for unknown feather %s", id)
		} else if t.Status != spec.TaskHatching {
			add(t.Path, "brood-consistency", Warning, "brood held but status is %s (expected hatching)", t.Status)
		}
	}
	for _, t := range set.Tasks {
		if t.Status == spec.TaskHatching && !slices.Contains(lockedTasks, t.ID) {
			add(t.Path, "brood-consistency", Warning, "status hatching but no brood is held")
		}
	}

	return fs
}

// HasErrors reports whether any finding is error-severity.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == Error {
			return true
		}
	}
	return false
}

func allDepsDone(set *spec.Set, t *spec.Task) bool {
	for _, dep := range t.DependsOn {
		d := set.Task(dep)
		if d == nil || d.Status != spec.TaskFledged {
			return false
		}
	}
	return true
}

type addFunc func(file, rule string, sev Severity, format string, a ...any)

func checkRequired(add addFunc, path string, fields map[string]string) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if fields[k] == "" {
			add(path, "schema", Error, "missing required field %q", k)
		}
	}
}

func checkEnum(add addFunc, path, field, value string, allowed []string) {
	if value == "" {
		return // covered by checkRequired
	}
	if !slices.Contains(allowed, value) {
		add(path, "schema", Error, "%s %q not one of %s", field, value, strings.Join(allowed, "|"))
	}
}

func checkAuthored(add addFunc, path, authored string) {
	if authored == "" {
		return
	}
	if _, err := time.Parse(time.RFC3339, authored); err != nil {
		add(path, "schema", Error, "authored %q is not RFC 3339", authored)
	}
}

func checkIDFilename(add addFunc, path, id string, re *regexp.Regexp) {
	if id == "" {
		return
	}
	if !re.MatchString(id) {
		add(path, "schema", Error, "id %q has invalid format", id)
		return
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, id+"-") && base != id+".md" {
		add(path, "id-filename", Error, "filename %q does not match id %s", base, id)
	}
}

// checkCriteriaComplete flags a fledged spec whose AC boxes aren't all checked
// (error), or that has no parseable checkboxes at all (legacy format, warning).
func checkCriteriaComplete(add addFunc, path string, body []byte) {
	cs := spec.ParseCriteria(body)
	if len(cs) == 0 {
		add(path, "criteria-format", Warning, "fledged but \"## Acceptance Criteria\" has no parseable checkboxes (expected \"- [ ] AC-N: ...\")")
		return
	}
	var unchecked []string
	for _, c := range cs {
		if !c.Checked {
			unchecked = append(unchecked, c.Label)
		}
	}
	if len(unchecked) > 0 {
		add(path, "criteria-incomplete", Error, "fledged but acceptance criteria unchecked: %s", strings.Join(unchecked, ", "))
	}
}

// checkCriteriaEvidence warns when a checked feather AC has no matching "## AC-N"
// section in the feather's molt evidence file under evidenceDir.
func checkCriteriaEvidence(add addFunc, t *spec.Task, evidenceDir string) {
	var checked []spec.Criterion
	for _, c := range spec.ParseCriteria(t.Body) {
		if c.Checked {
			checked = append(checked, c)
		}
	}
	if len(checked) == 0 {
		return
	}
	evPath := filepath.Join(evidenceDir, t.ID+".md")
	ev, err := os.ReadFile(evPath)
	if err != nil {
		add(t.Path, "criteria-evidence", Warning, "criteria checked but evidence file %s is missing", evPath)
		return
	}
	var missing []string
	for _, c := range checked {
		if !hasSection(ev, "## "+c.Label) {
			missing = append(missing, c.Label)
		}
	}
	if len(missing) > 0 {
		add(t.Path, "criteria-evidence", Warning, "checked criteria missing evidence sections in %s: %s", evPath, strings.Join(missing, ", "))
	}
}

// hasSection reports whether body contains a line equal to heading.
func hasSection(body []byte, heading string) bool {
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimRight(strings.TrimSuffix(line, "\r"), " ") == heading {
			return true
		}
	}
	return false
}

// sectionNonEmpty reports whether heading exists and has non-whitespace
// content before the next heading of any level.
func sectionNonEmpty(body []byte, heading string) bool {
	lines := strings.Split(string(body), "\n")
	in := false
	for _, raw := range lines {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimRight(line, " ")
		if in {
			if strings.HasPrefix(trimmed, "#") {
				return false
			}
			if strings.TrimSpace(line) != "" {
				return true
			}
			continue
		}
		if trimmed == heading {
			in = true
		}
	}
	return false
}
