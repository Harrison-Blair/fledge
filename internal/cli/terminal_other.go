//go:build !linux

package cli

import "io"

func isTerminalReader(io.Reader) bool {
	return false
}
