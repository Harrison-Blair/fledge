package cli

// Pointer returns nil for an empty string, so optional JSON fields encode null.
func Pointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Display renders an optional value for humans, showing "-" when it is absent.
func Display(s *string) string {
	if s == nil {
		return "-"
	}
	return DisplayString(*s)
}

// DisplayString renders a value for humans, showing "-" when it is empty.
func DisplayString(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
