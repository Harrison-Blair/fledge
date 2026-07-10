package bootstrap

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// Stamp is the scaffold record written as .fledge/scaffold.json after a
// successful fledge init. It records which fledge version last ran init, the
// full set of agent adapters scaffolded into this repo so far, and a content
// manifest of every file init wrote (hash, symlink target, or append lines).
type Stamp struct {
	FledgeVersion string                `json:"fledgeVersion"`
	Agents        []string              `json:"agents"`
	Files         map[string]StampEntry `json:"files"`
}

// StampEntry is one file's record in the scaffold manifest.
// Exactly one of Sha256, Target, or Lines is non-zero:
//   - content-bearing files (core, default, generate, primitive_map, overwrite): Sha256
//   - symlink: Target (relative symlink target as declared in the manifest)
//   - append_if_missing: Lines (the lines ensured present)
type StampEntry struct {
	Policy string   `json:"policy"`
	Sha256 string   `json:"sha256,omitempty"`
	Target string   `json:"target,omitempty"`
	Lines  []string `json:"lines,omitempty"`
}

// stampPath is the repo-relative path of the stamp file; excluded from its own
// file map and from ExpectedFiles output.
const stampPath = ".fledge/scaffold.json"

// LoadStamp reads the scaffold stamp from root/.fledge/scaffold.json.
// Returns (nil, nil) when the file is absent — the no-stamp path is cheap and
// non-erroring.
func LoadStamp(root string) (*Stamp, error) {
	data, err := os.ReadFile(filepath.Join(root, ".fledge", "scaffold.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load stamp: %w", err)
	}
	var s Stamp
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("load stamp: %w", err)
	}
	return &s, nil
}

// marshalStamp produces the canonical JSON bytes for s: indented with two
// spaces, trailing newline. Go's encoding/json sorts map keys alphabetically,
// making the output deterministic without extra work.
func marshalStamp(s *Stamp) ([]byte, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Write marshals s and writes it to root/.fledge/scaffold.json via
// writeIfChanged. Returns wrote=true when the file was created or its bytes
// changed (wrote=false means byte-identical, no disk write performed).
func (s *Stamp) Write(root string) (bool, error) {
	data, err := marshalStamp(s)
	if err != nil {
		return false, err
	}
	return writeIfChanged(filepath.Join(root, ".fledge", "scaffold.json"), data)
}

// renderEntry returns the bytes that writeFileEntry would write for
// content-bearing file entries (generate, primitive_map, overwrite, default).
// For symlink and append_if_missing entries it returns (nil, nil) — those
// entries record a target or lines, not bytes.
func renderEntry(m *Manifest, f ManifestFile, ctx renderContext) ([]byte, error) {
	if f.Symlink != "" || f.AppendIfMissing != "" || (f.Src == "" && f.Dst != "") {
		return nil, nil
	}
	if f.Generate || f.PrimitiveMap {
		data, err := FS.ReadFile(path.Join(m.dir, f.Src))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Src, err)
		}
		tmpl, err := template.New(f.Src).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", f.Src, err)
		}
		var b bytes.Buffer
		if err := tmpl.Execute(&b, ctx); err != nil {
			return nil, fmt.Errorf("render %s: %w", f.Src, err)
		}
		return b.Bytes(), nil
	}
	// overwrite or default: read verbatim
	data, err := FS.ReadFile(path.Join(m.dir, f.Src))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Src, err)
	}
	return data, nil
}

// filePolicy returns the canonical policy label for a ManifestFile.
// primitive_map takes precedence over generate when both are set.
func filePolicy(f ManifestFile) string {
	switch {
	case f.PrimitiveMap:
		return "primitive_map"
	case f.Generate:
		return "generate"
	case f.Overwrite:
		return "overwrite"
	case f.Symlink != "":
		return "symlink"
	case f.AppendIfMissing != "":
		return "append"
	default:
		return "default"
	}
}

// ExpectedFiles builds the rendered path→StampEntry map for all files that
// WriteCore and m.WriteAdapter would write. The stamp file itself
// (.fledge/scaffold.json) is excluded. commandOrder is fed to generated
// templates (e.g. the Claude allow-list).
//
// This is the shared surface that FTHR-011 (preen drift) and FTHR-012 (refresh
// preserve/prune) build on.
func ExpectedFiles(m *Manifest, commandOrder []string) (map[string]StampEntry, error) {
	out := make(map[string]StampEntry)

	// Core files: embedded under core/ → written to .fledge/<rel>.
	err := fs.WalkDir(FS, "core", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel := strings.TrimPrefix(p, "core/")
		repoPath := ".fledge/" + rel
		data, err := FS.ReadFile(p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		out[repoPath] = StampEntry{
			Policy: "core",
			Sha256: fmt.Sprintf("%x", h),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ExpectedFiles core: %w", err)
	}

	// Adapter files per their write policy.
	ctx := m.renderContext(commandOrder)
	// Accumulate append lines so multiple entries targeting the same dst merge.
	appendLines := map[string][]string{}

	for _, f := range m.Files {
		if f.Dst == stampPath {
			continue
		}
		switch {
		case f.Symlink != "":
			out[f.Dst] = StampEntry{Policy: "symlink", Target: f.Symlink}
		case f.AppendIfMissing != "" || (f.Src == "" && f.Dst != ""):
			appendLines[f.Dst] = append(appendLines[f.Dst], f.AppendIfMissing)
		default:
			data, err := renderEntry(m, f, ctx)
			if err != nil {
				return nil, fmt.Errorf("ExpectedFiles %s: %w", f.Dst, err)
			}
			h := sha256.Sum256(data)
			out[f.Dst] = StampEntry{
				Policy: filePolicy(f),
				Sha256: fmt.Sprintf("%x", h),
			}
		}
	}

	for dst, lines := range appendLines {
		out[dst] = StampEntry{Policy: "append", Lines: lines}
	}

	return out, nil
}
