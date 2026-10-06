package proposal

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/brief"
)

// valid is a brief filled in on the advisory template.
func valid() string {
	var b strings.Builder
	for _, s := range brief.Sections {
		b.WriteString("## " + s + "\ntext for " + s + "\n\n")
	}
	return b.String()
}

// task renders one [[tasks]] entry with a valid brief.
func task(key, after string) string {
	return "[[tasks]]\nkey = \"" + key + "\"\ntitle = \"Do " + key + "\"\nbrief = '''\n" + valid() + "'''\nafter = [" + after + "]\n\n"
}

const header = "schema_version = 1\n\n"

func TestDecodeAccepts(t *testing.T) {
	src := header + "[parent]\ntitle = \"Epic\"\nbrief = '''\n" + valid() + "'''\n\n" +
		task("a", "") + task("b", `"a", "0123abcd"`)
	p, err := Decode([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if p.Parent == nil || p.Parent.Title != "Epic" || p.Parent.Brief != valid() {
		t.Fatalf("parent = %+v", p.Parent)
	}
	if len(p.Tasks) != 2 || p.Tasks[0].Key != "a" || p.Tasks[0].Title != "Do a" || p.Tasks[0].Brief != valid() {
		t.Fatalf("tasks = %+v", p.Tasks)
	}
	if !slices.Equal(p.Tasks[1].After, []string{"a", "0123abcd"}) {
		t.Fatalf("after = %v", p.Tasks[1].After)
	}
}

// Briefs need not follow the template; decoded text is kept exactly.
func TestDecodeAcceptsAnyNonblankBrief(t *testing.T) {
	for _, tc := range []struct{ toml, want string }{
		{`"one line"`, "one line"},
		{`"  padded \n\n"`, "  padded \n\n"},
		{`"　résumé"`, "　résumé"},
		{"'''\n## Notes\nx\n'''", "## Notes\nx\n"},
	} {
		src := header + "[parent]\ntitle = \"E\"\nbrief = " + tc.toml + "\n\n[[tasks]]\nkey = \"a\"\ntitle = \"A\"\nbrief = " + tc.toml + "\n"
		p, err := Decode([]byte(src))
		if err != nil || p.Parent.Brief != tc.want || p.Tasks[0].Brief != tc.want {
			t.Fatalf("%s: %v %+v", tc.toml, err, p)
		}
	}
}

func TestDecodeWithoutParent(t *testing.T) {
	p, err := Decode([]byte(header + task("a", "")))
	if err != nil {
		t.Fatal(err)
	}
	if p.Parent != nil {
		t.Fatalf("parent = %+v", p.Parent)
	}
}

func TestDecodeInlineTasks(t *testing.T) {
	src := header + "tasks = [{ key = \"a\", title = \"A\", brief = " + strconv.Quote(valid()) + " }]\n"
	p, err := Decode([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 1 || p.Tasks[0].Key != "a" {
		t.Fatalf("tasks = %+v", p.Tasks)
	}
}

func TestDecodeRejects(t *testing.T) {
	good := task("a", "")
	for label, tc := range map[string]struct{ src, want string }{
		"syntax":                {header + "tasks = [", "toml"},
		"missing version":       {good, "schema_version is required"},
		"unsupported":           {"schema_version = 2\n" + good, "unsupported schema_version 2"},
		"unknown top":           {header + "extra = 1\n" + good, `unknown key "extra"`},
		"case variant top":      {"Schema_Version = 1\n" + good, `unknown key "Schema_Version"`},
		"unknown parent":        {header + "[parent]\ntitle = \"E\"\nbrief = '''\n" + valid() + "'''\nowner = \"x\"\n\n" + good, `unknown key "owner" in [parent]`},
		"unknown task":          {header + strings.Replace(good, "after = []", "after = []\nowner = \"x\"", 1), `task "a": unknown key "owner"`},
		"unknown inline task":   {header + "tasks = [{ key = \"a\", title = \"A\", brief = " + strconv.Quote(valid()) + ", extra = true }]\n", `task "a": unknown key "extra"`},
		"unknown inline parent": {header + "parent = { title = \"E\", brief = " + strconv.Quote(valid()) + ", owner = \"x\" }\n" + good, `unknown key "owner" in [parent]`},
		"no tasks":              {header, "at least one [[tasks]]"},
		"empty key":             {header + task("", ""), "key is required"},
		"multi-line key":        {header + strings.Replace(good, `key = "a"`, `key = "a\nb"`, 1), "key must be a single line"},
		"duplicate key":         {header + good + good, `task "a": duplicate key`},
		"id-shaped key":         {header + task("0123abcd", ""), `task "0123abcd": key must not look like a task id`},
		"empty title":           {header + strings.Replace(good, `title = "Do a"`, `title = ""`, 1), `task "a": title is required`},
		"multi-line title":      {header + strings.Replace(good, `title = "Do a"`, `title = "Do\na"`, 1), `task "a": title must be a single line`},
		"omitted brief":         {header + withBrief(good, ""), `task "a": brief must not be blank`},
		"blank brief":           {header + withBrief(good, `brief = " \t\n"`), `task "a": brief must not be blank`},
		"unicode blank brief":   {header + withBrief(good, `brief = "　  "`), `task "a": brief must not be blank`},
		"escaped NUL brief":     {header + withBrief(good, `brief = "a\u0000b"`), `task "a": brief must not contain NUL`},
		"parent title":          {header + "[parent]\ntitle = \"E\\nF\"\nbrief = '''\n" + valid() + "'''\n\n" + good, "parent: title must be a single line"},
		"parent blank brief":    {header + "[parent]\ntitle = \"E\"\nbrief = \"\\u2003\"\n\n" + good, "parent: brief must not be blank"},
		"parent omitted brief":  {header + "[parent]\ntitle = \"E\"\n\n" + good, "parent: brief must not be blank"},
		"parent NUL brief":      {header + "[parent]\ntitle = \"E\"\nbrief = \"\\u0000\"\n\n" + good, "parent: brief must not contain NUL"},
		"unknown after":         {header + task("a", `"zzz"`), `task "a": after names unknown key "zzz"`},
		"self cycle":            {header + task("a", `"a"`), "cycle: a → a"},
		"cycle":                 {header + task("a", `"b"`) + task("b", `"a"`), "cycle: a → b → a"},
		"cycle behind chain":    {header + task("x", "") + task("a", `"x", "c"`) + task("b", `"a"`) + task("c", `"b"`), "cycle: a → c → b → a"},
	} {
		_, err := Decode([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", label, err, tc.want)
		}
	}
}

func TestOrderDiamond(t *testing.T) {
	// d waits on b and c, which both wait on a; listed dependents first.
	src := header + task("d", `"b", "c"`) + task("c", `"a"`) + task("b", `"a", "0123abcd"`) + task("a", "")
	p, err := Decode([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	order, err := p.Order()
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 4 {
		t.Fatalf("order = %+v", order)
	}
	pos := map[string]int{}
	for i, task := range order {
		pos[task.Key] = i
	}
	for _, task := range order {
		for _, dep := range task.After {
			if i, ok := pos[dep]; ok && i >= pos[task.Key] {
				t.Errorf("%s precedes its prerequisite %s in %v", task.Key, dep, pos)
			}
		}
	}
}

func TestDecodeOrderedReturnsCheckedOrder(t *testing.T) {
	// Dependents listed first; a duplicate prerequisite and an existing id.
	src := header + task("d", `"b", "c", "b"`) + task("c", `"a"`) + task("b", `"a", "0123abcd"`) + task("a", "") + task("e", "")
	p, order, err := DecodeOrdered([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want, err := p.Order()
	if err != nil {
		t.Fatal(err)
	}
	keys := func(ts []Task) []string {
		var k []string
		for _, t := range ts {
			k = append(k, t.Key)
		}
		return k
	}
	if got := keys(order); !slices.Equal(got, keys(want)) || !slices.Equal(got, []string{"a", "b", "c", "d", "e"}) {
		t.Fatalf("order = %v, want %v", got, keys(want))
	}
	if _, _, err := DecodeOrdered([]byte(header + task("a", `"b"`) + task("b", `"a"`))); err == nil || !strings.Contains(err.Error(), "cycle: a → b → a") {
		t.Fatalf("cycle err = %v", err)
	}
}

func TestOrderReportsCycle(t *testing.T) {
	p := Proposal{Tasks: []Task{{Key: "a", After: []string{"b"}}, {Key: "b", After: []string{"a"}}}}
	if _, err := p.Order(); err == nil || !strings.Contains(err.Error(), "a → b → a") {
		t.Fatalf("err = %v", err)
	}
}

func TestSkeleton(t *testing.T) {
	s := Skeleton()
	if !strings.Contains(s, "[parent]") || !strings.Contains(s, "[[tasks]]") || !strings.Contains(s, brief.Skeleton()) {
		t.Fatalf("skeleton:\n%s", s)
	}
	// The unfilled skeleton is a valid proposal; its briefs are kept verbatim.
	p, err := Decode([]byte(s))
	if err != nil || p.Parent.Brief != brief.Skeleton() || len(p.Tasks) != 1 || p.Tasks[0].Brief != brief.Skeleton() {
		t.Fatalf("err = %v %+v", err, p)
	}
}

// withBrief replaces the valid brief in a task entry with line.
func withBrief(entry, line string) string {
	return strings.Replace(entry, "brief = '''\n"+valid()+"'''\n", line+"\n", 1)
}
