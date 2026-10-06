package harness

import "slices"

// ModelPolicy is how spawn passes --model. An empty Flag means spawn refuses
// --model; ShortConflict treats a native -m as a conflict; Prefix precedes the flag.
type ModelPolicy struct {
	Flag          string
	ShortConflict bool
	Prefix        []string
}

// DiscoveryKind is how agent models finds a harness's models.
type DiscoveryKind int

const (
	DiscoveryNone    DiscoveryKind = iota // no local model source
	DiscoveryCache                        // reads local cache files only
	DiscoveryCommand                      // runs a harness command
)

// ResumeTemplate builds the native arguments that resume a session ref.
type ResumeTemplate func(ref string) []string

// SessionRefKind is what the Herdr hook reports as the native session ref.
type SessionRefKind int

const (
	SessionRefNone SessionRefKind = iota
	SessionRefID
	SessionRefPath
	SessionRefIDOrPath
)

// UsageKind is what usage data the harness records where Fledge can read it.
type UsageKind int

const (
	UsageUnknown UsageKind = iota // not checked
	UsageNone
	UsageTokensOnly
	UsageTokensAndCost
)

// Profile holds one harness kind's typed behavior. A nil InterruptKeys or
// Resume means unknown. Evidence overrides the derived note for a capability.
type Profile struct {
	Kind          string
	InterruptKeys []string
	Model         ModelPolicy
	Discovery     DiscoveryKind
	Resume        ResumeTemplate
	SessionRef    SessionRefKind
	Hooks         bool
	Usage         UsageKind
	Evidence      map[Capability]string
}

var (
	esc     = []string{"esc"}
	escEsc  = []string{"esc", "esc"}
	ctrlC   = []string{"ctrl+c"}
	model   = ModelPolicy{Flag: "--model"}
	short   = ModelPolicy{Flag: "--model", ShortConflict: true}
	refused = ModelPolicy{}
)

func resume(args ...string) ResumeTemplate {
	return func(ref string) []string { return append(slices.Clone(args), ref) }
}

// profiles holds one entry per documented Herdr harness kind, in the order
// Kinds returns. Hooks follows Herdr's integration targets; the five installed
// harnesses carry verified evidence.
var profiles = []Profile{
	{Kind: "pi", InterruptKeys: esc, Model: model, Discovery: DiscoveryCache, Resume: resume("--session"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensAndCost, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.pi/agent/models-store.json", Resume: "--session <path|id> (also --resume)", SessionRef: "Herdr hook reports path when known, else id",
		UsageTokens: "session file", UsageCost: "estimate from pi's own price table"}},
	{Kind: "claude", InterruptKeys: esc, Model: model, Discovery: DiscoveryCache, Resume: resume("--resume"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensOnly, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.claude/cache/model-catalog", Resume: "--resume <session-id>", SessionRef: "Herdr hook reports id and transcript path at SessionStart",
		UsageTokens: "transcript", UsageCost: "transcript records no cost"}},
	{Kind: "codex", InterruptKeys: esc, Model: short, Discovery: DiscoveryCache, Resume: resume("resume"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensOnly, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.codex/models_cache.json", Resume: "codex resume <id>", SessionRef: "Herdr hook reports id and transcript path at SessionStart",
		UsageTokens: "rollout", UsageCost: "rollout records no cost"}},
	{Kind: "gemini", InterruptKeys: esc, Model: short},
	{Kind: "cursor", InterruptKeys: esc, Model: model, Discovery: DiscoveryCommand, Resume: resume("--resume"), SessionRef: SessionRefID, Hooks: true, Usage: UsageNone, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "command: cursor-agent --list-models", Resume: "--resume <chatId>", SessionRef: "Herdr hook reports chat id",
		UsageTokens: "chat store is encrypted", UsageCost: "chat store is encrypted"}},
	{Kind: "devin", InterruptKeys: esc, Model: model, Hooks: true},
	{Kind: "agy", InterruptKeys: esc, Model: model, Hooks: true},
	{Kind: "cline", InterruptKeys: esc, Model: short},
	{Kind: "omp", InterruptKeys: esc, Model: model, Hooks: true},
	{Kind: "mastracode", InterruptKeys: ctrlC, Model: refused, Hooks: true},
	{Kind: "opencode", InterruptKeys: escEsc, Model: short, Discovery: DiscoveryCommand, Resume: resume("--session"), SessionRef: SessionRefID, Hooks: true, Usage: UsageTokensAndCost, Evidence: map[Capability]string{
		Interrupt: "esc esc", ModelDiscovery: "command: opencode models", Resume: "--session <id> (also --continue, run -s <id>)", SessionRef: "Herdr plugin reports session id",
		UsageTokens: "opencode export", UsageCost: "harness-recorded; 0 on subscription"}},
	{Kind: "copilot", InterruptKeys: escEsc, Model: model, Hooks: true},
	{Kind: "kimi", InterruptKeys: esc, Model: short, Hooks: true},
	{Kind: "kiro", InterruptKeys: esc, Model: refused},
	{Kind: "droid", InterruptKeys: ctrlC, Model: short, Hooks: true},
	{Kind: "amp", InterruptKeys: escEsc, Model: refused},
	{Kind: "grok", InterruptKeys: ctrlC, Model: short, Hooks: true},
	{Kind: "hermes", InterruptKeys: ctrlC, Model: ModelPolicy{Flag: "--model", ShortConflict: true, Prefix: []string{"chat"}}, Hooks: true},
	{Kind: "kilo", InterruptKeys: escEsc, Model: short, Hooks: true},
	{Kind: "qodercli", InterruptKeys: ctrlC, Model: refused, Hooks: true},
	{Kind: "qwen", InterruptKeys: esc, Model: short, Hooks: true},
	{Kind: "letta", InterruptKeys: esc, Model: short, Hooks: true},
	{Kind: "maki", InterruptKeys: esc, Model: short},
	{Kind: "muse", InterruptKeys: esc, Model: refused},
}

// Lookup returns kind's profile; ok is false for an unknown kind. The
// returned slices are fresh copies.
func Lookup(kind string) (p Profile, ok bool) {
	i := slices.IndexFunc(profiles, func(p Profile) bool { return p.Kind == kind })
	if i < 0 {
		return Profile{}, false
	}
	p = profiles[i]
	p.InterruptKeys = slices.Clone(p.InterruptKeys)
	p.Model.Prefix = slices.Clone(p.Model.Prefix)
	return p, true
}
