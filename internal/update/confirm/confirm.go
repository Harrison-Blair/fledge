// Package confirm asks the user to approve an update on an interactive terminal.
package confirm

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Proceed prompts on out and reads the answer from in. Only "y" or "Y" approves;
// any other answer, including EOF, cancels with a notice and returns false. A
// non-terminal input is an error so scripts must pass --yes. isTerminal
// overrides detection; when nil, only a terminal *os.File counts.
func Proceed(in io.Reader, out io.Writer, isTerminal func() bool, latest, current string) (bool, error) {
	terminal := false
	if isTerminal != nil {
		terminal = isTerminal()
	} else if f, ok := in.(*os.File); ok {
		terminal = term.IsTerminal(int(f.Fd()))
	}
	if !terminal {
		return false, fmt.Errorf("update: stdin is not a terminal; use --yes to update without a prompt")
	}
	if _, err := fmt.Fprintf(out, "Install fledge %s over %s? [y/N] ", latest, current); err != nil {
		return false, err
	}
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("update: read confirmation: %w", err)
	}
	if strings.TrimSpace(answer) != "y" && strings.TrimSpace(answer) != "Y" {
		_, err := fmt.Fprintln(out, "update canceled")
		return false, err
	}
	return true, nil
}
