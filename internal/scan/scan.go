// Package scan walks a directory tree, skipping anything .fledgeignore excludes.
package scan

import (
	"io/fs"
	"path/filepath"

	"github.com/Harrison-Blair/fledge/internal/ignore"
)

// File is one scanned file: a slash-separated path relative to the scan root,
// and its size in bytes.
type File struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Files returns every file under root that m does not ignore, in lexical order.
//
// An ignored directory is pruned, so — as in git — a "!" line cannot
// re-include anything beneath an excluded directory.
func Files(root string, m *ignore.Matcher) ([]File, error) {
	var out []File

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if m.Match(rel, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			out = append(out, File{Path: rel, Size: info.Size()})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
