// Package spawn implements agent spawn: launching an agent in a Herdr pane.
// options.go validates flags and native model arguments; spawn.go runs startup, retries, and the first prompt;
// ready.go holds the first prompt until Herdr admits input from the started agent;
// profile.go applies a launch profile under explicit flags and places its role before the task prompt;
// placement.go resolves and creates the destination pane; worktree.go places agents in new or existing checkouts;
// identity.go registers the started agent's Fledge record;
// output.go renders results and startup recovery hints; picker.go prompts for options when spawn runs interactively with no flags.
package spawn
