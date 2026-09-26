// Package usage reads a harness session's token usage and harness-recorded
// cost from the harness's own store, filtered to a time window. Tokens are
// measured; cost appears only when the harness records it and is labelled an
// estimate; Fledge keeps no price table. Missing or unreadable data yields
// Basis unavailable with a Reason, never an error. Windows filter responses
// by timestamp on one session, so an agent working two tasks at once, or
// chatting with a human in the same pane, double-counts across windows.
// Input excludes cache reads and writes; codex counts reasoning inside
// Output, while opencode reports it separately.
//
// usage.go holds the types, Count, Locate, Read, and the shared tally;
// claude.go reads transcripts and sub-agent transcripts, deduplicated by requestId;
// codex.go reads rollouts' token_usage_record entries, falling back to token_count totals;
// pi.go reads session messages and pi's price-table cost;
// opencode.go runs opencode export and never opens opencode's database.
package usage
