// Package herdrtest renders fake herdr binaries for tests that drive Fledge
// through the herdr command line.
package herdrtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/herdr"
)

const (
	// VersionOutput is what a fake herdr reports for "--version".
	VersionOutput = "herdr 0.7.5"
	// protocol is the wire protocol Schema advertises.
	protocol = 17
	// emptySessions is the shell-quoted "session list" payload of a herdr with
	// no sessions.
	emptySessions = `'{"sessions":[]}'`
)

// Schema returns the "api schema" payload advertising every method Fledge requires.
func Schema() string {
	methods := make([]string, 0, len(herdr.RequiredMethods))
	for _, method := range herdr.RequiredMethods {
		methods = append(methods, fmt.Sprintf(`{"method":{"const":%s}}`, strconv.Quote(method)))
	}
	return fmt.Sprintf(`{"protocol":%d,"requests":[%s]}`, protocol, strings.Join(methods, ","))
}

// SessionCase is one "session list" answer: Payload is printed when Marker
// exists. A lone case with an empty Marker is printed unconditionally; in a
// multi-case list every Marker must be set.
//
// Payload is quoted with strconv.Quote, which does not escape "$" or backticks
// and leaves the shell free to expand them inside the double quotes it
// produces: payloads containing "$", backticks or control characters are
// unsupported.
type SessionCase struct {
	Marker  string
	Payload string
}

// Branch is a script branch beyond the ones Options renders: Body runs when the
// shell Condition holds.
type Branch struct {
	Condition string
	Body      string
}

// Options describes the fake herdr script WriteBinary renders. Branches are
// emitted in the order they are documented here, so every condition must be
// mutually exclusive.
type Options struct {
	// InvocationLog, when set, receives the arguments of every invocation.
	InvocationLog string
	// Version, when set, is reported by "--version" and enables "api schema".
	Version string
	// Sessions answers "session list", trying each case in order and falling
	// back to an empty session list. No case omits the branch.
	Sessions []SessionCase
	// DeleteRemoves, when set, enables "session delete" and names the marker a
	// successful deletion removes. It is required by the branch: DeleteLog and
	// DeleteFailOnce have no effect without it.
	DeleteRemoves string
	// DeleteLog, when set, receives the arguments of the deleting invocation,
	// replacing any earlier contents.
	DeleteLog string
	// DeleteFailOnce, when set, names a marker whose presence makes "session
	// delete" remove it, report a failure on stderr and exit 4.
	DeleteFailOnce string
	// Branches are appended after the branches above.
	Branches []Branch
	// UnknownExit is the exit status of an unrecognized invocation, 2 by default.
	UnknownExit int
}

// WriteBinary renders a fake herdr into dir and returns its path.
func WriteBinary(t *testing.T, dir string, opts Options) string {
	t.Helper()
	if len(opts.Sessions) > 1 {
		for i, sessionCase := range opts.Sessions {
			if sessionCase.Marker == "" {
				t.Fatalf("herdrtest: session case %d of %d needs a marker",
					i+1, len(opts.Sessions))
			}
		}
	}
	path := filepath.Join(dir, "herdr-fake")
	if err := os.WriteFile(path, []byte(render(opts)), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func render(opts Options) string {
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	if opts.InvocationLog != "" {
		script.WriteString(printfLine(`"$*"`, ">> "+strconv.Quote(opts.InvocationLog)))
	}
	var branches []Branch
	if opts.Version != "" {
		branches = append(branches,
			Branch{
				Condition: `[ "$1" = "--version" ]`,
				Body:      fmt.Sprintf("echo %s\n", strconv.Quote(opts.Version)),
			},
			Branch{
				Condition: `[ "$1" = "api" ] && [ "$2" = "schema" ]`,
				Body:      printfLine(strconv.Quote(Schema()), ""),
			},
		)
	}
	if len(opts.Sessions) > 0 {
		branches = append(branches, Branch{
			Condition: `[ "$1" = "session" ] && [ "$2" = "list" ]`,
			Body:      sessionListBody(opts.Sessions),
		})
	}
	if opts.DeleteRemoves != "" {
		branches = append(branches, Branch{
			Condition: `[ "$1" = "session" ] && [ "$2" = "delete" ]`,
			Body:      deleteBody(opts),
		})
	}
	branches = append(branches, opts.Branches...)
	unknownExit := opts.UnknownExit
	if unknownExit == 0 {
		unknownExit = 2
	}
	if len(branches) == 0 {
		fmt.Fprintf(&script, "exit %d\n", unknownExit)
		return script.String()
	}
	for i, branch := range branches {
		keyword := "elif"
		if i == 0 {
			keyword = "if"
		}
		fmt.Fprintf(&script, "%s %s; then\n%s", keyword, branch.Condition, indent(branch.Body))
	}
	fmt.Fprintf(&script, "else\n  exit %d\nfi\n", unknownExit)
	return script.String()
}

func sessionListBody(cases []SessionCase) string {
	if len(cases) == 1 && cases[0].Marker == "" {
		return printfLine(strconv.Quote(cases[0].Payload), "")
	}
	var body strings.Builder
	for i, sessionCase := range cases {
		keyword := "elif"
		if i == 0 {
			keyword = "if"
		}
		fmt.Fprintf(&body, "%s [ -f %s ]; then\n%s", keyword, strconv.Quote(sessionCase.Marker),
			indent(printfLine(strconv.Quote(sessionCase.Payload), "")))
	}
	fmt.Fprintf(&body, "else\n%sfi\n", indent(printfLine(emptySessions, "")))
	return body.String()
}

func deleteBody(opts Options) string {
	var body strings.Builder
	if opts.DeleteFailOnce != "" {
		marker := strconv.Quote(opts.DeleteFailOnce)
		fmt.Fprintf(&body, "if [ -f %s ]; then\n%sfi\n", marker, indent(fmt.Sprintf(
			"rm -f %s\necho \"injected deletion failure\" >&2\nexit 4\n", marker)))
	}
	if opts.DeleteLog != "" {
		body.WriteString(printfLine(`"$*"`, "> "+strconv.Quote(opts.DeleteLog)))
	}
	fmt.Fprintf(&body, "rm -f %s\n", strconv.Quote(opts.DeleteRemoves))
	body.WriteString(printfLine(`'{"deleted":true}'`, ""))
	return body.String()
}

// printfLine prints one already shell-quoted operand, optionally redirected.
// Operands quoted with strconv.Quote stay subject to shell expansion; see
// SessionCase.Payload for which characters that rules out.
func printfLine(operand, redirect string) string {
	line := fmt.Sprintf(`printf '%%s\n' %s`, operand)
	if redirect != "" {
		line += " " + redirect
	}
	return line + "\n"
}

func indent(body string) string {
	var indented strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		fmt.Fprintf(&indented, "  %s\n", line)
	}
	return indented.String()
}
