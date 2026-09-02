package session

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// TerminalConfirmer prompts on terminal input and output. Terminal detection
// is supplied by the CLI boundary so this package does not depend on a
// particular file-descriptor implementation.
type TerminalConfirmer struct {
	Input            io.Reader
	Output           io.Writer
	InputIsTerminal  bool
	OutputIsTerminal bool
}

// Confirm displays one default-negative prompt. Only y and yes, ignoring
// case, approve the operation. EOF and every other answer cancel it.
func (c TerminalConfirmer) Confirm(projectRoot string, names []string, selfStop bool) (bool, error) {
	if !c.InputIsTerminal || !c.OutputIsTerminal {
		return false, fmt.Errorf("confirmation requires terminal input and output")
	}
	if c.Input == nil {
		return false, fmt.Errorf("confirmation input is nil")
	}
	if c.Output == nil {
		return false, fmt.Errorf("confirmation output is nil")
	}

	targets := append([]string(nil), names...)
	sort.Strings(targets)
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Stop Fledge sessions for project %q?\n", projectRoot)
	for _, name := range targets {
		fmt.Fprintf(&prompt, "  - %s\n", name)
	}
	prompt.WriteString("This will terminate all panes and agents in these sessions.\n")
	if selfStop {
		prompt.WriteString("The current Herder session is included; Fledge may exit before showing final output.\n")
	}
	prompt.WriteString("Continue? [y/N] ")
	if _, err := io.WriteString(c.Output, prompt.String()); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}

	answer, err := bufio.NewReader(c.Input).ReadString('\n')
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer = strings.TrimSuffix(answer, "\n")
	answer = strings.TrimSuffix(answer, "\r")
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}
