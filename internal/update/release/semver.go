package release

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a release version of the shape Fledge's tags actually take:
// vMAJOR.MINOR.PATCH, with no pre-release or build metadata. Nothing else is
// ever published, so nothing else is accepted.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion parses a "vX.Y.Z" tag. Anything else — "dev", a bare "1.2.3",
// a pre-release suffix, leading or trailing space — is an error, which is
// what makes an unknown current version detectable rather than silently
// comparable.
func ParseVersion(s string) (Version, error) {
	invalid := fmt.Errorf("update: %q is not a vX.Y.Z version", s)
	rest, ok := strings.CutPrefix(s, "v")
	if !ok {
		return Version{}, invalid
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 {
		return Version{}, invalid
	}
	var v Version
	for i, p := range parts {
		// strconv.Atoi accepts a leading sign and Go's parser accepts
		// underscores in some forms; require plain digits so "v-1.2.3"
		// and friends are rejected. Atoi still rejects a component that
		// exceeds int.
		if p == "" || (len(p) > 1 && p[0] == '0') || strings.TrimFunc(p, func(r rune) bool { return r >= '0' && r <= '9' }) != "" {
			return Version{}, invalid
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, invalid
		}
		switch i {
		case 0:
			v.Major = n
		case 1:
			v.Minor = n
		case 2:
			v.Patch = n
		}
	}
	return v, nil
}

// CompareVersions reports whether a is older (-1), the same as (0) or newer
// (+1) than b. Either side failing to parse is an error: the caller then
// treats the comparison as unknown rather than guessing.
func CompareVersions(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]int{
		{va.Major, vb.Major},
		{va.Minor, vb.Minor},
		{va.Patch, vb.Patch},
	} {
		switch {
		case pair[0] < pair[1]:
			return -1, nil
		case pair[0] > pair[1]:
			return 1, nil
		}
	}
	return 0, nil
}
