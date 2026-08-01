// Package processenv defines environment policy for processes managed by
// Fledge.
package processenv

import "strings"

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
