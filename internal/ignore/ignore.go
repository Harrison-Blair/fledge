// Package ignore implements .fledgeignore matching using gitignore semantics.
package ignore

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// includePrefix splices another ignore file in at the point it appears. It is
// spelled as a comment so a .fledgeignore stays valid gitignore syntax; the
// space in "# include" keeps ordinary comments from being mistaken for it.
const includePrefix = "#include"

// Matcher holds the patterns from one ignore file, in file order.
type Matcher struct {
	patterns []pattern
}

type pattern struct {
	re      *regexp.Regexp
	negate  bool
	dirOnly bool
}

// ParseFile reads an ignore file. A missing file yields an empty matcher, so a
// scan still works in a tree that has never been initialized. Paths named by
// "#include" directives resolve against root, as do the patterns themselves.
func ParseFile(path, root string) (*Matcher, error) {
	m := &Matcher{}
	if err := m.load(path, root, map[string]bool{}, true); err != nil {
		return nil, err
	}
	return m, nil
}

// Parse reads patterns, one per line. Includes resolve against root.
func Parse(r io.Reader, root string) (*Matcher, error) {
	m := &Matcher{}
	if err := m.read(r, root, map[string]bool{}); err != nil {
		return nil, err
	}
	return m, nil
}

// load reads one file into m. An included file that is missing is an error —
// the directive asserts it exists — while the top-level file is optional.
func (m *Matcher) load(path, root string, seen map[string]bool, optional bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if seen[abs] {
		return fmt.Errorf("%s: include cycle", path)
	}
	seen[abs] = true

	f, err := os.Open(path)
	if err != nil {
		if optional && os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	return m.read(f, root, seen)
}

func (m *Matcher) read(r io.Reader, root string, seen map[string]bool) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()

		if target, ok := includeTarget(line); ok {
			if target == "" {
				return fmt.Errorf("%s: directive needs a path", includePrefix)
			}
			included := filepath.Join(root, filepath.FromSlash(target))
			if err := m.load(included, root, seen, false); err != nil {
				return err
			}
			continue
		}

		p, ok, err := parseLine(line)
		if err != nil {
			return err
		}
		if ok {
			m.patterns = append(m.patterns, p)
		}
	}
	return sc.Err()
}

// includeTarget reports whether line is an include directive, and the path it names.
func includeTarget(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), includePrefix)
	if !ok {
		return "", false
	}
	// Require a separator, so "#includes-are-neat" stays a comment.
	if rest != "" && !strings.ContainsAny(rest[:1], " \t") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// Match reports whether a slash-separated path relative to the scan root is
// ignored. The last pattern to match decides, so a later "!" line re-includes.
func (m *Matcher) Match(path string, isDir bool) bool {
	ignored := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if p.re.MatchString(path) {
			ignored = !p.negate
		}
	}
	return ignored
}

func parseLine(line string) (pattern, bool, error) {
	line = trimTrailingSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return pattern{}, false, nil
	}

	var p pattern
	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = line[1:]
	}
	// A leading backslash only escapes the "#" and "!" that would otherwise
	// have been consumed above.
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return pattern{}, false, nil
	}

	// A pattern containing a slash is anchored to the scan root; one without
	// matches at any depth.
	anchored := strings.Contains(line, "/")
	line = strings.TrimPrefix(line, "/")

	re, err := compile(line, anchored)
	if err != nil {
		return pattern{}, false, err
	}
	p.re = re
	return p, true, nil
}

// trimTrailingSpace drops trailing spaces unless the last one is backslash-escaped.
func trimTrailingSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		if len(s) >= 2 && s[len(s)-2] == '\\' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// compile translates a glob into an anchored regexp. "*" and "?" stop at a
// slash; "**" spans directories only when it stands as a whole segment.
func compile(glob string, anchored bool) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}

	for i, n := 0, len(glob); i < n; {
		switch c := glob[i]; c {
		case '\\':
			if i+1 < n {
				b.WriteString(regexp.QuoteMeta(string(glob[i+1])))
				i += 2
			} else {
				b.WriteString(regexp.QuoteMeta(`\`))
				i++
			}

		case '*':
			j := i
			for j < n && glob[j] == '*' {
				j++
			}
			wholeSegment := (i == 0 || glob[i-1] == '/') && (j == n || glob[j] == '/')
			switch {
			case j-i >= 2 && wholeSegment && j == n:
				// trailing "/**": everything below, but not the dir itself
				b.WriteString(".*")
			case j-i >= 2 && wholeSegment:
				// leading or interior "**/": zero or more directories
				b.WriteString("(?:.*/)?")
				j++ // consume the slash
			default:
				b.WriteString("[^/]*")
			}
			i = j

		case '?':
			b.WriteString("[^/]")
			i++

		case '[':
			j := i + 1
			if j < n && (glob[j] == '!' || glob[j] == '^') {
				j++
			}
			if j < n && glob[j] == ']' {
				j++
			}
			for j < n && glob[j] != ']' {
				j++
			}
			if j >= n {
				b.WriteString(regexp.QuoteMeta("["))
				i++
				break
			}
			class := glob[i+1 : j]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i = j + 1

		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}

	b.WriteString("$")
	return regexp.Compile(b.String())
}
