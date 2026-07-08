package nest

import "strings"

// ConcernDocs is the ordered closed set of known concern document names.
var ConcernDocs = []string{
	"architecture", "modules", "conventions", "data-model",
	"dependencies", "entry-points", "testing", "domain", "index",
}

// IsKnownDoc reports whether name is in ConcernDocs.
func IsKnownDoc(name string) bool {
	for _, d := range ConcernDocs {
		if d == name {
			return true
		}
	}
	return false
}

// Title returns the display title for a known concern doc name.
// Hyphens are replaced with spaces and the result is title-cased.
func Title(name string) string {
	words := strings.Split(strings.ReplaceAll(name, "-", " "), " ")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
