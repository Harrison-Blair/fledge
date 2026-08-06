package main

import (
	"fmt"
	"regexp"
	"strings"
)

// semverCore matches a bare major.minor.patch version. A leading "v" belongs to
// git tags, not to the VERSION file, so the pattern rejects it.
var semverCore = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// normalizeVersion trims the raw VERSION file contents and validates that what
// remains is a bare semver core.
func normalizeVersion(raw string) (string, error) {
	version := strings.TrimSpace(raw)
	if version == "" {
		return "", fmt.Errorf("VERSION is empty")
	}
	if !semverCore.MatchString(version) {
		return "", fmt.Errorf("VERSION %q is not major.minor.patch", version)
	}
	return version, nil
}
