package cli

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/spec"
)

func init() {
	register("criteria", runCriteria, "fledge criteria <ID> [--json] | fledge criteria check|uncheck <ID> <AC-N> [--json]")
}

// criterionJSON is one checkbox in `criteria <ID> --json` output.
type criterionJSON struct {
	N       int    `json:"n"`
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
	Text    string `json:"text"`
}

func runCriteria(args []string) int {
	fs := flag.NewFlagSet("criteria", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	positional, err := parseMixed(fs, args)
	if err != nil {
		return ExitUsage
	}
	if len(positional) == 0 {
		return usageErr("usage: fledge criteria <ID> | fledge criteria check|uncheck <ID> <AC-N>")
	}

	switch positional[0] {
	case "check", "uncheck":
		if len(positional) != 3 {
			return usageErr("usage: fledge criteria %s <ID> <AC-N>", positional[0])
		}
		n, err := parseACNumber(positional[2])
		if err != nil {
			return usageErr("%v", err)
		}
		return setCriterion(positional[1], n, positional[0] == "check", *jsonOut)
	default:
		if len(positional) != 1 {
			return usageErr("usage: fledge criteria <ID>")
		}
		return listCriteria(positional[0], *jsonOut)
	}
}

// uncheckedCriteria lists the labels of unchecked AC boxes in a spec body.
// Empty when all are checked or none are parseable (legacy prose criteria).
func uncheckedCriteria(body []byte) []string {
	var out []string
	for _, c := range spec.ParseCriteria(body) {
		if !c.Checked {
			out = append(out, c.Label)
		}
	}
	return out
}

// parseACNumber accepts "2" or "AC-2".
func parseACNumber(s string) (int, error) {
	trimmed := strings.TrimPrefix(s, "AC-")
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("usage: criterion must be a number or AC-<n>, got %q", s)
	}
	return n, nil
}

// specBody resolves an ID to its body and save hooks; ok=false with the
// printed exit code when the ID is unknown.
func specBody(id string) (body []byte, setBody func([]byte), save func() error, exitCode int, ok bool) {
	_, set, _, code, loaded := loadSet()
	if !loaded {
		return nil, nil, nil, code, false
	}
	if t := set.Task(id); t != nil {
		return t.Body, func(b []byte) { t.Body = b }, t.Save, 0, true
	}
	if r := set.Req(id); r != nil {
		return r.Body, func(b []byte) { r.Body = b }, r.Save, 0, true
	}
	return nil, nil, nil, fail("%s not found", id), false
}

func listCriteria(id string, jsonOut bool) int {
	body, _, _, code, ok := specBody(id)
	if !ok {
		return code
	}
	cs := spec.ParseCriteria(body)
	checked := 0
	for _, c := range cs {
		if c.Checked {
			checked++
		}
	}
	if jsonOut {
		items := make([]criterionJSON, 0, len(cs))
		for _, c := range cs {
			items = append(items, criterionJSON{c.N, c.Label, c.Checked, c.Text})
		}
		return emitJSON(map[string]any{
			"id": id, "total": len(cs), "checked": checked, "criteria": items,
		})
	}
	for _, c := range cs {
		box := " "
		if c.Checked {
			box = "x"
		}
		fmt.Printf("[%s] %s: %s\n", box, c.Label, c.Text)
	}
	fmt.Printf("%s: %d/%d checked\n", id, checked, len(cs))
	return ExitOK
}

func setCriterion(id string, n int, checked bool, jsonOut bool) int {
	body, setBody, save, code, ok := specBody(id)
	if !ok {
		return code
	}
	newBody, changed, err := spec.SetCriterion(body, n, checked)
	if err != nil {
		return fail("%s: %v", id, err)
	}
	if changed {
		setBody(newBody)
		if err := save(); err != nil {
			return fail("%v", err)
		}
	}
	if jsonOut {
		return emitJSON(map[string]any{
			"id": id, "label": fmt.Sprintf("AC-%d", n), "checked": checked, "changed": changed,
		})
	}
	verb := "checked"
	if !checked {
		verb = "unchecked"
	}
	if !changed {
		verb = "already " + verb
	}
	fmt.Printf("%s: AC-%d %s\n", id, n, verb)
	return ExitOK
}
