package usage

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/Harrison-Blair/fledge/internal/lib/harness"
)

// Basis values for a Summary.
const (
	Measured    = "measured"
	Unavailable = "unavailable"
)

// Tokens counts one session's tokens. Input excludes cache reads and writes.
type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Reasoning  int64 `json:"reasoning"`
}

func (t *Tokens) add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
	t.Reasoning += o.Reasoning
}

// Cost is a harness-recorded cost; Basis is always "estimate".
type Cost struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Basis    string  `json:"basis"`
	Source   string  `json:"source"`
}

// Summary is one session's usage inside a window. Subagents holds
// harness-internal sub-agents, already folded into Tokens and Turns.
type Summary struct {
	Harness      string     `json:"harness"`
	SessionKind  string     `json:"session_kind"`
	SessionValue string     `json:"session_value"`
	Models       []string   `json:"models"`
	Turns        int        `json:"turns"`
	Tokens       Tokens     `json:"tokens"`
	Cost         *Cost      `json:"cost"`
	First        *time.Time `json:"first"`
	Last         *time.Time `json:"last"`
	Subagents    *Tokens    `json:"subagents"`
	Basis        string     `json:"basis"`
	Reason       string     `json:"reason"`
	Sources      []string   `json:"sources"`
}

// Ref is a native session ref: Kind "id" or "path". Cwd is the session's
// working directory, which claude needs to resolve an id.
type Ref struct {
	Kind  string
	Value string
	Cwd   string
}

// Window bounds response timestamps; a nil end is unbounded.
type Window struct {
	From, To *time.Time
}

// contains reports whether a response at ts falls inside w. A response
// without a timestamp counts only when w is unbounded.
func (w Window) contains(ts *time.Time) bool {
	if ts == nil {
		return w.From == nil && w.To == nil
	}
	return (w.From == nil || !ts.Before(*w.From)) && (w.To == nil || !ts.After(*w.To))
}

// Runner executes a harness command and returns its standard output.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Discovery reads session stores under Home and runs harness commands through Run.
type Discovery struct {
	Home string
	Run  Runner
}

// LocalDiscovery reads the real home directory and executes real harness commands.
func LocalDiscovery() Discovery {
	home, _ := os.UserHomeDir()
	return Discovery{Home: home, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, name, args...)
		command.Stderr = io.Discard
		return command.Output()
	}}
}

type reader func(ctx context.Context, d Discovery, ref Ref, w Window) (*tally, error)

var readers = map[string]reader{
	"claude":   readClaude,
	"codex":    readCodex,
	"pi":       readPi,
	"opencode": readOpencode,
}

// Read summarizes kind's session ref inside w. Data problems never fail the
// caller: they yield Basis unavailable with a Reason.
func Read(ctx context.Context, d Discovery, kind string, ref Ref, w Window) Summary {
	s := Summary{Harness: kind, SessionKind: ref.Kind, SessionValue: ref.Value}
	p, ok := harness.Lookup(kind)
	if !ok {
		return s.unavailable(fmt.Sprintf("unknown harness %q", kind))
	}
	if p.Usage == harness.UsageNone {
		reason := p.Evidence[harness.UsageTokens]
		if reason == "" {
			reason = "harness records no usage"
		}
		return s.unavailable(reason)
	}
	read := readers[kind]
	if read == nil {
		return s.unavailable("no usage reader for harness " + kind)
	}
	t, err := read(ctx, d, ref, w)
	if err != nil {
		return s.unavailable(err.Error())
	}
	return t.summary(s)
}

func (s Summary) unavailable(reason string) Summary {
	s.Basis, s.Reason = Unavailable, reason
	return s
}

