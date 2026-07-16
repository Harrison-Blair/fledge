package bootstrap

import (
	"strings"
	"testing"
)

// skuaDoc returns the embedded skua.md contents.
func skuaDoc(t *testing.T) string {
	t.Helper()
	data, err := FS.ReadFile("core/skills/fledge-orchestrate/skua.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// skuaReviewingSection extracts the "### Reviewing a feather" subsection body.
func skuaReviewingSection(t *testing.T, doc string) string {
	t.Helper()
	idx := strings.Index(doc, "### Reviewing a feather")
	if idx == -1 {
		t.Fatal("skua.md: no \"### Reviewing a feather\" subsection found")
	}
	rest := doc[idx+len("### Reviewing a feather"):]
	end := strings.Index(rest, "\n### ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// skuaVerdictSection extracts the "### Verdict" subsection body.
func skuaVerdictSection(t *testing.T, doc string) string {
	t.Helper()
	idx := strings.Index(doc, "### Verdict")
	if idx == -1 {
		t.Fatal("skua.md: no \"### Verdict\" subsection found")
	}
	rest := doc[idx+len("### Verdict"):]
	end := strings.Index(rest, "\n### ")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// TestSkuaDocSections: skua.md is titled "# Skua" and contains its four
// subsection headings, plus the third-rejection and pass-signal key phrases.
func TestSkuaDocSections(t *testing.T) {
	doc := skuaDoc(t)

	if !strings.HasPrefix(doc, "# Skua") {
		t.Errorf("skua.md must start with \"# Skua\", got %q", firstLine(doc))
	}

	for _, heading := range []string{
		"### Communication rules",
		"### Reviewing a feather",
		"### Verdict",
		"### Lifecycle",
	} {
		if !strings.Contains(doc, heading) {
			t.Errorf("skua.md must contain the %q subsection heading", heading)
		}
	}

	thirdRejection := "if a feather fails review 3 times, do NOT start a fourth cycle"
	if !strings.Contains(doc, thirdRejection) {
		t.Errorf("skua.md must still contain the third-rejection sentence verbatim: %q", thirdRejection)
	}

	passSignal := "the only merge signal"
	if !strings.Contains(doc, passSignal) {
		t.Errorf("skua.md must still contain the pass-signal phrasing: %q", passSignal)
	}
}

// TestSkuaConcessionHardened: the concession rule in ### Verdict must require
// independently re-verified disproof before a finding withdraws, and must no
// longer contain the old lenient bare-assertion sentence.
func TestSkuaConcessionHardened(t *testing.T) {
	verdict := skuaVerdictSection(t, skuaDoc(t))

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
	reviewing := skuaReviewingSection(t, skuaDoc(t))

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
	reviewing := skuaReviewingSection(t, skuaDoc(t))

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
