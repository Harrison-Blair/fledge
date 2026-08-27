// Package catalog reports the model identifiers each supported agent harness
// accepts. Every lookup shells out to the harness CLI, so results reflect the
// models installed on this machine.
package catalog

import (
	"bytes"
	"context"
	"fledge/internal/subprocess"
	"slices"
	"strings"
	"sync"
	"time"
)

// Harness names an agent CLI that Fledge can launch.
type Harness string

const (
	Pi       Harness = "pi"
	Claude   Harness = "claude"
	Codex    Harness = "codex"
	OpenCode Harness = "opencode"
	Cursor   Harness = "cursor"
)

// codexProvider is the pi provider whose rows list the models Codex accepts.
const codexProvider = "openai-codex"

// claudePrefix marks the models Claude Code accepts.
const claudePrefix = "claude-"

// cursorSeparator divides a model ID from its description in cursor-agent
// --list-models output.
const cursorSeparator = " - "

// Harnesses returns every supported harness in presentation order.
func Harnesses() []Harness {
	return []Harness{Pi, Claude, Codex, OpenCode, Cursor}
}

// Models returns the model IDs harness accepts via --model, de-duplicated
// and ordered by model family (see families), then highest-version-first
// within each family. timeout bounds each
// harness command and must be positive; a non-positive timeout reports no
// models. Models never fails: a missing binary, a non-zero exit, or a
// timeout all yield an empty list, as does an unrecognized harness.
func Models(ctx context.Context, harness Harness, timeout time.Duration) []string {
	switch harness {
	case Pi:
		return piModels(ctx, timeout)
	case Claude:
		return claudeModels(ctx, timeout)
	case Codex:
		return codexModels(ctx, timeout)
	case OpenCode:
		return normalize(openCodeLines(ctx, timeout))
	case Cursor:
		return cursorModels(ctx, timeout)
	}
	return nil
}

// piModels reports every model pi lists, qualified by its provider because pi
// selects models as "provider/model".
func piModels(ctx context.Context, timeout time.Duration) []string {
	rows := piRows(ctx, timeout)
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.provider+"/"+row.model)
	}
	return normalize(ids)
}

// codexModels reports the Codex-provider rows of the pi table. Codex selects
// models by bare name, so the provider column is dropped.
func codexModels(ctx context.Context, timeout time.Duration) []string {
	var ids []string
	for _, row := range piRows(ctx, timeout) {
		if row.provider == codexProvider {
			ids = append(ids, row.model)
		}
	}
	return normalize(ids)
}

// claudeModels unions the Claude models advertised by pi and by opencode. The
// two catalogs overlap, so the sources are queried concurrently and merged.
func claudeModels(ctx context.Context, timeout time.Duration) []string {
	var (
		wg    sync.WaitGroup
		rows  []piRow
		lines []string
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		rows = piRows(ctx, timeout)
	}()
	go func() {
		defer wg.Done()
		lines = openCodeLines(ctx, timeout)
	}()
	wg.Wait()

	var ids []string
	for _, row := range rows {
		if name := claudeName(row.model); name != "" {
			ids = append(ids, name)
		}
	}
	for _, line := range lines {
		if name := claudeName(line); name != "" {
			ids = append(ids, name)
		}
	}
	return normalize(ids)
}

// cursorModels reports the models cursor-agent lists, which the command prints
// as "id - description" rows framed by a heading and a usage tip. Only the
// rows carry the separator, so it also skips the surrounding prose. The
// command fails while unauthenticated.
func cursorModels(ctx context.Context, timeout time.Duration) []string {
	out, ok := run(ctx, timeout, "cursor-agent", "--list-models")
	if !ok {
		return nil
	}
	var ids []string
	for _, line := range parseLines(out) {
		id, _, found := strings.Cut(line, cursorSeparator)
		if !found {
			continue
		}
		ids = append(ids, strings.TrimSpace(id))
	}
	return normalize(ids)
}

func piRows(ctx context.Context, timeout time.Duration) []piRow {
	out, ok := run(ctx, timeout, "pi", "--list-models")
	if !ok {
		return nil
	}
	return parsePiTable(out)
}

func openCodeLines(ctx context.Context, timeout time.Duration) []string {
	out, ok := run(ctx, timeout, "opencode", "models")
	if !ok {
		return nil
	}
	return parseLines(out)
}

// claudeName strips any "provider/" prefix from model and returns the result
// when it names a Claude model, or "" when it does not.
func claudeName(model string) string {
	if _, name, found := strings.Cut(model, "/"); found {
		model = name
	}
	if !strings.HasPrefix(model, claudePrefix) {
		return ""
	}
	return model
}

// normalize sorts ids by family rank, then highest-first in descending
// natural order within a rank, and drops duplicates, reporting no models as
// a nil slice.
func normalize(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	slices.SortFunc(ids, compareModels)
	return slices.Compact(ids)
}

// run executes name with args and reports its standard output. It reports
// false when the command cannot start, exits non-zero, or outlives timeout.
func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout bytes.Buffer
	cmd := subprocess.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return stdout.String(), true
}
