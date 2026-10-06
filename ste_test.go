// STE ratchet: structural ASD-STE100 checks over the agent instruction text,
// with a per-file baseline. Counts may only fall. The checks port the
// deterministic rules of the upstream ste-lint.py and add the glossary's
// "Do not use" column from STE-GLOSSARY.md.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// steFiles lists the instruction text the ratchet covers. Each entry is a
// filepath.Glob pattern. A later wave adds files here.
var steFiles = []string{
	"AGENTS.md",
	"internal/lib/profiles/builtin/*.md",
}

const (
	steGlossaryPath = "STE-GLOSSARY.md"
	steBaselinePath = "testdata/ste-baseline.json"
	steMaxWords     = 25
)

// steViolation is one rule match, reported as "file:line rule: text".
type steViolation struct {
	File string
	Line int
	Rule string
	Text string
}

func (v steViolation) String() string {
	return fmt.Sprintf("%s:%d %s: %s", v.File, v.Line, v.Rule, v.Text)
}

// steRule matches one banned construction. guardBefore and guardAfter stand in
// for the lookaround the upstream patterns use, which RE2 does not support: a
// match is dropped when guardBefore matches the text before it or guardAfter
// matches the text after it. prefer, when set, names the term that wins.
type steRule struct {
	id          string
	pattern     *regexp.Regexp
	guardBefore *regexp.Regexp
	guardAfter  *regexp.Regexp
	prefer      string
}

// Participle lists and patterns come from the upstream linter. The passive
// heuristic keeps its narrower list. The perfect heuristic also covers
// intransitive verbs.
const steIrregularParticiples = "given|taken|made|done|found|seen|known|shown|written|built|sent|set|run|read|kept|held|left|put|cut|hit|let|shut|split|spread|begun|become|come|gone|got|gotten|lost|met|paid|said|sold|told|thought|brought|bought|caught|taught|won|worn|torn|born|drawn|grown|thrown|flown|driven|risen|chosen|broken|spoken|frozen|hidden|ridden|forgotten|fallen|eaten|beaten|understood|stood|struck|stuck|swung|hung|led|fed|bled|fled|sped|bound|wound|dug|spun|slid|bit|lit|quit"

const stePassiveParticiples = "given|taken|made|done|found|seen|known|shown|written|built|sent|set|run|read|kept|held|left|put"

var steRules = []steRule{
	{
		id:      "semicolon",
		pattern: regexp.MustCompile(";"),
	},
	{
		id:      "phrasal-verb",
		pattern: regexp.MustCompile(`(?i)\b(spin(?:ning|s)? up|spun up|reach(?:ing|es|ed)? out|div(?:e|es|ing|ed) into|dove into|kick(?:ing|s|ed)? off|circl(?:e|es|ing|ed) back|touch(?:ing|es|ed)? base)\b`),
	},
	{
		id:         "passive-voice",
		pattern:    regexp.MustCompile(`(?i)\b(is|are|was|were|been|being)\s+(\w+ed|` + stePassiveParticiples + `)\b`),
		guardAfter: regexp.MustCompile(`(?i)^\s+(?:to|for|by)\s+\w+ing`),
	},
	{
		// A modal plus a perfect infinitive ("may have failed") is a hedge,
		// not a present perfect, and the skill never flags confidence.
		id:          "present-perfect",
		pattern:     regexp.MustCompile(`(?i)\b(has|have|had)\s+(?:been\s+)?(?:\w+(?:ed|en)|` + steIrregularParticiples + `)\b`),
		guardBefore: regexp.MustCompile(`(?i)\b(?:may|might|could|should|would|must|can|will|shall)(?:\s+not|n['\x{2019}]t)?\s+$`),
	},
}

// steSegment is one lintable run of prose and the source line it came from.
type steSegment struct {
	line int
	text string
}

var (
	steCodeFence  = regexp.MustCompile("^(```|~~~)")
	steInlineCode = regexp.MustCompile("`[^`]*`")
)

