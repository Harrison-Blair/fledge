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

// profiles holds one entry per kind. Hooks follows Herdr's integration
// targets; the five installed harnesses carry verified evidence.
var profiles = map[string]Profile{
	"pi": {Kind: "pi", InterruptKeys: esc, Model: model, Discovery: DiscoveryCache, Resume: resume("--session"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensAndCost, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.pi/agent/models-store.json", Resume: "--session <path|id> (also --resume)", SessionRef: "Herdr hook reports path when known, else id",
		UsageTokens: "session file", UsageCost: "estimate from pi's own price table"}},
	"claude": {Kind: "claude", InterruptKeys: esc, Model: model, Discovery: DiscoveryCache, Resume: resume("--resume"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensOnly, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.claude/cache/model-catalog", Resume: "--resume <session-id>", SessionRef: "Herdr hook reports id and transcript path at SessionStart",
		UsageTokens: "transcript", UsageCost: "transcript records no cost"}},
	"codex": {Kind: "codex", InterruptKeys: esc, Model: short, Discovery: DiscoveryCache, Resume: resume("resume"), SessionRef: SessionRefIDOrPath, Hooks: true, Usage: UsageTokensOnly, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "cache: ~/.codex/models_cache.json", Resume: "codex resume <id>", SessionRef: "Herdr hook reports id and transcript path at SessionStart",
		UsageTokens: "rollout", UsageCost: "rollout records no cost"}},
	"gemini": {Kind: "gemini", InterruptKeys: esc, Model: short},
	"cursor": {Kind: "cursor", InterruptKeys: esc, Model: model, Discovery: DiscoveryCommand, Resume: resume("--resume"), SessionRef: SessionRefID, Hooks: true, Usage: UsageNone, Evidence: map[Capability]string{
		Interrupt: "esc", ModelDiscovery: "command: cursor-agent --list-models", Resume: "--resume <chatId>", SessionRef: "Herdr hook reports chat id",
		UsageTokens: "chat store is encrypted", UsageCost: "chat store is encrypted"}},
	"devin":      {Kind: "devin", InterruptKeys: esc, Model: model, Hooks: true},
	"agy":        {Kind: "agy", InterruptKeys: esc, Model: model, Hooks: true},
	"cline":      {Kind: "cline", InterruptKeys: esc, Model: short},
	"omp":        {Kind: "omp", InterruptKeys: esc, Model: model, Hooks: true},
	"mastracode": {Kind: "mastracode", InterruptKeys: ctrlC, Model: refused, Hooks: true},
	"opencode": {Kind: "opencode", InterruptKeys: escEsc, Model: short, Discovery: DiscoveryCommand, Resume: resume("--session"), SessionRef: SessionRefID, Hooks: true, Usage: UsageTokensAndCost, Evidence: map[Capability]string{
		Interrupt: "esc esc", ModelDiscovery: "command: opencode models", Resume: "--session <id> (also --continue, run -s <id>)", SessionRef: "Herdr plugin reports session id",
		UsageTokens: "opencode export", UsageCost: "harness-recorded; 0 on subscription"}},
	"copilot":  {Kind: "copilot", InterruptKeys: escEsc, Model: model, Hooks: true},
	"kimi":     {Kind: "kimi", InterruptKeys: esc, Model: short, Hooks: true},
	"kiro":     {Kind: "kiro", InterruptKeys: esc, Model: refused},
	"droid":    {Kind: "droid", InterruptKeys: ctrlC, Model: short, Hooks: true},
	"amp":      {Kind: "amp", InterruptKeys: escEsc, Model: refused},
	"grok":     {Kind: "grok", InterruptKeys: ctrlC, Model: short, Hooks: true},
	"hermes":   {Kind: "hermes", InterruptKeys: ctrlC, Model: ModelPolicy{Flag: "--model", ShortConflict: true, Prefix: []string{"chat"}}, Hooks: true},
	"kilo":     {Kind: "kilo", InterruptKeys: escEsc, Model: short, Hooks: true},
	"qodercli": {Kind: "qodercli", InterruptKeys: ctrlC, Model: refused, Hooks: true},
	"qwen":     {Kind: "qwen", InterruptKeys: esc, Model: short, Hooks: true},
	"letta":    {Kind: "letta", InterruptKeys: esc, Model: short, Hooks: true},
	"maki":     {Kind: "maki", InterruptKeys: esc, Model: short},
	"muse":     {Kind: "muse", InterruptKeys: esc, Model: refused},
}

// Lookup returns kind's profile; ok is false for an unknown kind. The
// returned slices are fresh copies.
func Lookup(kind string) (p Profile, ok bool) {
	p, ok = profiles[kind]
	p.InterruptKeys = slices.Clone(p.InterruptKeys)
	p.Model.Prefix = slices.Clone(p.Model.Prefix)
	return p, ok
}
