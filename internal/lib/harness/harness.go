package harness

// Kinds returns the documented Herdr harness kinds, in registry order, as a
// fresh slice.
func Kinds() []string {
	kinds := make([]string, len(profiles))
	for i, p := range profiles {
		kinds[i] = p.Kind
	}
	return kinds
}

// IsKind reports whether kind is a documented Herdr harness kind.
func IsKind(kind string) bool {
	_, ok := Lookup(kind)
	return ok
}