// steSegments drops fenced code and inline code spans, and splits a Markdown
// table row into cells so no sentence runs across a pipe. A cell that held
// only code is empty afterwards and contributes nothing.
func steSegments(text string) []steSegment {
	var out []steSegment
	inFence := false
	for i, raw := range strings.Split(text, "\n") {
		if steCodeFence.MatchString(strings.TrimSpace(raw)) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		cells := []string{raw}
		if strings.Contains(raw, "|") {
			cells = strings.Split(raw, "|")
		}
		for _, cell := range cells {
			cell = steInlineCode.ReplaceAllString(cell, "")
			if strings.TrimSpace(cell) == "" {
				continue
			}
			out = append(out, steSegment{line: i + 1, text: cell})
		}
	}
	return out
}

// steSentenceBreak ends a sentence: terminal punctuation then whitespace.
var steSentenceBreak = regexp.MustCompile(`[.!?]\s+`)

// steLongSentences flags a sentence above the word cap. Like the upstream
// linter, it measures one segment at a time, so a sentence that wraps across
// source lines is never measured. It catches a long single line or table cell.
func steLongSentences(file string, seg steSegment) []steViolation {
	var out []steViolation
	start := 0
	sentences := []string{}
	for _, m := range steSentenceBreak.FindAllStringIndex(seg.text, -1) {
		sentences = append(sentences, seg.text[start:m[0]+1])
		start = m[1]
	}
	sentences = append(sentences, seg.text[start:])
	for _, sentence := range sentences {
		if n := len(strings.Fields(sentence)); n > steMaxWords {
			out = append(out, steViolation{
				File: file,
				Line: seg.line,
				Rule: "long-sentence",
				Text: fmt.Sprintf("%d words (cap %d): %s", n, steMaxWords, strings.TrimSpace(sentence)),
			})
		}
	}
	return out
}

