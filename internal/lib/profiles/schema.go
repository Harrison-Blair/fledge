package profiles

import (
	"bytes"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
)

// schemaVersion is the only profile file version this binary reads.
const schemaVersion = 1

// file is one decoded profile file. Pointers distinguish an omitted field,
// which inherits, from an explicit empty value, which clears.
type file struct {
	SchemaVersion *int64            `toml:"schema_version"`
	Extends       *string           `toml:"extends"`
	Harness       *string           `toml:"harness"`
	Model         *string           `toml:"model"`
	Args          *[]string         `toml:"args"`
	Reads         *[]string         `toml:"reads"`
	Protocol      *bool             `toml:"protocol"`
	Sections      map[string]string `toml:"sections"`
	Append        map[string]string `toml:"sections_append"`
}

// keys are the exact top-level key names a profile file may use.
var keys = []string{"schema_version", "extends", "harness", "model", "args", "reads", "protocol", "sections", "sections_append"}

// sectionNames are the section keys of [sections] and [sections_append], in
// render order.
var sectionNames = []string{"mission", "workflow", "always", "never", "protocol", "report"}

// decode strictly parses and validates one profile file.
func decode(data []byte) (file, error) {
	var f file
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&f)
	if err != nil {
		return f, err
	}
	// The decoder matches keys case-insensitively, so check exact names.
	for _, key := range md.Keys() {
		if key[0] == "role" || key[0] == "role_append" {
			return f, fmt.Errorf("unknown key %q; role was replaced by [sections]; see README \"Profiles\"", key.String())
		}
		if !slices.Contains(keys, key[0]) {
			return f, fmt.Errorf("unknown key %q", key.String())
		}
		if len(key) > 2 || len(key) == 2 && !slices.Contains(sectionNames, key[1]) {
			return f, fmt.Errorf("unknown section %q; sections are %s", key.String(), strings.Join(sectionNames, ", "))
		}
	}
	if f.SchemaVersion == nil {
		return f, fmt.Errorf("schema_version is required")
	}
	if *f.SchemaVersion != schemaVersion {
		return f, fmt.Errorf("unsupported schema_version %d; this Fledge reads %d", *f.SchemaVersion, schemaVersion)
	}
	for _, table := range []string{"sections", "sections_append"} {
		if md.IsDefined(table) && md.Type(table) != "Hash" {
			return f, fmt.Errorf("%s must be a table", table)
		}
	}
	for name := range f.Sections {
		if _, ok := f.Append[name]; ok {
			return f, fmt.Errorf("section %s cannot be in both [sections] and [sections_append]", name)
		}
	}
	if f.Harness != nil && !libagent.IsHarness(*f.Harness) {
		return f, fmt.Errorf("harness must be a documented Herdr harness kind")
	}
	texts := []*string{f.Extends, f.Harness, f.Model}
	for _, list := range []*[]string{f.Args, f.Reads} {
		if list != nil {
			for i := range *list {
				texts = append(texts, &(*list)[i])
			}
		}
	}
	for _, table := range []map[string]string{f.Sections, f.Append} {
		for _, text := range table {
			texts = append(texts, &text)
		}
	}
	for _, s := range texts {
		if s != nil && (!utf8.ValidString(*s) || strings.ContainsRune(*s, 0)) {
			return f, fmt.Errorf("values must be valid UTF-8 without NUL")
		}
	}
	if f.Reads != nil {
		for _, r := range *f.Reads {
			if path.IsAbs(r) || slices.Contains(strings.Split(r, "/"), "..") {
				return f, fmt.Errorf("reads entry %q must be a relative path without .. segments", r)
			}
		}
	}
	return f, nil
}

// overlay applies the fields f sets to p. Scalars, lists, and [sections]
// entries replace; [sections_append] adds a paragraph to the inherited section.
func (p Profile) overlay(f file) Profile {
	if f.Harness != nil {
		p.Harness = *f.Harness
	}
	if f.Model != nil {
		p.Model = *f.Model
	}
	if f.Args != nil {
		p.Args = append([]string{}, *f.Args...)
	}
	if f.Reads != nil {
		p.Reads = append([]string{}, *f.Reads...)
	}
	if f.Protocol != nil {
		p.Protocol = *f.Protocol
	}
	for name, text := range f.Sections {
		*p.Sections.field(name) = text
	}
	for name, text := range f.Append {
		if s := p.Sections.field(name); *s == "" {
			*s = text
		} else {
			*s += "\n\n" + text
		}
	}
	return p
}

// invalid reports a profile file problem as invalid input naming the file.
func invalid(path string, err error) error { return libagent.Invalid("profile %s: %v", path, err) }
