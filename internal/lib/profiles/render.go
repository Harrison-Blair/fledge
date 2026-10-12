package profiles

import (
	_ "embed"
	"strings"
)

// shared is the Fledge protocol block every profile's brief ends with.
//
//go:embed builtin/protocol.md
var shared string

// Brief renders the role text byte for byte, then the shared Fledge protocol
// block, separated by one blank line.
func (p Profile) Brief() string {
	sep := "\n\n"
	if strings.HasSuffix(p.Role, "\n") {
		sep = "\n"
	}
	return p.Role + sep + "## Fledge protocol\n" + shared
}