// steLint reports every violation of rules in text.
func steLint(file, text string, rules []steRule) []steViolation {
	var out []steViolation
	for _, seg := range steSegments(text) {
		for _, rule := range rules {
			for _, m := range rule.pattern.FindAllStringIndex(seg.text, -1) {
				match := seg.text[m[0]:m[1]]
				if rule.guardBefore != nil && rule.guardBefore.MatchString(seg.text[:m[0]]) {
					continue
				}
				if rule.guardAfter != nil && rule.guardAfter.MatchString(seg.text[m[1]:]) {
					continue
				}
				text := match
				if rule.prefer != "" {
					text = fmt.Sprintf("%q (use %q)", match, rule.prefer)
				}
				out = append(out, steViolation{File: file, Line: seg.line, Rule: rule.id, Text: text})
			}
		}
		out = append(out, steLongSentences(file, seg)...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// steGlossaryRules turns each word in the glossary's "Do not use" column into
// a rule, matched case-insensitively on word boundaries with the simple
// inflections -s, -es, -ed, -d and -ing.
func steGlossaryRules(glossary string) ([]steRule, error) {
	var rules []steRule
	for _, line := range strings.Split(glossary, "\n") {
		cells := strings.Split(line, "|")
		if len(cells) != 5 {
			continue
		}
		term := strings.TrimSpace(cells[1])
		banned := strings.TrimSpace(cells[3])
		if term == "" || term == "Term" || steTableRule.MatchString(term) {
			continue
		}
		for _, word := range strings.Split(banned, ",") {
			word = strings.TrimSpace(word)
			if word == "" {
				continue
			}
			pattern, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(word) + `(?:s|es|ed|d|ing)?\b`)
			if err != nil {
				return nil, fmt.Errorf("glossary term %s: %w", term, err)
			}
			rules = append(rules, steRule{id: "glossary", pattern: pattern, prefer: term})
		}
	}
	return rules, nil
}

// steTableRule matches a Markdown table separator cell.
var steTableRule = regexp.MustCompile(`^:?-{3,}:?$`)

// steExpand resolves the patterns in steFiles to sorted file paths.
func steExpand(patterns []string) ([]string, error) {
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern %s: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("pattern %s matches no file", pattern)
		}
		files = append(files, matches...)
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

// steCompare reports every way counts departs from baseline. The ratchet only
// lets a count fall, and a fallen count must be written back.
func steCompare(counts, baseline map[string]int) []string {
	var problems []string
	for _, file := range steKeys(counts) {
		base, ok := baseline[file]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: no baseline entry (now %d); add it to %s", file, counts[file], steBaselinePath))
		case counts[file] > base:
			problems = append(problems, fmt.Sprintf("%s: %d violations, baseline %d; fix the new ones", file, counts[file], base))
		case counts[file] < base:
			problems = append(problems, fmt.Sprintf("%s: %d violations, baseline %d; lower the baseline in %s to %d", file, counts[file], base, steBaselinePath, counts[file]))
		}
	}
	for _, file := range steKeys(baseline) {
		if _, ok := counts[file]; !ok {
			problems = append(problems, fmt.Sprintf("%s: baseline names a file the ratchet does not check; remove it from %s", file, steBaselinePath))
		}
	}
	return problems
}

// steKeys returns the keys of m in sorted order.
func steKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestSTESemicolon(t *testing.T) {
	steRuleCases(t, "semicolon", []steCase{
		{"pass", "Split the work into separate sentences.", 0},
		{"fail", "The panel is clean; start the job.", 1},
	})
}

func TestSTEPassiveVoice(t *testing.T) {
	steRuleCases(t, "passive-voice", []steCase{
		{"pass", "The verifier runs the suite.", 0},
		{"fail", "The suite is run by the verifier.", 1},
	})
}

func TestSTEPresentPerfect(t *testing.T) {
	steRuleCases(t, "present-perfect", []steCase{
		{"pass", "The task ran.", 0},
		{"fail", "The task has run.", 1},
		{"modal perfect stays a hedge", "The task may have run.", 0},
	})
}

func TestSTELongSentence(t *testing.T) {
	steRuleCases(t, "long-sentence", []steCase{
		{"pass", strings.TrimSpace(strings.Repeat("word ", steMaxWords)) + ".", 0},
		{"fail", strings.TrimSpace(strings.Repeat("word ", steMaxWords+1)) + ".", 1},
	})
}

func TestSTEPhrasalVerb(t *testing.T) {
	steRuleCases(t, "phrasal-verb", []steCase{
		{"pass", "Start the job and contact the owner.", 0},
		{"fail", "Spin up the job and reach out to the owner.", 2},
	})
}

func TestSTEGlossaryTerm(t *testing.T) {
	const glossary = "# STE glossary\n\n" +
		"| Term | Meaning | Do not use |\n" +
		"| --- | --- | --- |\n" +
		"| check | Run the suite yourself. | confirm, validate |\n" +
		"| spawn | Start a new agent. | launch |\n" +
		"| task | One unit of tracked work. | work unit |\n" +
		"| verify | Record a passed independent check. | |\n"
	rules, err := steGlossaryRules(glossary)
	if err != nil {
		t.Fatalf("steGlossaryRules: %v", err)
	}
	cases := []steCase{
		{"pass", "Check the output, then spawn the next worker and verify the task.", 0},
		{"fail", "Confirm the output, then launch the next worker.", 2},
		{"inflections count", "The run validated the launches.", 2},
		{"multi-word term", "Record each work unit.", 1},
		{"word boundary", "Stopping the pane is not a ping.", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := steLint("x.md", c.text, rules)
			if len(got) != c.want {
				t.Fatalf("got %d violations, want %d:\n%s", len(got), c.want, steFormat(got))
			}
			for _, v := range got {
				if v.Rule != "glossary" {
					t.Errorf("rule = %q, want glossary", v.Rule)
				}
			}
		})
	}
}

type steCase struct {
	name string
	text string
	want int
}

// steRuleCases checks that each case yields exactly want violations of rule.
func steRuleCases(t *testing.T, rule string, cases []steCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []steViolation
			for _, v := range steLint("x.md", c.text, steRules) {
				if v.Rule == rule {
					got = append(got, v)
				}
			}
			if len(got) != c.want {
				t.Fatalf("got %d %s violations, want %d:\n%s", len(got), rule, c.want, steFormat(got))
			}
		})
	}
}

