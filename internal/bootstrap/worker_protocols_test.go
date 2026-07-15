package bootstrap

import (
	"strings"
	"testing"
)

// workerProtocolsDoc returns the embedded worker-protocols.md contents.
func workerProtocolsDoc(t *testing.T) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/worker-protocols.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// skuaSection extracts the "## Skua" section body (up to the next top-level
// "## " heading or EOF).
func skuaSection(t *testing.T, doc string) string {
	t.Helper()
	idx := strings.Index(doc, "## Skua")
	if idx == -1 {
		t.Fatal("worker-protocols.md: no \"## Skua\" section found")
	}
	rest := doc[idx+len("## Skua"):]
	end := strings.Index(rest, "\n## ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// reviewingSection extracts the "### Reviewing a feather" subsection body.
func reviewingSection(t *testing.T, skua string) string {
	t.Helper()
	idx := strings.Index(skua, "### Reviewing a feather")
	if idx == -1 {
		t.Fatal("Skua section: no \"### Reviewing a feather\" subsection found")
	}
	rest := skua[idx+len("### Reviewing a feather"):]
	end := strings.Index(rest, "\n### ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// verdictSection extracts the "### Verdict" subsection body.
func verdictSection(t *testing.T, skua string) string {
	t.Helper()
	idx := strings.Index(skua, "### Verdict")
	if idx == -1 {
		t.Fatal("Skua section: no \"### Verdict\" subsection found")
	}
	rest := skua[idx+len("### Verdict"):]
	end := strings.Index(rest, "\n### ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// TestSkuaConcessionHardened: the concession rule in ### Verdict must require
// independently re-verified disproof before a finding withdraws, and must no
// longer contain the old lenient bare-assertion sentence.
func TestSkuaConcessionHardened(t *testing.T) {
	doc := workerProtocolsDoc(t)
	skua := skuaSection(t, doc)
	verdict := verdictSection(t, skua)

	oldSentence := "If a brooder pushes back on a finding with a fact verified to be correct, withdraw the finding"
	if strings.Contains(verdict, oldSentence) {
		t.Errorf("### Verdict still contains the old lenient concession sentence: %q", oldSentence)
	}

	if !strings.Contains(verdict, "re-verif") {
		t.Errorf("### Verdict must state the skua itself re-verifies the brooder's disproof before withdrawing a finding")
	}
	if !strings.Contains(verdict, "independently checkable") {
		t.Errorf("### Verdict must require the disproof to be independently checkable")
	}
	if !strings.Contains(verdict, "never withdraws a finding") {
		t.Errorf("### Verdict must state a bare/unverified counter-assertion never withdraws a finding")
	}
}

// TestSkuaEvidenceGuiltyUntilProven: the criteria-audit item in
// ### Reviewing a feather must make the guilty-until-proven default explicit.
func TestSkuaEvidenceGuiltyUntilProven(t *testing.T) {
	doc := workerProtocolsDoc(t)
	skua := skuaSection(t, doc)
	reviewing := reviewingSection(t, skua)

	if !strings.Contains(reviewing, "Criteria audit") {
		t.Fatal("### Reviewing a feather: no \"Criteria audit\" item found")
	}

	if !strings.Contains(reviewing, "NOT proof") {
		t.Errorf("### Reviewing a feather criteria-audit item must state ambiguous/incomplete/terse-log evidence is \"NOT proof\"")
	}
	if !strings.Contains(reviewing, "where cheap") {
		t.Errorf("### Reviewing a feather criteria-audit item must keep the \"where cheap\" re-run carve-out")
	}
	if !strings.Contains(reviewing, "must be sufficient to independently confirm") {
		t.Errorf("### Reviewing a feather criteria-audit item must require that for any command not re-run, the recorded output itself must be sufficient to independently confirm the claim")
	}
}

// TestSkuaRedTeamPass: a "Red-team pass" checklist item must exist, positioned
// after "Diff vs. spec" and before the "Scope and simplicity" item, and must
// direct the skua to report gaps as findings only, never fixing/committing.
func TestSkuaRedTeamPass(t *testing.T) {
	doc := workerProtocolsDoc(t)
	skua := skuaSection(t, doc)
	reviewing := reviewingSection(t, skua)

	diffIdx := strings.Index(reviewing, "Diff vs. spec")
	if diffIdx == -1 {
		t.Fatal("### Reviewing a feather: no \"Diff vs. spec\" item found")
	}
	redTeamIdx := strings.Index(reviewing, "Red-team pass")
	if redTeamIdx == -1 {
		t.Fatal("### Reviewing a feather: no \"Red-team pass\" item found")
	}
	scopeIdx := strings.Index(reviewing, "Scope and simplicity")
	if scopeIdx == -1 {
		t.Fatal("### Reviewing a feather: no \"Scope and simplicity\" item found")
	}

	if !(diffIdx < redTeamIdx && redTeamIdx < scopeIdx) {
		t.Errorf("expected order Diff vs. spec (%d) < Red-team pass (%d) < Scope and simplicity (%d)", diffIdx, redTeamIdx, scopeIdx)
	}

	redTeamEnd := scopeIdx
	redTeamText := reviewing[redTeamIdx:redTeamEnd]

	if !strings.Contains(redTeamText, "every") {
		t.Errorf("Red-team pass item must state it runs every review cycle")
	}
	if !strings.Contains(redTeamText, "reported as") && !strings.Contains(redTeamText, "findings only") && !strings.Contains(redTeamText, "never fix") {
		t.Errorf("Red-team pass item must state gaps are reported as findings only, never fixed/committed by the skua")
	}
}

// TestSkuaUnchangedInvariants: guards the 3-rejection sentence and the cited
// ## Brooder fix-loop sentence against incidental change (FC-7/FC-8).
func TestSkuaUnchangedInvariants(t *testing.T) {
	doc := workerProtocolsDoc(t)

	thirdRejection := "if a feather fails review 3 times, do NOT start a fourth cycle"
	if !strings.Contains(doc, thirdRejection) {
		t.Errorf("worker-protocols.md must still contain the third-rejection sentence verbatim: %q", thirdRejection)
	}

	brooderFixLoop := "Do not argue a finding with the skua past one round of clarification"
	if !strings.Contains(doc, brooderFixLoop) {
		t.Errorf("worker-protocols.md must still contain the Brooder fix-loop sentence verbatim: %q", brooderFixLoop)
	}
}
