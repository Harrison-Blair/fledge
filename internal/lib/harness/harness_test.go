package harness

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestKinds(t *testing.T) {
	want := strings.Fields("pi claude codex gemini cursor devin agy cline omp mastracode opencode copilot kimi kiro droid amp grok hermes kilo qodercli qwen letta maki muse")
	got := Kinds()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	got[0] = "mutated"
	if Kinds()[0] != "pi" {
		t.Fatal("Kinds leaked its backing array")
	}
	if !IsKind("claude") || IsKind("mutated") || IsKind("") {
		t.Fatal("IsKind disagrees with Kinds")
	}
}

func TestEveryKindHasOneProfile(t *testing.T) {
	for _, kind := range Kinds() {
		p, ok := Lookup(kind)
		if !ok || p.Kind != kind {
			t.Fatalf("%s: no registry entry (%+v, %v)", kind, p, ok)
		}
	}
	if len(profiles) != len(Kinds()) {
		t.Fatalf("registry has %d profiles for %d kinds", len(profiles), len(Kinds()))
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Fatal("unknown kind found")
	}
	if rows := Capabilities("nope"); rows != nil {
		t.Fatalf("unknown kind has rows %v", rows)
	}
}

// The typed fields pause and spawn read; pinned for every kind so the
// migration from their switches cannot drift.
func TestInterruptAndModelPolicy(t *testing.T) {
	for _, kind := range Kinds() {
		p, _ := Lookup(kind)
		keys := []string{"esc"}
		switch kind {
		case "amp", "copilot", "opencode", "kilo":
			keys = []string{"esc", "esc"}
		case "droid", "grok", "hermes", "mastracode", "qodercli":
			keys = []string{"ctrl+c"}
		}
		if !reflect.DeepEqual(p.InterruptKeys, keys) {
			t.Errorf("%s keys %q want %q", kind, p.InterruptKeys, keys)
		}
		model := ModelPolicy{Flag: "--model"}
		if slices.Contains(strings.Fields("kiro amp muse mastracode qodercli"), kind) {
			model.Flag = ""
		}
		model.ShortConflict = slices.Contains(strings.Fields("codex gemini cline opencode kimi droid grok hermes kilo qwen letta maki"), kind)
		if kind == "hermes" {
			model.Prefix = []string{"chat"}
		}
		if !reflect.DeepEqual(p.Model, model) {
			t.Errorf("%s model %+v want %+v", kind, p.Model, model)
		}
	}
}

func TestResumeTemplates(t *testing.T) {
	want := map[string][]string{
		"claude":   {"--resume", "R"},
		"codex":    {"resume", "R"},
		"pi":       {"--session", "R"},
		"opencode": {"--session", "R"},
		"cursor":   {"--resume", "R"},
	}
	for _, kind := range Kinds() {
		p, _ := Lookup(kind)
		if args, ok := want[kind]; ok {
			if p.Resume == nil || !reflect.DeepEqual(p.Resume("R"), args) {
				t.Errorf("%s resume template wrong", kind)
			}
		} else if p.Resume != nil {
			t.Errorf("%s claims an unverified resume template", kind)
		}
	}
}

func TestCapabilitiesFixedRows(t *testing.T) {
	names := []Capability{Interrupt, ModelSelect, ModelDiscovery, Resume, SessionRef, LifecycleHooks, UsageTokens, UsageCost}
	want := "interrupt model_select model_discovery resume session_ref lifecycle_hooks usage_tokens usage_cost"
	var got []string
	for _, n := range names {
		got = append(got, string(n))
	}
	if strings.Join(got, " ") != want {
		t.Fatalf("capability names %q", got)
	}
	for _, kind := range Kinds() {
		rows := Capabilities(kind)
		if len(rows) != len(names) {
			t.Fatalf("%s: %d rows", kind, len(rows))
		}
		for i, r := range rows {
			if r.Name != names[i] || !slices.Contains([]Level{Fledge, Native, None, Unknown}, r.Level) || strings.TrimSpace(r.Evidence) == "" {
				t.Errorf("%s row %d: %+v", kind, i, r)
			}
		}
	}
}

// Plan D section 2.3: the verified rows for the five installed harnesses.
func TestInstalledRows(t *testing.T) {
	want := map[string]string{
		"claude":   "fledge fledge fledge native fledge native fledge none",
		"codex":    "fledge fledge fledge native fledge native fledge none",
		"pi":       "fledge fledge fledge native fledge native fledge fledge",
		"opencode": "fledge fledge fledge native fledge native fledge fledge",
		"cursor":   "fledge fledge fledge native fledge native none none",
	}
	for kind, levels := range want {
		if got := levelsOf(Capabilities(kind)); got != levels {
			t.Errorf("%s: got %q want %q", kind, got, levels)
		}
	}
}

// Uninstalled kinds get only what code already encodes and unknown elsewhere.
func TestUninstalledRows(t *testing.T) {
	want := map[string]string{
		"gemini":     "fledge fledge none unknown none none unknown unknown",
		"mastracode": "fledge none none unknown native native unknown unknown",
		"kiro":       "fledge none none unknown none none unknown unknown",
		"agy":        "fledge fledge none unknown native native unknown unknown",
	}
	for kind, levels := range want {
		rows := Capabilities(kind)
		if got := levelsOf(rows); got != levels {
			t.Errorf("%s: got %q want %q", kind, got, levels)
		}
		if !strings.Contains(rows[0].Evidence, "default binding, unverified") {
			t.Errorf("%s interrupt evidence %q", kind, rows[0].Evidence)
		}
	}
}

func levelsOf(rows []Row) string {
	var levels []string
	for _, r := range rows {
		levels = append(levels, string(r.Level))
	}
	return strings.Join(levels, " ")
}
