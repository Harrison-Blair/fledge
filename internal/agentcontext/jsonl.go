package agentcontext

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// readTranscript follows Herdr's correlation kind exactly. A path reference is
// read as the one path Herdr reported; an id reference is located only through
// the harness-specific filename pattern. Neither kind falls back to the other.
func readTranscript(ref Ref, deps Deps, idPattern string) ([]byte, error) {
	switch ref.Kind {
	case "path":
		if !filepath.IsAbs(ref.Value) {
			return nil, errNativeSession
		}
		return readTranscriptFile(deps, filepath.Clean(ref.Value))
	case "id":
		if !safeSessionID(ref.Value) {
			return nil, errNativeSession
		}
		return readOnlyMatch(deps, idPattern)
	default:
		return nil, errNativeSession
	}
}

// safeSessionID keeps a native id in one literal filename segment. Rejecting
// glob metacharacters is what makes an id lookup exact instead of allowing a
// reported id to widen the match.
func safeSessionID(value string) bool {
	return value != "" && filepath.Base(value) == value && value != "." && value != ".." &&
		!strings.ContainsAny(value, `*?[\\`)
}

// readOnlyMatch globs a transcript pattern and returns its bytes. Zero matches
// is transcript_not_found; a glob or read failure is a telemetry error. When a
// pattern matches more than one file correlation is ambiguous, so no reading
// is reported.
func readOnlyMatch(deps Deps, pattern string) ([]byte, error) {
	matches, err := deps.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, errTranscriptNotFound
	}
	if len(matches) != 1 {
		return nil, errors.New("native session id matched multiple transcripts")
	}
	return readTranscriptFile(deps, matches[0])
}

func readTranscriptFile(deps Deps, path string) ([]byte, error) {
	contents, err := deps.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errTranscriptNotFound
	}
	return contents, err
}

// splitLines splits transcript bytes into non-empty lines without allocating a
// scanner, tolerating both \n and \r\n terminators.
func splitLines(contents []byte) [][]byte {
	raw := bytes.Split(contents, []byte("\n"))
	lines := make([][]byte, 0, len(raw))
	for _, line := range raw {
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
