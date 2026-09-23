package profiles

import (
	"bytes"
	"fmt"
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
	SchemaVersion *int64    `toml:"schema_version"`
	Extends       *string   `toml:"extends"`
	Harness       *string   `toml:"harness"`
	Model         *string   `toml:"model"`
	Args          *[]string `toml:"args"`
	Role          *string   `toml:"role"`
	RoleAppend    *string   `toml:"role_append"`
}

// keys are the exact key names a profile file may use.
var keys = []string{"schema_version", "extends", "harness", "model", "args", "role", "role_append"}

// decode strictly parses and validates one profile file.
func decode(data []byte) (file, error) {
	var f file
	md, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&f)
	if err != nil {
		return f, err
	}
	// The decoder matches keys case-insensitively, so check exact names.
	for _, key := range md.Keys() {
		if !slices.Contains(keys, key.String()) {
			return f, fmt.Errorf("unknown key %q", key.String())
		}
	}
	if f.SchemaVersion == nil {
		return f, fmt.Errorf("schema_version is required")
	}
	if *f.SchemaVersion != schemaVersion {
		return f, fmt.Errorf("unsupported schema_version %d; this Fledge reads %d", *f.SchemaVersion, schemaVersion)
	}
	if f.Role != nil && f.RoleAppend != nil {
		return f, fmt.Errorf("role and role_append cannot both be set")
	}
	if f.Harness != nil && !libagent.IsHarness(*f.Harness) {
		return f, fmt.Errorf("harness must be a documented Herdr harness kind")
	}
	texts := []*string{f.Extends, f.Harness, f.Model, f.Role, f.RoleAppend}
	if f.Args != nil {
		for i := range *f.Args {
			texts = append(texts, &(*f.Args)[i])
		}
	}
	for _, s := range texts {
		if s != nil && (!utf8.ValidString(*s) || strings.ContainsRune(*s, 0)) {
			return f, fmt.Errorf("values must be valid UTF-8 without NUL")
		}
	}
	return f, nil
}

// overlay applies the fields f sets to p. Scalars and args replace;
// role_append adds a paragraph to the inherited role.
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
	if f.Role != nil {
		p.Role = *f.Role
	}
	if f.RoleAppend != nil {
		if p.Role == "" {
			p.Role = *f.RoleAppend
		} else {
			p.Role += "\n\n" + *f.RoleAppend
		}
	}
	return p
}

// invalid reports a profile file problem as invalid input naming the file.
func invalid(path string, err error) error { return libagent.Invalid("profile %s: %v", path, err) }
