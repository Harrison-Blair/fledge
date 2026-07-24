package flock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/scaffold"
)

func TestValidate(t *testing.T) {
	valid := []string{"a", "flock1", "test", "abc123", strings.Repeat("a", MaxName)}
	for _, name := range valid {
		if err := Validate(name); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",
		"Flock",    // uppercase
		"my-flock", // dash
		"my_flock", // underscore
		"my flock", // space
		"my/flock", // path separator
		"..",       // parent directory
		".",        // current directory
		"flock.1",  // dot
		strings.Repeat("a", MaxName+1),
	}
	for _, name := range invalid {
		if err := Validate(name); err == nil {
			t.Errorf("Validate(%q) = nil, want an error", name)
		}
	}
}

// scratch makes a scaffolded workspace with the named flocks already present.
func scratch(t *testing.T, flocks ...string) string {
	t.Helper()
	root := t.TempDir()
	if _, err := scaffold.Ensure(root); err != nil {
		t.Fatal(err)
	}
	for _, f := range flocks {
		if err := os.MkdirAll(Dir(root, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestListReportsFlocksSorted(t *testing.T) {
	root := scratch(t, "zulu", "alpha", "mike")

	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "mike", "zulu"}
	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	}
}

func TestListOnWorkspaceWithNoFlocks(t *testing.T) {
	got, err := List(scratch(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("List = %v, want empty", got)
	}
}

// A workspace that was never scaffolded has no flocks, not an error: the
// status overview runs before init.
func TestListOnMissingWorkspace(t *testing.T) {
	got, err := List(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("List = %v, want nil error", err)
	}
	if len(got) != 0 {
		t.Fatalf("List = %v, want empty", got)
	}
}

func TestMintTakesLowestFreeName(t *testing.T) {
	cases := []struct {
		existing []string
		want     string
	}{
		{nil, "flock1"},
		{[]string{"flock1"}, "flock2"},
		{[]string{"flock1", "flock2"}, "flock3"},
		{[]string{"flock2"}, "flock1"},           // fills the gap
		{[]string{"flock1", "flock3"}, "flock2"}, // fills the gap
		{[]string{"named"}, "flock1"},            // named flocks do not shift the sequence
	}
	for _, c := range cases {
		got, err := Mint(scratch(t, c.existing...))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Mint(%v) = %q, want %q", c.existing, got, c.want)
		}
	}
}

// A minted name is always fresh, so auto-start never resumes a stale journal.
func TestMintNeverReturnsAnExistingFlock(t *testing.T) {
	root := scratch(t, "flock1", "flock2", "flock3")
	got, err := Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Dir(root, got)); !os.IsNotExist(err) {
		t.Fatalf("Mint returned %q, which already has state", got)
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(Env, "alpha")
	got, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if got != "alpha" {
		t.Fatalf("FromEnv = %q, want alpha", got)
	}
}

func TestFromEnvUnsetIsHardError(t *testing.T) {
	t.Setenv(Env, "")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("FromEnv with no flock set returned nil error")
	}
	if !strings.Contains(err.Error(), "run inside a flock session") {
		t.Fatalf("FromEnv error = %q, want the operator instruction", err)
	}
}

// A malformed environment value must not become a path segment.
func TestFromEnvRejectsBadName(t *testing.T) {
	t.Setenv(Env, "../escape")
	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv accepted a traversing flock name")
	}
}

// A flock's session and window title must both announce fledge, so an
// operator can tell a managed session from a hand-started one.
func TestSessionNameAndWindowTitleAreBranded(t *testing.T) {
	got := SessionName(t.TempDir(), "flock1")
	if !strings.HasPrefix(got, "fledge-") || !strings.HasSuffix(got, "-flock1") {
		t.Errorf("SessionName = %q, want fledge-…-flock1", got)
	}
	if got := WindowTitle("flock1"); got != "fledge · flock1" {
		t.Errorf("WindowTitle = %q, want %q", got, "fledge · flock1")
	}
}

// Herdr's session namespace is global to the server, so two workspaces
// running the same-named flock must derive different sessions: a shared name
// is how one project's flock stop used to tear the other project down.
func TestSessionNameDiffersPerWorkspace(t *testing.T) {
	a := SessionName(t.TempDir(), "flock1")
	b := SessionName(t.TempDir(), "flock1")
	if a == b {
		t.Errorf("two workspaces derived the same session %q", a)
	}
}

// The session name keys off the workspace's identity, not the spelling of the
// path used to reach it, or a daemon and a client could disagree.
func TestSessionNameStableAcrossRootSpellings(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	spelled := filepath.Join(sub, "..")
	if got, want := SessionName(spelled, "flock1"), SessionName(root, "flock1"); got != want {
		t.Errorf("SessionName(%q) = %q, want %q", spelled, got, want)
	}
}
