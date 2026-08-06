package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	kindWidth = 13
	// routeWidth holds "origin -> target" for the longest agent names in common
	// use; a longer pair pushes the remaining columns right rather than wrapping.
	routeWidth = 25
	refWidth   = 7
	// bodyRunes keeps one exchange readable beside everything else happening in
	// the session. The stored JSON always has the whole body.
	bodyRunes = 100
)

const (
	reset   = "\x1b[0m"
	red     = "\x1b[31m"
	yellow  = "\x1b[33m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
	dim     = "\x1b[2m"
)

// Human renders one record as a single terminal line, without a trailing
// newline. Color is applied to the kind column only, so the trace stays
// readable when the rest of the line is grepped or copied.
func Human(r Record, color bool) string {
	kind := fmt.Sprintf("%-*s", kindWidth, r.Kind)
	if color {
		if code := colorFor(r.Kind); code != "" {
			kind = code + kind + reset
		}
	}
	route := r.Origin
	if r.Target != "" {
		route += " -> " + r.Target
	}
	line := r.At.Format("15:04:05") + " " + kind +
		fmt.Sprintf(" %-*s %-*s ", routeWidth, route, refWidth, shortRef(r.Ref)) + detail(r)
	return strings.TrimRight(line, " ")
}

// JSON renders one record as the line stored in the dispatcher log.
func JSON(r Record) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(r); err != nil {
		return nil, fmt.Errorf("encode trace record: %w", err)
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

// Decode reads back a record stored by JSON. A line that is not a record, such
// as output from an older Fledge, reports false.
func Decode(line []byte) (Record, bool) {
	var r Record
	if err := json.Unmarshal(line, &r); err != nil {
		return Record{}, false
	}
	if strings.TrimSpace(r.Kind) == "" {
		return Record{}, false
	}
	return r, true
}

// detail is the trailing column. A record either carries a body — the thing
// actually exchanged — or the fields describing what the dispatcher did with
// it; printing both would bury the body no one can reconstruct from the rest.
func detail(r Record) string {
	if body := truncate(stripC0(strings.Join(strings.Fields(r.Body), " ")), bodyRunes); body != "" {
		return body
	}
	var parts []string
	if r.Note != "" {
		parts = append(parts, r.Note)
	}
	if r.Pane != "" {
		parts = append(parts, "pane="+r.Pane)
	}
	if r.Status != "" {
		parts = append(parts, "status="+r.Status)
	}
	return stripC0(strings.Join(parts, " "))
}

func stripC0(value string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' {
			return -1
		}
		return r
	}, value)
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

// shortRef keeps a durable ID recognizable without spending a column on the
// random part nobody reads: the prefix plus the first four characters is enough
// to follow one message, task, or wake down the trace.
func shortRef(ref string) string {
	if index := strings.IndexByte(ref, '-'); index >= 0 {
		if len(ref) > index+5 {
			return ref[:index+5]
		}
		return ref
	}
	if len(ref) > refWidth {
		return ref[:refWidth]
	}
	return ref
}

func colorFor(kind string) string {
	switch {
	case strings.HasSuffix(kind, ".failed") || kind == "dispatcher.exit":
		return red
	case kind == "message" || kind == "reply":
		return cyan
	case strings.HasPrefix(kind, "task."):
		return magenta
	case strings.HasPrefix(kind, "wake.") || strings.HasPrefix(kind, "delivery."):
		return yellow
	case strings.HasPrefix(kind, "agent.") || strings.HasPrefix(kind, "herdr.") ||
		strings.HasPrefix(kind, "dispatcher.") || strings.HasPrefix(kind, "session."):
		return dim
	}
	return ""
}