// Locate returns the session file for a file-backed harness's ref.
func Locate(d Discovery, kind string, ref Ref) (string, error) {
	if ref.Kind == "path" {
		return ref.Value, nil
	}
	if ref.Kind != "id" || ref.Value == "" {
		return "", fmt.Errorf("unsupported session ref kind %q", ref.Kind)
	}
	var pattern string
	switch kind {
	case "claude":
		if ref.Cwd == "" {
			return "", errors.New("claude id ref needs the session cwd")
		}
		return filepath.Join(d.Home, ".claude", "projects", claudeSlug(ref.Cwd), ref.Value+".jsonl"), nil
	case "codex":
		pattern = filepath.Join(d.Home, ".codex", "sessions", "*", "*", "*", "rollout-*-"+ref.Value+".jsonl")
	case "pi":
		pattern = filepath.Join(d.Home, ".pi", "agent", "sessions", "*", "*_"+ref.Value+".jsonl")
	default:
		return "", fmt.Errorf("harness %s has no session file", kind)
	}
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return "", fmt.Errorf("no %s session file for id %s", kind, ref.Value)
	}
	return matches[0], nil
}

// response is one deduplicated model response.
type response struct {
	at     *time.Time
	model  string
	tokens Tokens
	cost   float64
}

// tally accumulates the responses a reader keeps.
type tally struct {
	tokens     Tokens
	subagents  *Tokens
	cost       *Cost
	models     []string
	turns      int
	first      *time.Time
	last       *time.Time
	sources    []string
	lines      int
	malformed  int
	recognized bool // saw the harness's session marker
	records    int  // usage records parsed, before window filtering
	note       string
}

// add counts r when it falls inside w.
func (t *tally) add(w Window, r response) {
	if !w.contains(r.at) {
		return
	}
	t.tokens.add(r.tokens)
	t.turns++
	if t.cost != nil {
		t.cost.Amount += r.cost
	}
	if r.model != "" && !slices.Contains(t.models, r.model) {
		t.models = append(t.models, r.model)
	}
	if r.at != nil {
		if t.first == nil || r.at.Before(*t.first) {
			t.first = r.at
		}
		if t.last == nil || r.at.After(*t.last) {
			t.last = r.at
		}
	}
}

// merge folds a sub-agent tally into t and its Subagents component.
func (t *tally) merge(o *tally) {
	if t.subagents == nil {
		t.subagents = &Tokens{}
	}
	t.subagents.add(o.tokens)
	t.tokens.add(o.tokens)
	t.turns += o.turns
	for _, m := range o.models {
		if !slices.Contains(t.models, m) {
			t.models = append(t.models, m)
		}
	}
	if o.first != nil && (t.first == nil || o.first.Before(*t.first)) {
		t.first = o.first
	}
	if o.last != nil && (t.last == nil || o.last.After(*t.last)) {
		t.last = o.last
	}
	t.sources = append(t.sources, o.sources...)
	t.lines += o.lines
	t.malformed += o.malformed
}

func (t *tally) summary(s Summary) Summary {
	s.Models, s.Turns, s.Tokens, s.Cost = t.models, t.turns, t.tokens, t.cost
	s.First, s.Last, s.Subagents, s.Sources = t.first, t.last, t.subagents, t.sources
	s.Basis, s.Reason = Measured, t.note
	if t.malformed > 0 {
		s.Reason = joinReason(s.Reason, fmt.Sprintf("%d malformed lines of %d skipped", t.malformed, t.lines))
	}
	return s
}

// check rejects a session file whose malformed lines leave no readable usage
// record, or that lacks the harness's session marker, so unreadable usage is
// never reported as a measured zero.
func (t *tally) check(path, kind string) error {
	if t.malformed > 0 && t.records == 0 {
		return fmt.Errorf("%d malformed lines of %d and no readable usage records in %s", t.malformed, t.lines, path)
	}
	if !t.recognized {
		return fmt.Errorf("%s is not a recognized %s session file", path, kind)
	}
	return nil
}

func joinReason(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// scanLines calls parse for each non-blank line of path; a parse error counts
// the line as malformed and skips it.
func (t *tally) scanLines(path string, parse func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	t.sources = append(t.sources, path)
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			t.lines++
			if parse(line) != nil {
				t.malformed++
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
