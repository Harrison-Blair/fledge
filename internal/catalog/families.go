package catalog

import (
	"slices"
	"strings"
)

// families lists model family names in priority order. A family's index in
// this slice IS its rank: lower index = higher priority. Any ID that matches
// no family ranks as len(families), i.e. below every listed family.
var families = []string{"sol", "terra", "luna", "fable", "opus", "sonnet", "haiku"}

// familySeparators are the byte separators used to split a model ID into the
// tokens that family matching tests. Dropping '_' or ':' here would silently
// change ordering for IDs that use them (see order_test.go case "underscore/colon").
const familySeparators = "-._/:"

// isFamilySeparator reports whether r is one of familySeparators.
func isFamilySeparator(r rune) bool {
	return strings.ContainsRune(familySeparators, r)
}

// familyRank reports id's family rank: the index in families of the
// highest-priority family whose name equals one whole token of id (id is
// lowercased and split on familySeparators), or len(families) when no token
// matches any family.
func familyRank(id string) int {
	best := len(families)
	for _, tok := range strings.FieldsFunc(strings.ToLower(id), isFamilySeparator) {
		if i := slices.Index(families, tok); i >= 0 && i < best {
			best = i
		}
	}
	return best
}
