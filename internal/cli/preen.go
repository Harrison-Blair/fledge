package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/bootstrap"
	"github.com/Harrison-Blair/fledge/internal/check"
)

func init() { register("preen", runCheck, "fledge preen [--strict] [--json]") }

func runCheck(args []string) int {
	fs := flag.NewFlagSet("preen", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	r, set, locked, code, ok := loadSet()
	if !ok {
		return code
	}
	findings := check.Run(set, locked, r.EvidenceDir())
	for i := range findings {
		findings[i].File = relPath(r.Root, findings[i].File)
	}

	// Scaffold drift check: load stamp, build expected, classify.
	scaffoldFindings, scaffoldJSON := scaffoldDrift(r.Root)
	findings = append(findings, scaffoldFindings...)

	failed := check.HasErrors(findings) || (*strict && len(findings) > 0)

	if *jsonOut {
		if findings == nil {
			findings = []check.Finding{}
		}
		emitJSON(map[string]any{
			"ok":       !failed,
			"findings": findings,
			"scaffold": scaffoldJSON,
		})
	} else {
		errs, warns := 0, 0
		for _, f := range findings {
			label := "WARN "
			if f.Severity == check.Error {
				label = "ERROR"
				errs++
			} else {
				warns++
			}
			fmt.Printf("%s %s: %s\n", label, f.File, f.Message)
		}
		switch {
		case len(findings) == 0:
			fmt.Printf("spec clean: %d plumages, %d feathers\n", len(set.Reqs), len(set.Tasks))
		default:
			fmt.Printf("%s\n", summaryLine(errs, warns))
		}
	}
	if failed {
		return ExitFail
	}
	return ExitOK
}

// scaffoldJSON is the --json shape for the scaffold section.
type scaffoldJSONOut struct {
	StampVersion  string          `json:"stampVersion"`
	BinaryVersion string          `json:"binaryVersion"`
	Files         []scaffoldEntry `json:"files"`
}

type scaffoldEntry struct {
	Path   string              `json:"path"`
	Status bootstrap.DriftStatus `json:"status"`
	Policy string              `json:"policy"`
}

// scaffoldDrift loads the stamp, builds the expected file set from the stamp's
// agents, runs DriftReport, and returns check.Findings (warnings) plus the JSON
// object for --json output.
//
// If .fledge/skills/ does not exist, init has never been run for this repo and
// no scaffold check is needed.
func scaffoldDrift(root string) ([]check.Finding, scaffoldJSONOut) {
	if !fileExists(filepath.Join(root, ".fledge", "skills")) {
		return nil, scaffoldJSONOut{BinaryVersion: binaryVersion}
	}

	stamp, err := bootstrap.LoadStamp(root)
	if err != nil {
		// Unreadable stamp is not a hard failure; report as a single warning.
		f := check.Finding{
			File:     ".fledge/scaffold.json",
			Rule:     "scaffold-drift",
			Severity: check.Warning,
			Message:  fmt.Sprintf("could not read scaffold stamp: %v", err),
		}
		return []check.Finding{f}, scaffoldJSONOut{BinaryVersion: binaryVersion}
	}

	if stamp == nil {
		// No stamp yet: adoption warning.
		f := check.Finding{
			File:     ".fledge/scaffold.json",
			Rule:     "scaffold-drift",
			Severity: check.Warning,
			Message:  "no scaffold stamp — run fledge init --refresh to adopt",
		}
		return []check.Finding{f}, scaffoldJSONOut{StampVersion: "", BinaryVersion: binaryVersion}
	}

	// Build the expected file map from the stamp's agents.
	expected := baseScaffoldEntries()
	for _, agentName := range stamp.Agents {
		m, mErr := bootstrap.FindAdapter(agentName)
		if mErr != nil || m == nil {
			continue // unknown adapter; skip gracefully
		}
		ef, efErr := bootstrap.ExpectedFiles(m, commandOrder)
		if efErr != nil {
			continue
		}
		for k, v := range ef {
			expected[k] = v
		}
	}

	drifts := bootstrap.DriftReport(root, stamp, expected)

	// Sort for deterministic output.
	sort.Slice(drifts, func(i, j int) bool { return drifts[i].Path < drifts[j].Path })

	// Build JSON files list (all entries, including up-to-date).
	files := make([]scaffoldEntry, 0, len(drifts))
	for _, d := range drifts {
		files = append(files, scaffoldEntry{Path: d.Path, Status: d.Status, Policy: d.Policy})
	}

	// Emit findings for non-up-to-date entries.
	var findings []check.Finding
	for _, d := range drifts {
		if d.Status == bootstrap.StatusUpToDate {
			continue
		}
		msg := scaffoldMessage(d)
		findings = append(findings, check.Finding{
			File:     d.Path,
			Rule:     "scaffold-drift",
			Severity: check.Warning,
			Message:  msg,
		})
	}

	return findings, scaffoldJSONOut{
		StampVersion:  stamp.FledgeVersion,
		BinaryVersion: binaryVersion,
		Files:         files,
	}
}

// scaffoldMessage returns an actionable human message for a non-up-to-date drift entry.
func scaffoldMessage(d bootstrap.Drift) string {
	switch d.Status {
	case bootstrap.StatusStale:
		return fmt.Sprintf("scaffold file is stale (unedited, refresh-safe) — run fledge init --refresh")
	case bootstrap.StatusModified:
		return fmt.Sprintf("scaffold file is user-edited; refresh will preserve it — run fledge init --refresh")
	case bootstrap.StatusMissing:
		return fmt.Sprintf("scaffold file is missing — run fledge init --refresh")
	case bootstrap.StatusObsolete:
		return fmt.Sprintf("scaffold file is obsolete (no longer shipped) — run fledge init --refresh to prune")
	default:
		return fmt.Sprintf("scaffold drift: %s", d.Status)
	}
}

func summaryLine(errs, warns int) string {
	var parts []string
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errs))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warns))
	}
	return strings.Join(parts, ", ")
}
