package fledge

import (
	"strings"
	"unicode/utf8"
)

// renderPTYCommand validates and shell-quotes argv for injection into a
// terminal pane. Terminal control bytes are rejected rather than quoted:
// quoting changes shell interpretation, but it cannot make bytes such as ETX
// or CR safe from the terminal emulator that receives them first.
func renderPTYCommand(argv []string) (string, error) {
	for _, arg := range argv {
		if !validTerminalArgument(arg) {
			// Do not include arg in this error. Invalid terminal input can contain
			// bytes that alter or forge the user's terminal output.
			return "", NewError("invalid_terminal_input", "terminal command contains invalid input")
		}
	}
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " "), nil
}

func validTerminalArgument(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r <= '\x1f' || r >= '\x7f' && r <= '\x9f' {
			return false
		}
	}
	return true
}

// shellQuote returns value unquoted only when every rune is on a conservative
// always-safe allowlist. Everything else, including printable Unicode and
// shell metacharacters, is single-quoted with embedded quotes escaped.
func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, shellUnsafeRune) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func shellUnsafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("@%+=:,./_-", r)
}
