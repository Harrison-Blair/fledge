package record

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const (
	namePrefix       = "fledge-"
	randomByteCount  = 4
	MaxSessionLength = 64
	MinSessionLength = len(namePrefix) + 1 + 1 + randomByteCount*2
)

// Slug converts a project name to the filesystem-safe character set accepted
// by Herder session names.
func Slug(projectName string) string {
	var slug strings.Builder
	invalid := false

	for _, r := range projectName {
		if validNameRune(r) {
			slug.WriteRune(r)
			invalid = false
			continue
		}
		if !invalid {
			slug.WriteByte('-')
			invalid = true
		}
	}

	value := strings.Trim(slug.String(), "._-")
	if value == "" {
		return "project"
	}
	return value
}

// GenerateName creates a collision-free Fledge Herder session name. Names in
// unavailable include both local records and all sessions known to Herder.
func GenerateName(projectName string, maxNameLength int, unavailable map[string]struct{}, entropy io.Reader) (string, error) {
	if maxNameLength < MinSessionLength {
		return "", fmt.Errorf("generate session name: maximum length %d is too short", maxNameLength)
	}
	if maxNameLength > MaxSessionLength {
		maxNameLength = MaxSessionLength
	}
	if entropy == nil {
		return "", fmt.Errorf("generate session name: entropy reader is nil")
	}

	slug := Slug(projectName)
	maxSlugLength := maxNameLength - len(namePrefix) - 1 - randomByteCount*2
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "._-")
		if slug == "" {
			slug = "project"
		}
	}

	for {
		random := make([]byte, randomByteCount)
		if _, err := io.ReadFull(entropy, random); err != nil {
			return "", fmt.Errorf("generate session name: read entropy: %w", err)
		}

		name := namePrefix + slug + "-" + hex.EncodeToString(random)
		if _, exists := unavailable[name]; !exists {
			return name, nil
		}
	}
}

func validNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '.' || r == '_' || r == '-'
}

func validHerderName(name string) bool {
	if len(name) == 0 || len(name) > MaxSessionLength {
		return false
	}
	for _, r := range name {
		if !validNameRune(r) {
			return false
		}
	}
	return true
}
