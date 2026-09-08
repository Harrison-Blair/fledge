package herdr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Diagnostic bounds. Rendered argument vectors and captured output only ever
// appear in error text, so each is capped regardless of what a caller or
// Herder supplies. The exec argv itself is never touched.
const (
	maxRenderedArgs   = 16
	maxVerbatimBytes  = 48
	maxOperationBytes = 512
	maxOutputBytes    = 512
	maxCodeBytes      = 64
)

// renderOperation describes one Herder invocation for error text. Every
// argument is rendered by renderArg, and the result stops at maxRenderedArgs
// arguments or maxOperationBytes bytes, whichever comes first, with the number
// of omitted arguments appended.
func renderOperation(argv []string) string {
	var b strings.Builder
	b.WriteString("herdr")
	for i, arg := range argv {
		omitted := len(argv) - i
		piece := renderArg(arg)
		if i >= maxRenderedArgs || b.Len()+1+len(piece) > maxOperationBytes-len(omittedArgs(omitted)) {
			b.WriteString(omittedArgs(omitted))
			return b.String()
		}
		b.WriteByte(' ')
		b.WriteString(piece)
	}
	return b.String()
}

func omittedArgs(count int) string {
	return fmt.Sprintf(" …+%d args", count)
}

// renderArg renders one argument so that boundaries are unambiguous. Plain
// arguments appear verbatim; anything containing whitespace, quotes, angle
// brackets, control characters, invalid UTF-8, or other punctuation is
// quoted and escaped; values longer than maxVerbatimBytes are replaced by a
// byte count so no prefix of a long value leaks.
func renderArg(arg string) string {
	if len(arg) > maxVerbatimBytes {
		return "<" + strconv.Itoa(len(arg)) + " bytes>"
	}
	if isPlainArg(arg) {
		return arg
	}
	return strconv.Quote(arg)
}

func isPlainArg(arg string) bool {
	if arg == "" {
		return false
	}
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case strings.IndexByte("-_./:=+,@%~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// renderOutput bounds captured stderr or a server message for error text.
func renderOutput(text string) string {
	return renderText(text, maxOutputBytes)
}

// renderText escapes control characters, backslashes, and invalid UTF-8 in
// text and stops once the rendered form would exceed limit bytes, appending
// the number of source bytes left unrendered.
func renderText(text string, limit int) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		piece := renderRune(r, size, text[i])
		if b.Len()+len(piece) > limit-len(omittedBytes(len(text)-i)) {
			b.WriteString(omittedBytes(len(text) - i))
			return b.String()
		}
		b.WriteString(piece)
		i += size
	}
	return b.String()
}

func renderRune(r rune, size int, raw byte) string {
	switch {
	case r == utf8.RuneError && size == 1:
		return fmt.Sprintf(`\x%02x`, raw)
	case r == '\\':
		return `\\`
	case r == '\n':
		return `\n`
	case r == '\r':
		return `\r`
	case r == '\t':
		return `\t`
	case r == ' ' || unicode.IsPrint(r):
		return string(r)
	case r < 0x10000:
		return fmt.Sprintf(`\u%04x`, r)
	default:
		return fmt.Sprintf(`\U%08x`, r)
	}
}

func omittedBytes(count int) string {
	return fmt.Sprintf(" …+%d bytes", count)
}
