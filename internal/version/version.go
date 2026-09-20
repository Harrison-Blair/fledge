// Package version reports the release or development build version.
package version

import "runtime/debug"

// release is set by release builds using -ldflags -X. Local builds use Go's
// embedded module version, including its revision and dirty marker.
var release string

// Version returns the best available build version without consulting Git or
// the runtime working directory.
func Version() string {
	info, _ := debug.ReadBuildInfo()
	return resolve(release, info)
}

func resolve(release string, info *debug.BuildInfo) string {
	if release != "" {
		return release
	}
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
