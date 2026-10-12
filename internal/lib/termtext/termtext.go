// Package termtext removes terminal control sequences from dynamic text
// before human output writes it to a terminal.
package termtext

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Clean is the boundary for dynamic terminal text. Strip established escape
// sequences before filtering residual controls; preserve Unicode and lines.
func Clean(text string) string {
	// The ANSI parser accepts byte C1 sequences; normalize Unicode-encoded C1
	// introductions before stripping, without corrupting other UTF-8 text.
	text = strings.NewReplacer("\u009b", "\x1b[", "\u009d", "\x1b]", "\u0090", "\x1bP", "\u009c", "\x1b\\", "\u0098", "\x1bX", "\u009e", "\x1b^", "\u009f", "\x1b_").Replace(text)
	text = ansi.Strip(text)
	return strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && r != '‍' && r != '‌') {
			return -1
		}
		return r
	}, text)
}
