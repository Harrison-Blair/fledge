package selector

import (
	"reflect"
	"testing"

	"github.com/spf13/pflag"
)

func TestBindFlagsParsesEveryField(t *testing.T) {
	var f Filter
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	BindFlags(fs, &f)
	err := fs.Parse([]string{"--mine", "--parent", "0000beef", "--state", "idle", "--state", "done", "--harness", "pi",
		"--profile", "p", "--task", "0000cafe", "--worktree", "/w", "--worktree", "/x", "--registered"})
	if err != nil {
		t.Fatal(err)
	}
	want := Filter{Mine: true, Parent: "0000beef", States: []string{"idle", "done"}, Harnesses: []string{"pi"},
		Profiles: []string{"p"}, Tasks: []string{"0000cafe"}, Worktrees: []string{"/w", "/x"}, Registered: true}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v, want %+v", f, want)
	}
}

func TestBindFlagsDefaultsAndHelp(t *testing.T) {
	var f Filter
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	BindFlags(fs, &f)
	if err := fs.Parse(nil); err != nil || !reflect.DeepEqual(f, Filter{}) {
		t.Fatalf("%v %+v", err, f)
	}
	for name, usage := range map[string]string{
		"mine":       "Only agents whose parent is the caller's own record",
		"parent":     "Only agents whose parent is this Fledge agent record ID",
		"state":      "Only agents in this state: idle, working, blocked, done, or unknown (repeatable)",
		"harness":    "Only agents running this harness (repeatable)",
		"profile":    "Only agents spawned with this profile (repeatable)",
		"task":       "Only the owner of this task ID (repeatable)",
		"worktree":   "Only agents whose recorded worktree is this path (repeatable)",
		"registered": "Only agents with a live record in this repository",
	} {
		if flag := fs.Lookup(name); flag == nil || flag.Usage != usage {
			t.Errorf("--%s: %+v", name, flag)
		}
	}
}
