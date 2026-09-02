package record

import (
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	namePrefix = "fledge-"
	// timestampLayout formats the fresh-session UTC timestamp with filesystem-
	// and Herder-safe separators (dots, not colons). It always renders 20 bytes.
	timestampLayout  = "2006-01-02T15.04.05Z"
	randomByteCount  = 4
	MaxSessionLength = 64
	// fixedNameCost counts every byte of a generated name except the slug: the
	// prefix, the timestamp, the two "-" separators, and the hex suffix (37).
	fixedNameCost = len(namePrefix) + len(timestampLayout) + 1 + 1 + randomByteCount*2
	// MinSessionLength admits the fixed cost plus a one-byte slug (38).
	MinSessionLength = fixedNameCost + 1
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

// GenerateName creates a collision-free Fledge Herder session name of the shape
// fledge-<UTC timestamp>-<slug>-<random hex>. createdAt supplies the timestamp
// and must already be normalized to UTC by the caller; placing it before the
// variable slug lets a name-ascending listing order fresh sessions by creation
// time. Names in unavailable include both local records and all sessions known
// to Herder.
func GenerateName(projectName string, createdAt time.Time, maxNameLength int, unavailable map[string]struct{}, entropy io.Reader) (string, error) {
	if maxNameLength < MinSessionLength {
		return "", fmt.Errorf("generate session name: maximum length %d is too short", maxNameLength)
	}
	if maxNameLength > MaxSessionLength {
		maxNameLength = MaxSessionLength
	}
	if entropy == nil {
		return "", fmt.Errorf("generate session name: entropy reader is nil")
	}

	timestamp := createdAt.Format(timestampLayout)
	slug := Slug(projectName)
	maxSlugLength := maxNameLength - fixedNameCost
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

		name := namePrefix + timestamp + "-" + slug + "-" + hex.EncodeToString(random)
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
