package spec

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// SplitFrontmatter separates the leading `---` YAML block from the body.
// Body bytes are returned exactly as found and must never be re-serialized.
func SplitFrontmatter(b []byte) (fm, body []byte, err error) {
	var rest []byte
	switch {
	case bytes.HasPrefix(b, []byte("---\n")):
		rest = b[4:]
	case bytes.HasPrefix(b, []byte("---\r\n")):
		rest = b[5:]
	default:
		return nil, nil, errors.New("missing leading --- frontmatter delimiter")
	}
	off := 0
	for off < len(rest) {
		nl := bytes.IndexByte(rest[off:], '\n')
		var line []byte
		var next int
		if nl == -1 {
			line = rest[off:]
			next = len(rest)
		} else {
			line = rest[off : off+nl]
			next = off + nl + 1
		}
		if bytes.Equal(bytes.TrimSuffix(line, []byte("\r")), []byte("---")) {
			return rest[:off], rest[next:], nil
		}
		off = next
	}
	return nil, nil, errors.New("unterminated frontmatter (no closing ---)")
}

var reqKeys = []string{"id", "title", "status", "priority", "authored", "agent", "fledge_version"}
var taskKeys = []string{"id", "title", "plumage", "status", "priority", "depends_on", "oversight", "authored", "agent", "fledge_version"}

func parseFrontmatterMap(fm []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(fm, &m); err != nil {
		return nil, fmt.Errorf("invalid YAML frontmatter: %w", err)
	}
	return m, nil
}

func strField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}

func unknownKeys(m map[string]any, known []string) []string {
	var out []string
	for k := range m {
		found := false
		for _, kk := range known {
			if k == kk {
				found = true
				break
			}
		}
		if !found {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// ParseRequirementFile parses one PLM (plumage) file. Returns the requirement, any
// unknown frontmatter keys, and a parse error.
func ParseRequirementFile(path string, b []byte) (*Requirement, []string, error) {
	fm, body, err := SplitFrontmatter(b)
	if err != nil {
		return nil, nil, err
	}
	m, err := parseFrontmatterMap(fm)
	if err != nil {
		return nil, nil, err
	}
	r := &Requirement{
		ID:            strField(m, "id"),
		Title:         strField(m, "title"),
		Status:        strField(m, "status"),
		Priority:      strField(m, "priority"),
		Authored:      strField(m, "authored"),
		Agent:         strField(m, "agent"),
		FledgeVersion: strField(m, "fledge_version"),
		Path:          path,
		Body:          body,
	}
	return r, unknownKeys(m, reqKeys), nil
}

// ParseTaskFile parses one FTHR (feather) file. Returns the task, any unknown
// frontmatter keys, and a parse error.
func ParseTaskFile(path string, b []byte) (*Task, []string, error) {
	fm, body, err := SplitFrontmatter(b)
	if err != nil {
		return nil, nil, err
	}
	m, err := parseFrontmatterMap(fm)
	if err != nil {
		return nil, nil, err
	}
	t := &Task{
		ID:            strField(m, "id"),
		Title:         strField(m, "title"),
		Requirement:   strField(m, "plumage"),
		Status:        strField(m, "status"),
		Priority:      strField(m, "priority"),
		Oversight:     strField(m, "oversight"),
		Authored:      strField(m, "authored"),
		Agent:         strField(m, "agent"),
		FledgeVersion: strField(m, "fledge_version"),
		Path:          path,
		Body:          body,
	}
	if v, ok := m["depends_on"]; ok && v != nil {
		items, ok := v.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("depends_on must be a list, got %T", v)
		}
		for _, it := range items {
			t.DependsOn = append(t.DependsOn, fmt.Sprint(it))
		}
	}
	return t, unknownKeys(m, taskKeys), nil
}

// safeScalar matches strings that need no quoting in our frontmatter.
var safeScalar = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/()-]*$`)

// YAMLScalar returns s as a canonical YAML scalar: quoted when necessary
// (empty, numeric, boolean keyword, or containing unsafe characters), bare otherwise.
// The same quoting rules apply to all fledge frontmatter fields.
func YAMLScalar(s string) string { return yamlScalar(s) }

func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	lower := strings.ToLower(s)
	ambiguous := lower == "true" || lower == "false" || lower == "null" ||
		lower == "yes" || lower == "no" || lower == "on" || lower == "off"
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		ambiguous = true
	}
	if !safeScalar.MatchString(s) || ambiguous || s != strings.TrimSpace(s) {
		return strconv.Quote(s)
	}
	return s
}

// Frontmatter renders the canonical frontmatter block including both ---
// fences. Key order is fixed; optional keys are omitted when empty.
func (r *Requirement) Frontmatter() []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", r.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(r.Title))
	fmt.Fprintf(&b, "status: %s\n", r.Status)
	fmt.Fprintf(&b, "priority: %s\n", r.Priority)
	fmt.Fprintf(&b, "authored: %s\n", r.Authored)
	fmt.Fprintf(&b, "agent: %s\n", yamlScalar(r.Agent))
	fmt.Fprintf(&b, "fledge_version: %s\n", yamlScalar(r.FledgeVersion))
	b.WriteString("---\n")
	return b.Bytes()
}

func (t *Task) Frontmatter() []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", t.ID)
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(t.Title))
	fmt.Fprintf(&b, "plumage: %s\n", t.Requirement)
	fmt.Fprintf(&b, "status: %s\n", t.Status)
	fmt.Fprintf(&b, "priority: %s\n", t.Priority)
	fmt.Fprintf(&b, "depends_on: [%s]\n", strings.Join(t.DependsOn, ", "))
	if t.Oversight != "" {
		fmt.Fprintf(&b, "oversight: %s\n", t.Oversight)
	}
	fmt.Fprintf(&b, "authored: %s\n", t.Authored)
	fmt.Fprintf(&b, "agent: %s\n", yamlScalar(t.Agent))
	fmt.Fprintf(&b, "fledge_version: %s\n", yamlScalar(t.FledgeVersion))
	b.WriteString("---\n")
	return b.Bytes()
}

// Render returns the full file bytes: canonical frontmatter + preserved body.
func (r *Requirement) Render() []byte { return append(r.Frontmatter(), r.Body...) }
func (t *Task) Render() []byte        { return append(t.Frontmatter(), t.Body...) }

// Save atomically rewrites the file at Path.
func (r *Requirement) Save() error { return WriteFileAtomic(r.Path, r.Render()) }
func (t *Task) Save() error        { return WriteFileAtomic(t.Path, t.Render()) }

// WriteFileAtomic writes data via a temp file in the same directory followed
// by rename, so readers never observe a partial file.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fledge-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
