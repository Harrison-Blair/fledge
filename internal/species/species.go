// Package species assigns penguin-species slugs to registered agents.
package species

import "fmt"

// Slugs lists the 18 extant penguin species as flattened lowercase slugs, in
// fixed order. Auto-assignment walks this order.
var Slugs = []string{
	"emperor",
	"king",
	"adelie",
	"chinstrap",
	"gentoo",
	"little",
	"african",
	"galapagos",
	"humboldt",
	"magellanic",
	"yelloweyed",
	"erectcrested",
	"fiordland",
	"macaroni",
	"northernrockhopper",
	"royal",
	"snares",
	"southernrockhopper",
}

// Pick returns the species slug to assign. If requested is non-empty it must be
// a known, un-taken slug. If requested is empty, Pick returns the first slug in
// Slugs order for which taken reports false.
func Pick(taken func(string) bool, requested string) (string, error) {
	if requested != "" {
		if !known(requested) {
			return "", fmt.Errorf("unknown species %q", requested)
		}
		if taken(requested) {
			return "", fmt.Errorf("species %q already taken", requested)
		}
		return requested, nil
	}

	for _, slug := range Slugs {
		if !taken(slug) {
			return slug, nil
		}
	}
	return "", fmt.Errorf("all %d species taken", len(Slugs))
}

func known(slug string) bool {
	for _, s := range Slugs {
		if s == slug {
			return true
		}
	}
	return false
}
