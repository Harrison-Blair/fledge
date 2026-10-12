package spawn

import "strings"

// daemonArguments starts Codex without its shared app-server daemon, which runs
// tool commands with the environment of the pane that first started it.
func daemonArguments(kind string, args []string) []string {
	if kind != "codex" {
		return args
	}
	for i := 0; i < len(args); i++ {
		flag, _, attached := strings.Cut(args[i], "=")
		switch flag {
		case "--":
			return append([]string{"--no-daemon"}, args...)
		case "--no-daemon":
			return args
		case "--model", "-m", "--profile", "-p":
			if !attached {
				i++
			}
		default:
			_, consumed := codexConfigKey(args, i)
			i += consumed
		}
	}
	return append([]string{"--no-daemon"}, args...)
}
