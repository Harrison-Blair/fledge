package harness

import "strings"

// Capability names one row of the fixed capability set.
type Capability string

const (
	Interrupt      Capability = "interrupt"
	ModelSelect    Capability = "model_select"
	ModelDiscovery Capability = "model_discovery"
	Resume         Capability = "resume"
	SessionRef     Capability = "session_ref"
	LifecycleHooks Capability = "lifecycle_hooks"
	UsageTokens    Capability = "usage_tokens"
	UsageCost      Capability = "usage_cost"
)

// Level is who provides a capability: Fledge, the harness natively, nobody, or not checked.
type Level string

const (
	Fledge  Level = "fledge"
	Native  Level = "native"
	None    Level = "none"
	Unknown Level = "unknown"
)

// Row is one capability's level with a one-line evidence note.
type Row struct {
	Name     Capability `json:"name"`
	Level    Level      `json:"level"`
	Evidence string     `json:"evidence"`
}

// Capabilities derives kind's fixed capability rows from its profile's typed
// fields, so the report cannot drift from behavior. A profile's Evidence
// replaces the derived note. An unknown kind has no rows.
func Capabilities(kind string) []Row {
	p, ok := Lookup(kind)
	if !ok {
		return nil
	}
	rows := []Row{
		{Interrupt, Unknown, "no binding"},
		{ModelSelect, None, "--model refused; pass native arguments"},
		{ModelDiscovery, None, "no local model source"},
		{Resume, Unknown, "unknown"},
		{SessionRef, None, "no Herdr integration target"},
		{LifecycleHooks, None, "no Herdr integration target"},
		{UsageTokens, Unknown, "unknown"},
		{UsageCost, Unknown, "unknown"},
	}
	set := func(i int, level Level, note string) { rows[i].Level, rows[i].Evidence = level, note }
	if p.InterruptKeys != nil {
		set(0, Fledge, "default binding, unverified: "+strings.Join(p.InterruptKeys, " "))
	}
	if p.Model.Flag != "" {
		set(1, Fledge, "spawn passes "+strings.Join(append(p.Model.Prefix, p.Model.Flag), " "))
	}
	switch p.Discovery {
	case DiscoveryCache:
		set(2, Fledge, "agent models reads a local cache")
	case DiscoveryCommand:
		set(2, Fledge, "agent models runs a harness command")
	}
	if p.Resume != nil {
		set(3, Native, "native resume option")
	}
	if p.Hooks {
		set(4, Native, "Herdr integration target exists; reported ref unknown")
		set(5, Native, "Herdr integration target exists")
	}
	if p.SessionRef != SessionRefNone {
		set(4, Fledge, "Herdr hook reports it")
	}
	switch p.Usage {
	case UsageNone:
		set(6, None, "harness records none")
		set(7, None, "harness records none")
	case UsageTokensOnly:
		set(6, Fledge, "harness-recorded tokens")
		set(7, None, "harness records none")
	case UsageTokensAndCost:
		set(6, Fledge, "harness-recorded tokens")
		set(7, Fledge, "harness-recorded estimate")
	}
	for i := range rows {
		if note := p.Evidence[rows[i].Name]; note != "" {
			rows[i].Evidence = note
		}
	}
	return rows
}
