package profile

import (
	_ "embed"
	"strings"
)

// Canonical instruction fragments shared by composed managed profiles.
var (
	//go:embed profiles/fledge-core.md
	coreFragment string

	//go:embed profiles/fledge-general.md
	generalWorkerFragment string

	//go:embed profiles/fledge-worker-report.md
	workerReportFragment string
)

// composeInstructions trims outer whitespace from each section, joins the
// sections with exactly one blank line, and ends the result with one newline.
func composeInstructions(sections ...string) string {
	trimmed := make([]string, len(sections))
	for i, section := range sections {
		trimmed[i] = strings.TrimSpace(section)
	}
	return strings.Join(trimmed, "\n\n") + "\n"
}

// managedProfile assembles managed-profile instructions in canonical order:
// the shared session core, then the given role sections, then the report
// protocol.
func managedProfile(roleSections ...string) string {
	sections := make([]string, 0, len(roleSections)+2)
	sections = append(sections, coreFragment)
	sections = append(sections, roleSections...)
	sections = append(sections, workerReportFragment)
	return composeInstructions(sections...)
}

// managedWorker assembles worker instructions, inserting the general
// managed-worker rules before any role addenda.
func managedWorker(roleAddenda ...string) string {
	sections := make([]string, 0, len(roleAddenda)+1)
	sections = append(sections, generalWorkerFragment)
	sections = append(sections, roleAddenda...)
	return managedProfile(sections...)
}

// managedManager assembles manager instructions from role sections without
// the general managed-worker rules.
func managedManager(roleSections ...string) string {
	return managedProfile(roleSections...)
}
