package spec

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// NextID returns the next sequential zero-padded ID (e.g. TASK-004) for the
// given prefix, scanning existing filenames in dir. Padding is 3 digits, or
// wider if an existing ID is wider.
func NextID(dir, prefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("%s-%03d", prefix, 1), nil
		}
		return "", err
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-(\d+)[-.]`)
	max, width := 0, 3
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
		if len(m[1]) > width {
			width = len(m[1])
		}
	}
	return fmt.Sprintf("%s-%0*d", prefix, width, max+1), nil
}

// Kebab lowercases s and replaces every run of non-alphanumeric characters
// with a single hyphen. Unicode letters and digits are preserved.
func Kebab(s string) string {
	var b strings.Builder
	prevHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
