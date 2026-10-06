package selector

import "github.com/spf13/pflag"

// BindFlags registers the filter flags on fs and stores their values in f.
func BindFlags(fs *pflag.FlagSet, f *Filter) {
	fs.BoolVar(&f.Mine, "mine", false, "Only agents whose parent is the caller's own record")
	fs.StringVar(&f.Parent, "parent", "", "Only agents whose parent is this Fledge agent record ID")
	fs.StringArrayVar(&f.States, "state", nil, "Only agents in this state: idle, working, blocked, done, or unknown (repeatable)")
	fs.StringArrayVar(&f.Harnesses, "harness", nil, "Only agents running this harness (repeatable)")
	fs.StringArrayVar(&f.Profiles, "profile", nil, "Only agents spawned with this profile (repeatable)")
	fs.StringArrayVar(&f.Tasks, "task", nil, "Only the owner of this task ID (repeatable)")
	fs.StringArrayVar(&f.Worktrees, "worktree", nil, "Only agents whose recorded worktree is this path (repeatable)")
	fs.BoolVar(&f.Registered, "registered", false, "Only agents with a live record in this repository")
}