// steFormat renders violations one per line.
func steFormat(violations []steViolation) string {
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		lines = append(lines, "  "+v.String())
	}
	return strings.Join(lines, "\n")
}

func TestSTESegmentsSkipCode(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []steSegment
	}{
		{"fenced code", "```\nx = a; y = b\n```", nil},
		{"tilde fence", "~~~\nx = a; y = b\n~~~", nil},
		{"inline code span", "Run `go vet ./...` now.", []steSegment{{1, "Run  now."}}},
		{"table row of only code", "| `--name` | `fledge agent stop` |", nil},
		{"table cells split", "| Stop | End an agent. |", []steSegment{{1, " Stop "}, {1, " End an agent. "}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := steSegments(c.text)
			if len(got) != len(c.want) {
				t.Fatalf("got %d segments %v, want %d %v", len(got), got, len(c.want), c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("segment %d = %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestSTECompare(t *testing.T) {
	cases := []struct {
		name     string
		counts   map[string]int
		baseline map[string]int
		want     string
	}{
		{"equal", map[string]int{"a.md": 3}, map[string]int{"a.md": 3}, ""},
		{"above baseline", map[string]int{"a.md": 4}, map[string]int{"a.md": 3}, "fix the new ones"},
		{"below baseline", map[string]int{"a.md": 2}, map[string]int{"a.md": 3}, "lower the baseline"},
		{"missing entry", map[string]int{"a.md": 2}, map[string]int{}, "no baseline entry"},
		{"stale entry", map[string]int{}, map[string]int{"a.md": 1}, "does not check"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := steCompare(c.counts, c.baseline)
			if c.want == "" {
				if len(problems) != 0 {
					t.Fatalf("got problems %v, want none", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("got %d problems %v, want 1", len(problems), problems)
			}
			if !strings.Contains(problems[0], c.want) {
				t.Errorf("problem %q does not mention %q", problems[0], c.want)
			}
		})
	}
}

// TestSTERatchet is the ratchet: every checked file must stay at or below its
// baseline count, and a fallen count must be written back.
func TestSTERatchet(t *testing.T) {
	glossary, err := os.ReadFile(steGlossaryPath)
	if err != nil {
		t.Fatalf("read glossary: %v", err)
	}
	rules, err := steGlossaryRules(string(glossary))
	if err != nil {
		t.Fatalf("glossary rules: %v", err)
	}
	if len(rules) == 0 {
		t.Fatalf("%s yielded no rule: check its table format", steGlossaryPath)
	}
	rules = append(slices.Clone(steRules), rules...)

	files, err := steExpand(steFiles)
	if err != nil {
		t.Fatalf("expand steFiles: %v", err)
	}
	counts := map[string]int{}
	byFile := map[string][]steViolation{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		violations := steLint(filepath.ToSlash(file), string(data), rules)
		counts[filepath.ToSlash(file)] = len(violations)
		byFile[filepath.ToSlash(file)] = violations
	}

	raw, err := os.ReadFile(steBaselinePath)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	baseline := map[string]int{}
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("parse %s: %v", steBaselinePath, err)
	}

	problems := steCompare(counts, baseline)
	if len(problems) == 0 {
		return
	}
	var report strings.Builder
	for _, p := range problems {
		fmt.Fprintf(&report, "%s\n", p)
	}
	for _, p := range problems {
		file := strings.SplitN(p, ":", 2)[0]
		if violations := byFile[file]; len(violations) > 0 {
			fmt.Fprintf(&report, "\n%s\n%s\n", file, steFormat(violations))
		}
	}
	current, err := json.MarshalIndent(counts, "", "  ")
	if err != nil {
		t.Fatalf("encode counts: %v", err)
	}
	fmt.Fprintf(&report, "\nnew counts for %s:\n%s\n", steBaselinePath, current)
	t.Fatal(report.String())
}
