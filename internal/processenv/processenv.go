// Package processenv defines environment policy for processes managed by
// Fledge.
package processenv

import "strings"

// Managed returns an independent environment for a Fledge-managed process.
// It removes NO_COLOR and replaces any inherited TMPDIR with tempDir while
// preserving every unrelated entry. An empty tempDir leaves TMPDIR unchanged.
func Managed(environ []string, tempDir string) []string {
	if tempDir == "" {
		return WithoutNoColor(environ)
	}
	managed := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if name != "NO_COLOR" && name != "TMPDIR" {
			managed = append(managed, entry)
		}
	}
	return append(managed, "TMPDIR="+tempDir)
}

// WithoutNoColor returns a copy of environ with exactly the NO_COLOR variable
// removed. The input slice and all unrelated entries are left unchanged.
func WithoutNoColor(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if name != "NO_COLOR" {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
