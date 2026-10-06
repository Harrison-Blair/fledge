package memory

import (
	"errors"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/cli"
)

func valid() Memory {
	return Memory{Name: "herdr-socket", Description: "Herdr commands need socket access", Type: "project", Body: "Every agent command connects to the socket.\n"}
}

func TestFormatWritesFrontmatterThenBody(t *testing.T) {
	want := "---\nname: herdr-socket\ndescription: Herdr commands need socket access\ntype: project\n---\n\nEvery agent command connects to the socket.\n"
	if got := string(Format(valid())); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	m := valid()
	m.Body = "no trailing newline"
	if got := string(Format(m)); !strings.HasSuffix(got, "---\n\nno trailing newline\n") {
		t.Fatalf("%q", got)
	}
}

func TestParseRoundTripsFormat(t *testing.T) {
	m, err := Parse("herdr-socket", Format(valid()))
	if err != nil || m != valid() {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestParseRejectsMalformedFiles(t *testing.T) {
	good := string(Format(valid()))
	for name, data := range map[string]string{
		"no frontmatter":    "Every agent command.\n",
		"unterminated":      "---\nname: herdr-socket\n",
		"missing type":      strings.Replace(good, "type: project\n", "", 1),
		"duplicate key":     strings.Replace(good, "type: project\n", "type: project\ntype: user\n", 1),
		"unknown key":       strings.Replace(good, "type: project\n", "type: project\nowner: x\n", 1),
		"name mismatch":     strings.Replace(good, "name: herdr-socket", "name: other", 1),
		"invalid type":      strings.Replace(good, "type: project", "type: note", 1),
		"empty body":        strings.SplitAfter(good, "---\n\n")[0],
		"no blank after fm": strings.Replace(good, "---\n\n", "---\n", 1),
	} {
		if _, err := Parse("herdr-socket", []byte(data)); err == nil {
			t.Errorf("%s: accepted %q", name, data)
		}
	}
}

func TestValidateRejectsEachBadField(t *testing.T) {
	if err := Validate(valid()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Memory){
		"empty name":           func(m *Memory) { m.Name = "" },
		"upper name":           func(m *Memory) { m.Name = "Herdr" },
		"leading dash":         func(m *Memory) { m.Name = "-herdr" },
		"double dash":          func(m *Memory) { m.Name = "herdr--socket" },
		"path name":            func(m *Memory) { m.Name = "../herdr" },
		"long name":            func(m *Memory) { m.Name = strings.Repeat("a", 65) },
		"empty description":    func(m *Memory) { m.Description = "" },
		"multiline":            func(m *Memory) { m.Description = "a\nb" },
		"padded description":   func(m *Memory) { m.Description = " a" },
		"unknown type":         func(m *Memory) { m.Type = "note" },
		"blank body":           func(m *Memory) { m.Body = " \n\t" },
		"invalid utf-8 body":   func(m *Memory) { m.Body = "\xff" },
		"frontmatter-like key": func(m *Memory) { m.Description = "x\r" },
	} {
		m := valid()
		mutate(&m)
		var input *cli.InputError
		if err := Validate(m); !errors.As(err, &input) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestIndexIsOneLinePerMemoryByName(t *testing.T) {
	b := valid()
	a := Memory{Name: "alpha", Description: "First fact", Type: "user", Body: "x"}
	want := "- [alpha](alpha.md) — First fact\n- [herdr-socket](herdr-socket.md) — Herdr commands need socket access\n"
	if got := Index([]Memory{b, a}); got != want {
		t.Fatalf("%q\nwant %q", got, want)
	}
	if got := Index(nil); got != "" {
		t.Fatalf("%q", got)
	}
}
