package buildinfo

import (
	_ "embed"
	"runtime"
	"runtime/debug"
	"strings"
)

// version is the single authoritative Fledge release version.
//
//go:embed VERSION
var version string

// Info describes the executable that is currently running.
type Info struct {
	Version     string `json:"version"`
	Revision    string `json:"revision,omitempty"`
	Modified    bool   `json:"modified"`
	GoVersion   string `json:"go_version"`
	Development bool   `json:"development"`
}

// Current returns embedded release and Go VCS build metadata.
func Current() Info {
	out := Info{
		Version:     strings.TrimSpace(version),
		GoVersion:   runtime.Version(),
		Development: true,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range bi.Settings {
			switch setting.Key {
			case "vcs.revision":
				out.Revision = setting.Value
			case "vcs.modified":
				out.Modified = setting.Value == "true"
			}
		}
	}
	out.Development = out.Revision == "" || out.Modified
	return out
}

// Version returns the authoritative embedded release version.
func Version() string { return strings.TrimSpace(version) }
